## ADDED Requirements

### Requirement: 引用角标与参考资料结构
答案 SHALL 在关键结论处带引用角标（如 [1][2]），并在末尾展示结构化参考资料。参考资料每条 MUST 标明来源类型与定位信息：PubMed 来源形如「[1] PMID: xxxxx, Title, Journal, Year」；上传文档来源形如「[2] 上传文档：xxx.pdf，第 3 页，Methods 段」；知识库来源 MUST 标识知识库文档与 chunk。引用角标与参考资料条目 MUST 一一对应。引用可点击率验收目标 MUST ≥ 95%。

#### Scenario: 关键结论带角标并对应参考资料
- **WHEN** 系统生成含 PubMed 与上传文档来源的答案
- **THEN** 关键结论后附 [1][2] 角标，末尾参考资料按来源类型结构化列出，角标与条目一一对应

#### Scenario: 上传文档引用标注页码段落
- **WHEN** 答案引用来自上传文档的内容
- **THEN** 参考资料条目标注文档名、页码与段落（如「上传文档：xxx.pdf，第 3 页，Methods 段」）

### Requirement: 点击引用角标定位来源
点击引用角标后，系统 SHALL 按来源类型定位：PubMed 来源 MUST 打开 PubMed 文章详情；上传文件来源 MUST 打开文档预览并定位到对应页码/段落；知识库来源 MUST 打开知识库文档并定位到对应 chunk。引用源定位成功率验收目标 MUST ≥ 90%，引用定位页码误差 MUST ≤ 1 页。定位前 MUST 校验当前用户对该来源的访问权限（tenant_id/kb_id/user_id/role/document_acl/chunk_acl）。

#### Scenario: 点击 PubMed 角标打开文章详情
- **WHEN** 用户点击 PubMed 来源的引用角标
- **THEN** 系统打开对应 PubMed 文章详情

#### Scenario: 点击上传文件角标定位页码段落
- **WHEN** 用户点击上传文件来源的引用角标
- **THEN** 系统打开文档预览并定位到引用的页码/段落，页码误差 ≤ 1 页

#### Scenario: 点击知识库角标定位 chunk
- **WHEN** 用户点击知识库来源的引用角标
- **THEN** 系统打开知识库文档并定位到对应 chunk

#### Scenario: 定位前校验权限
- **WHEN** 用户点击引用角标
- **THEN** 系统先按 tenant_id/kb_id/user_id/role/document_acl/chunk_acl 校验访问权限，通过后方可打开来源

### Requirement: 引用异常分支
引用定位遇异常时，系统 SHALL 降级提示而不暴露越权或失效内容：权限不足提示「该引用源暂时不可用」；原文已删除提示「该引用源已删除」；外链失效提示「该引用源暂时不可用」；chunk 定位失败提示「已打开来源文档，请手动查看相关段落」。系统 MUST NOT 因引用异常而中断整篇答案的展示。

#### Scenario: 权限不足降级提示
- **WHEN** 用户点击的引用源其无访问权限
- **THEN** 系统提示「该引用源暂时不可用」，MUST NOT 暴露源内容

#### Scenario: 原文已删除提示
- **WHEN** 引用对应的原文已被删除
- **THEN** 系统提示「该引用源已删除」

#### Scenario: 外链失效提示
- **WHEN** PubMed/外链引用源不可达
- **THEN** 系统提示「该引用源暂时不可用」

#### Scenario: chunk 定位失败降级到文档
- **WHEN** 知识库/上传文件 chunk 精确定位失败
- **THEN** 系统打开来源文档并提示「已打开来源文档，请手动查看相关段落」

### Requirement: 引用点击与溯源审计
引用点击、来源定位与外部导入授权等溯源相关操作 SHALL 写入 audit_logs / agent_steps，满足可审计红线。审计记录 MUST 包含操作用户、来源标识与定位结果，便于追溯引用使用情况。

#### Scenario: 引用点击写入审计
- **WHEN** 用户点击引用角标定位来源
- **THEN** 系统记录该点击与定位结果到 audit_logs/agent_steps，含用户与来源标识
