# ADR 0001：后端技术栈选定为 Go（gin + gorm）

- 状态：已采纳（2026-06-15）
- 范围：仅实现层（后端）。不改任何 OpenSpec 规格、PRD §18 数据模型、前端 `apps/web`。

## 背景

后端语言**此前从未在权威需求源中约定**。检索 `docs/MedOffice_AI_完整产品需求文档_V1.0.md`（唯一权威需求源）与全部 OpenSpec 产物：均对后端语言/框架零约束，design/tasks 一律用抽象说法（`后端服务 / BFF / 业务服务`）。最初的 `apps/api` 用 Node/Fastify + TypeScript 实现，是历史会话在 c01 落地时**未经确认的隐式选择**。复核时决定改用 Go。

## 决策

后端改用 **Go + gin + gorm**，建于 `apps/server`：

- **gin**：HTTP 框架，路由组直译原 Fastify `register*Routes`，中间件做鉴权/权限 preHandler。
- **gorm**：运行时查询层（**不接管 schema**，无 AutoMigrate）；schema 唯一来源是 6 个 `.sql` 迁移（与原 Node 字节一致，`go:embed` 内嵌）。gorm 表达不了处（动态 UPDATE、`set_config`、`information_schema` 内省、`EXTRACT(EPOCH)`、`to_regclass`）用 `db.Raw/db.Exec` 原生 SQL。
- 配套：`gin-contrib/sessions`(memstore 会话)、`aws-sdk-go-v2`(对象存储，presign 与原 aws-sdk-v3 同形)、`golang-jwt/jwt/v5`(ONLYOFFICE HS256)、`x/crypto/bcrypt`(口令，兼容原 bcryptjs `$2a$` 种子)、`crypto/cipher`(AES-256-GCM 凭据加密，格式 `iv.tag.ciphertext` 与 Node 互通)。

**前端不动**：唯一前后端契约是 HTTP API + cookie 会话（vite 代理 `/api`→:3001）。Go 后端在 3001 复刻同一套契约。

## 迁移方式

并行构建 + 末尾切流：保留 Node `apps/api` 原样运行，Go 服务逐层（基建→c01→c02→c03）建于 `apps/server`，每层用对应冒烟把关、Node 充当差分对照；4 层全部对等后切根脚本/端口至 Go 并删除 `apps/api`。

**对等性由移植后的 6 套冒烟保证**（infra/integration/authz/onlyoffice/editor-authz/c03，本机 docker PG+MinIO+ONLYOFFICE 实跑全绿），它们是行为契约（逐条断言状态码、跨租户隔离、写回意图状态机、fallback 审计四要素、解析双链路、schema 内省等）。bcrypt 与 AES-GCM 跨实现互通经 Node→Go golden 验证。

## 安全加固（超出原 Node 端口，对齐医疗红线）

网络策略（私有/离线 provider 不得出公网）在 Go 端比原正则更严：`IsPrivateHost` 改 CIDR 判定，显式拒绝 169.254/链路本地（云元数据）与 0.0.0.0，主机名锚定 DNS 后缀白名单（堵前缀绕过）；provider 出站重定向再过网络策略 + 跨主机剥离鉴权头。生产环境 `cmd/migrate` 跳过演示 seed，不注入已知口令账号。

## 后果

- 正面：Go 静态二进制、并发模型适配后台解析 worker；网络策略加固。
- 代价：一次性重写 `apps/api` ~7100 行实现（规格/前端/DB 复用）。
- 不变：OpenSpec 规格、§18 数据模型、横切契约 owner、前端、docker 依赖、env 变量名。
