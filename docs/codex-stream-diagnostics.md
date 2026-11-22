# Codex 长对话流式诊断与分组核对

## 本地修复范围

Responses 合法 `response.completed` / `response.done` 转发成功并读取用量后立即关闭上游，不再等待 EOF 或 `[DONE]`。明确失败、未完成、取消或非 completed 的终态仍为失败。上游 EOF/DONE 会先排空事件队列，防止提前收尾。已经输出的失败请求保持禁止跨渠道重试。

用户反馈 `OutputTextDelta without active item` 后，确认原原生 Responses 转发器遗漏 `response.output_item.added`、`response.content_part.added` 等事件。补丁现按上游顺序转发正常 `response.*` 生命周期、推理摘要、工具参数及扩展事件，保留原始 JSON（含未知字段和序列号）；文本、item.done、usage 的计费处理继续沿用原路径，不重复转发。异常/失败终态仍走错误处理，事件名包含换行或 NUL 时拒绝，防止 SSE 头部注入。

此修复避免“上游有事件但网关丢弃”的客户端空闲；上游自身缺失 active item 时不会伪造输出项补救。原 `: PING` 注释心跳保持不变，不能保证它会重置客户端的协议事件空闲计时，也不伪造文本、完成或重复生命周期事件。上游真正长期没有业务事件的超时仍需客户端实测和请求指标定位。

Relay 业务成功且流正常结束、无处理错误、客户端未取消时，才允许刷新渠道绑定。任务类非 Relay 接口保留原 HTTP 成功约定。绑定失败不续期，不自动移除旧绑定或改动 TTL。

错误记录默认开启，显式 `ERROR_LOG_ENABLED=false` 仍可关闭。错误日志正文使用固定通用消息，不保存上游错误正文；保留错误分类、状态和渠道以定位。管理员诊断放在统一日志 `other.admin_info.stream_diagnostics`，普通用户接口会移除整个 `admin_info`。未新增内容审计、数据库表或正文采集。

## 指标解释

单位均为毫秒。`frt` 保持原含义，不能当首字时间。

| 字段 | 含义 |
| --- | --- |
| first_protocol_event_ms | 从请求开始到收到首个非 DONE 的 SSE data 事件 |
| first_visible_text_ms | 从请求开始到首个非空 Responses 文本 delta 成功写入并 Flush；不代表客户端已渲染 |
| max_upstream_event_gap_ms | 上游相邻 data 事件的最大接收间隔；注释心跳不计入 |
| max_text_gap_ms | 相邻非空文本 delta 完成写入的最大间隔；工具事件、心跳不计入文本 |
| max_downstream_write_ms | Responses 事件写入和 Flush 的最长耗时，不含上游等待 |
| completed_to_return_ms | 完成事件处理开始至 scanner 清理结束，包含完成事件写入和关闭上游 |

无文本时不产生首文本字段。协议事件指标覆盖 scanner；文本与写入指标目前针对 Responses。所有计时仅存固定数量的时间值，不随对话长度增长。此数据不包含提问、回复、工具参数或密钥。

## 2026-10-07 生产分组只读快照

清单来自 enabled abilities 与 status=1 channels 的交集，代表当前配置有可选渠道，**不是上游真实支持或稳定性的实测证明**。未调用付费生成、未启用停用渠道、未改模型映射。

| 模型 | Codex Pro | Codex Plus | codex Pro A组 |
| --- | --- | --- | --- |
| codex-auto-review | 可路由 | 可路由 | 可路由 |
| gpt-5.5 | 可路由 | 可路由 | 可路由 |
| gpt-5.6-sol | 可路由 | 可路由 | 可路由 |
| gpt-5.6-terra | 可路由 | 可路由 | 可路由 |
| gpt-6-astra | 可路由 | 可路由 | 可路由 |
| gpt-6-luna | 可路由 | 不可路由 | 可路由 |
| gpt-6-sol | 可路由 | 可路由 | 可路由 |
| gpt-6.1-sol | 可路由 | 可路由 | 可路由 |
| gpt-5.6-luna | 不可路由 | 不可路由 | 不可路由 |
| gpt-5.4 | 不可路由 | 不可路由 | 不可路由 |

`gpt-5.6-luna` 与 `gpt-6-luna` 是不同请求名，不能互相映射。`Codex Plus低价版` 与 `Codex Plus` 也是不同分组。客户端模型目录若包含不可路由名称，需要选择本分组确实支持的模型；新增支持必须先验证上游协议和模型名称。

## 发布与对照验收

1. 本轮只在本地实施。部署前确认没有显式关闭错误记录，备份 Compose/数据库并记录旧固定镜像；保持内容审计开关、超时、心跳、HTTP/2 和连接池不变。
2. 用同一用户、令牌分组、模型、推理档位和输出要求对比长对话与新对话。真实生成需单独授权或由用户发起。记录请求号、实际渠道、输入/输出 Token、完成状态和上述指标，不记录正文。
3. 管理员在统一日志详情的 `admin_info.stream_diagnostics` 查看指标。无错误日志不代表没有取消，消费日志的 stream_status 也需检查。
4. 事件间隔大而写入短：进一步定位对应上游；写入耗时大：检查客户端网络及消费速度。只有长上下文慢时再验证上下文压缩和推理档位，不直接截断用户上下文。
5. 完成后等待接近零仅证明收尾修复有效，不证明生成中途停顿根因已消失。HTTP 200、平均总耗时或新对话首字快均不能单独作为验收依据。

浏览器工具认证失败，未声称完成网页验收。实际卡顿根因仍需上线后的请求样本确认。

## 本地验证

`relay/common`、`relay/helper`、`relay/channel/openai`、`service`、`middleware`、`controller`、`common` 全包回归通过；`model` 日志权限定向测试通过，`git diff --check` 通过。新增用例覆盖不关闭的上游完成事件、兼容 done、非成功终态、心跳不算文本、Flush 阻塞/取消、写入失败、脱敏错误记录，以及 miniredis 时间推进下失败绑定 TTL 不续期。原有中断、客户端取消、流顺序、工具/图片计费、团队令牌和连接池回归仍通过。

本机未配置可用的 CGO C 编译器，`go test -race` 提示需要 CGO，未完成 race 验证。无前端修改，因此没有重新运行前端构建或声称网页验收完成。

事件兼容补充测试：完整 21 事件序列（created/in_progress、推理摘要、function call、active item/content、text delta/done、annotation、扩展事件、completed）逐字核对下游 SSE，保护顺序、只转发一次和未知字段保留；失败/未完成/扩展事件携带错误与非法事件头拒绝且不泄露错误正文。
