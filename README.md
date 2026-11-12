# Luma

**Luma** 是可自行部署的开源节点管理与用户流量面板，包含管理端、用户中心和 Go API。配套节点工程：[chinahch/caddy](https://github.com/chinahch/caddy)。

Vue 3 · TypeScript · Naive UI · Go/Gin · PostgreSQL · Redis · Docker Compose

> **0.1.0-preview**：工程预览版。真实数据库、接口、原生协议回显连接及基础 Compose 换机恢复已验证；生产网络、真实 CF 账户、低内存 ARM64 和业务压测仍需验收。支付、工单、自动续费、XLSX 与原生节点端持久上报尚未实现。

## 已有功能

- 按服务器卡片管理节点、拖拽保存布局、真实物理公网 IP 国家识别；全部协议一键生成及原配置编辑回填；TLS 证书自动下发、申请与续期；服务器定时自测连通性。详见 [节点证书与新版管理说明](docs/node-certificates.md)。

- 管理员/用户会话登录、角色隔离、用户额度与到期时间、套餐记录。
- 服务器一条命令自动接入、在线状态、CPU/内存/磁盘、节点数与心跳；认证自动配置，日常流程不需要复制令牌。
- 创建或编辑节点时直接选择归属服务器、默认落地和分流，节点与路由一起保存并自动同步。
- XBoard 节点 v1/v2 REST 适配：握手、配置、用户列表、报告、Machine 节点发现/状态。握手关闭 WebSocket，客户端使用 REST 轮询。
- 用户 × 节点 × 日期/小时/月用量，UTC 存储、IANA 时区查询、原始与倍率计费用量分离，CSV 精确字节导出。
- v3 可靠批次接收 API：同一节点/批次事务防重，改变内容的同 ID 返回 409，账本与配额计数原子更新。
- 普通用户仅能访问自身记录；个人 VLESS 订阅链接，可主动重新生成以撤销旧链接。
- 深色/浅色、响应式布局。前端从同源 API 获取数据，不内嵌生产账号或演示凭据。
- 落地链接单条/多行导入、去重、编辑、加密保存，按节点默认出站或域名/CIDR 分流；内核不兼容时明确拒绝。
- 15 项协议组合的配置入口：原生 sing-box/Xray、六种 ENC 认证/外观、REALITY、WS、XHTTP/H3；CF Tunnel 和每用户 MTProxy 由独立连接器运行。
- 原生协议分享链接订阅及个人 Telegram 代理链接。详见 [协议、落地和连接器](docs/protocols-exits.md)。

## 一台服务器部署

需要一台 Linux 服务器、Docker Engine 与 Compose V2。当前采用从源码构建，不依赖尚未发布的 GHCR 镜像。

```sh
git clone https://github.com/chinahch/luma.git
cd luma
cp .env.example .env
# 生成数据库密码，填入 .env 的 POSTGRES_PASSWORD；推荐使用此命令的十六进制结果
openssl rand -hex 32
# 再独立生成一份密钥，填入 LUMA_MASTER_KEY，用于落地和节点私密配置加密
openssl rand -hex 32
# 检查 .env 后启动
docker compose up -d --build --wait
```

默认仅绑定服务器 `127.0.0.1:8088`。本地测试可用 SSH 转发：`ssh -L 8088:127.0.0.1:8088 user@server`，然后访问 `http://localhost:8088`。初始 `.env.example` 使用 HTTP 测试设置，正式接入域名前配置 HTTPS。

创建首个管理员，**没有默认密码**。以下交互会隐藏密码输入，不要把密码提交到 Git：

```sh
read -r -p '管理员邮箱: ' LUMA_ADMIN_EMAIL
read -r -s -p '管理员密码（12–72字节）: ' LUMA_ADMIN_PASSWORD
printf '\n'
export LUMA_ADMIN_EMAIL LUMA_ADMIN_PASSWORD
docker compose run --rm -e LUMA_ADMIN_EMAIL -e LUMA_ADMIN_PASSWORD api admin create
unset LUMA_ADMIN_EMAIL LUMA_ADMIN_PASSWORD
```

登录后先到“服务器管理”添加服务器，复制一条安装命令到节点机以 root 执行。节点使用原生二进制，无需 Docker；安装后即使没有节点也会显示心跳和负载。再创建套餐/用户与节点，选择归属服务器及已有落地，保存后通常在下一次 10 秒同步周期内生效。协议需要的证书与端口仍须在服务器准备。CF Tunnel / MTProxy 使用独立连接器；已安装且使用同一身份的连接器需要协调迁移，安装器会在接入前明确拒绝不安全的轮换。详见 [自动接入](docs/server-enrollment.md)、[部署与换机](docs/deploy.md)、[接口与计费边界](docs/api-and-accounting.md)。

## HTTPS 与正式访问

在宿主机已有 HTTPS 反向代理上将域名指向 `127.0.0.1:8088`。将 `.env` 的 `PUBLIC_URL` 改为实际 `https://panel.example.com`，`COOKIE_SECURE=true`，然后 `docker compose up -d`。保持数据库/Redis 无宿主机端口映射。外层代理必须保留原始 `Host`，不得记录订阅路径与带 token 的节点请求查询串。

## 更换服务器

代码仓库不包含数据库。迁移时同时准备数据库备份、私有配置和部署版本：

```sh
# 旧服务器，备份文件为私密文件
sh scripts/backup.sh
# 另外安全保存 .env、当前 git commit、TLS/反向代理配置

# 新服务器：检出与备份匹配的源码版本，放好 .env 后
docker compose up -d db redis --wait
sh scripts/restore.sh /secure/path/luma-backup.dump
docker compose up -d --build --wait
```

恢复脚本只接受空数据库，不会先删除旧库；API 自动迁移前应先恢复。验证登录、账号与节点数量、配额、用量账本后再切换域名。建议保持原域名，避免逐个修改节点面板地址。Redis 仅为可重建快照缓存，不承载唯一账本或待计费用量。

## 本地开发与测试

后端使用 Go 1.26.10。准备专用 PostgreSQL 测试库，名称必须以 `_test` 结尾：

```sh
export DATABASE_URL='postgres://luma:YOUR_PASSWORD@127.0.0.1:5432/luma?sslmode=disable'
export TEST_DATABASE_URL='postgres://luma:YOUR_PASSWORD@127.0.0.1:5432/luma_test?sslmode=disable'
go test ./...
go run ./cmd/luma migrate
COOKIE_SECURE=false PUBLIC_URL=http://127.0.0.1:15173 LISTEN_ADDR=127.0.0.1:18080 go run ./cmd/luma
```

测试在专用数据库中创建随机 schema 并清理，不触碰其他 schema。没有 `TEST_DATABASE_URL` 时数据库测试会跳过；仅此时的绿色结果不能视为集成验收。

前端需要 Node 24 与 pnpm 11.25.0：

```sh
cd web
corepack enable
pnpm install --frozen-lockfile
pnpm build
pnpm dev
```

Windows 受限环境可用构建后的 `pnpm exec vite preview --host 127.0.0.1 --port 15173 --configLoader native` 代替开发预打包；它复用开发配置中的 API 代理。HTTP 本地测试明确设置 `COOKIE_SECURE=false`，生产使用 HTTPS/secure cookie。

## 数据语义

旧节点仍上报**增量窗口**，但缺乏稳定批次 ID，失败缓存只在内存。Luma 将它标为 `legacy`，无法知道“服务端已提交但响应丢失”的重报是否重复，不能承诺精确一次。v3 服务器防重只有在发送端也使用持久且重试不变的批次身份后，才有端到端意义；当前配套节点尚未实现该发送端升级。

每个采集窗口按结束时间归档；旧协议结束时间为服务器接收时间估计。跨日窗口不按真实字节分割，因为现有源数据没有窗口内明细。倍率在首次入账时由数据库节点配置取快照，重试保留原计费结果。展示平均速率和数据年龄，不宣称瞬时网速。

## 项目结构与许可

- `cmd/luma`：服务、迁移、管理员初始化。
- `internal/store`：SQL 迁移、原始用量和计费事务。
- `internal/httpapi`：会话/RBAC、管理、节点契约与查询。
- `web`：管理端和用户中心。
- `deploy` / `scripts`：同源代理、备份与空库恢复。

Luma 原创代码采用 [MIT](LICENSE)，依赖遵循各自许可。安全问题见 [SECURITY.md](SECURITY.md)。

账户与节点升级：管理员可使用节点、稳定订阅、邀请注册、多套餐权限、月度流量重置、节点列表启停/删除、原生落地连通及 Ping、故障诊断和排除 WARP 的物理公网 IP 自动填入，见 [使用与升级说明](docs/account-plans-health.md)。
