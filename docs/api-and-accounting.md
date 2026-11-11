# API 与计费边界

所有时间存为 UTC。查询 `timezone` 接受 IANA 名称；`from`/`to` 为该时区日期，to 当天包含在范围内。最大 366 天。grain=hour/day/month。返回 up/down/billed 为十进制字符串，避免合计在 JavaScript 中被默默截断；界面人类可读单位为近似显示，CSV 保留整数。

## 会话与管理
POST /api/login：email/password；HttpOnly、SameSite=Strict Cookie。生产设 secure。POST /api/logout、GET /api/me。
GET/POST /api/admin/users|plans|nodes|machines，PATCH users/:id、plans/:id、nodes/:id。无硬删除，以保留历史账本引用。用户已用量不随配额编辑清零；更换套餐不会自动续费或开启新周期。
GET /api/dashboard、/api/usage、/api/usage/export。管理员可按 user_id/node_id 筛选，普通用户只能查询自身。
POST /api/me/subscription-token 返回仅此次可见链接，并撤销旧链接。GET /sub/:token 为 Base64 VLESS 列表，仅支持预览版已声明的传输，不假装所有上游协议均已实现订阅。

## 固定旧客户端契约
Node GET 认证 query：token、node_id、node_type。Machine GET：token、machine_id、node_id。POST 对应字段在 JSON body；也接受 Authorization: Bearer token，身份字段仍必须明确。

| Endpoint | Method | Response/behavior |
|---|---|---|
| /api/v2/server/handshake | POST | websocket.enabled=false；settings.push_interval/pull_interval |
| /api/v1/server/UniProxy/config、user | GET | Node 配置/用户；非 v2-only 替代 |
| /api/v2/server/config、user | GET | Machine 绑定的逻辑节点配置/用户 |
| /api/v2/server/machine/nodes | POST | nodes[] 与 base_config；以实际 client.go 为准，非类型注释中的 GET |
| /api/v2/server/machine/status | POST | CPU/内存/磁盘/网卡状态白名单，不保存 token |
| /api/v2/server/report | POST | traffic={user_id:[upload,download]}、status；legacy 质量 |
| /api/v1/server/UniProxy/push、alive、status | POST | 兼容旧方法；alive 当前只确认、不持久化 IP |

机器 token 必须匹配机器，且提交的 node_id 必须属于该机器；Node token 只能匹配自己的节点。数据库只保存令牌 SHA256。旧接口可能把 token 放在 URL，因此反向代理也必须避免记录原始 URI。

## v3 可靠批次接收
POST /api/v3/server/report，身份同上。示例不包含真实令牌：
```json
{
  "node_id": 1,
  "report_id": "persistent-node-sequence-123",
  "period_start": "2026-11-11T00:00:00Z",
  "period_end": "2026-11-11T00:01:00Z",
  "entries": [{"user_id":2,"up_bytes":100,"down_bytes":200}]
}
```
同一 node_id/report_id 唯一；规范化排序后的相同报告重试返回 duplicate=true，不扣第二次。不同内容复用 ID 返回409。未知用户、重复用户、负量、超过单条上限、窗口超过24h、结束时间超过服务器5分钟、算术溢出拒绝；整批事务回滚。

计费使用 `ceil((up+down)*multiplier_milli/1000)`，每用户每报告向上取整。首次入账快照为服务端当时的节点倍率；补报并不重建历史倍率规则，故不能据此保证任意历史调价的财务结算。原始字节、账本和用户已用计数同事务提交。迟到报告照常入账，不替换更近的速率窗口。

服务器唯一键不能代替发送端持久化。配套节点目前仍是 legacy：若先提交后响应丢失，重报可能重复；异常退出可能丢内存待报量。没有真实周期边界的 legacy 数据按接收时间与配置间隔估算，不承诺秒级速率或精确跨日分摊。
