# Object Storage Specification

## Purpose
对象存储抽象（MinIO/S3）：文件落盘与 file_hash 计算、下载与按权限的访问控制，为文档版本与各类生成产物提供统一、可离线/私有化降级的存储层。

## Requirements

### Requirement: 对象存储抽象层
系统 SHALL 提供统一的对象存储抽象层，后端 MUST 可由 MinIO 或 S3 兼容实现，且为各类生成产物（文档版本、翻译产物、模板生成文件等）提供统一落盘接口。抽象层 MUST 屏蔽具体后端差异，使上层文档中心与各模块通过同一接口读写文件。在无公网环境下，MUST 支持以本地/内网 MinIO 作为私有化存储后端。

#### Scenario: 通过统一接口落盘
- **WHEN** 上层模块请求保存一个文件对象
- **THEN** 抽象层将其写入已配置的 MinIO/S3 后端并返回对象引用，调用方无需感知具体后端

#### Scenario: 私有化后端可用
- **WHEN** 部署环境无公网且配置为本地/内网 MinIO
- **THEN** 文件落盘与读取仍正常工作，不依赖公网对象存储

#### Scenario: 默认 MinIO 真实落盘全链路
- **WHEN** 以默认 MinIO 后端依次对同一对象执行 `put` → `headObject` → `get` → `presignedUrl` → `delete`
- **THEN** 每步均真实执行成功（非 mock）：`put` 后 `headObject` 返回该对象元数据、`get` 取回与写入一致的内容、`presignedUrl` 生成可访问的短时效链接、`delete` 后该对象不再可取，符合 PRD §0.2 真实接入而非 mock 要求

#### Scenario: 同接口切换 S3 兼容后端
- **WHEN** 将存储后端从默认 MinIO 切换为 S3 兼容后端，保持同一 `ObjectStorage` 接口不变
- **THEN** 同组 `put` / `headObject` / `get` / `presignedUrl` / `delete` 操作在新后端上仍全部通过，上层调用方代码无需改动

### Requirement: 文件落盘与 file_hash
系统 SHALL 在文件落盘时计算并记录其内容哈希 `file_hash`，并将该哈希与文档版本（`document_versions`）关联。`file_hash` MUST 由文件内容确定性计算得到，相同内容 MUST 得到相同哈希，以支撑版本一致性校验与去重判断。

#### Scenario: 落盘计算 file_hash
- **WHEN** 一个文件被写入对象存储
- **THEN** 系统计算其 `file_hash` 并随对应文档版本记录

#### Scenario: 相同内容得到一致哈希
- **WHEN** 两次写入内容完全相同的文件
- **THEN** 两次计算得到的 `file_hash` 相同

#### Scenario: 内容损坏可被校验发现
- **WHEN** 读取文件时其内容与记录的 `file_hash` 不一致
- **THEN** 系统判定校验失败并报告异常，不将损坏内容当作有效文件返回

### Requirement: 下载与访问控制
系统 SHALL 对所有下载与文件访问执行访问控制：下载请求 MUST 先校验请求者在所属租户内对该文件的权限分级，权限不足或跨租户访问 MUST 被拒绝。对象存储 MUST NOT 暴露可绕过权限校验的公开直链；如使用临时访问凭据，其有效期 MUST 受限。每次成功下载 MUST 写入审计日志。

#### Scenario: 有权限下载成功并审计
- **WHEN** 拥有 §10.4「下载」能力（与 document-center「文档权限分级」同源定义同一条「下载能力位」口径：`owner` / `manage` / `edit` / `view` 四级均含下载，仅 `comment`（可评论）级别不含下载，为唯一非单调缺口）的用户在权限校验通过后请求下载
- **THEN** 系统返回文件内容并记录一条下载审计事件

#### Scenario: 跨租户访问被拒绝
- **WHEN** 某租户用户请求另一租户的文件对象
- **THEN** 系统按 `tenant_id` 判定无权限并拒绝，不返回任何文件内容

#### Scenario: 无权限直链被拒绝
- **WHEN** 请求者尝试绕过权限校验直接访问对象存储路径
- **THEN** 系统拒绝访问，不提供未经鉴权的公开直链
