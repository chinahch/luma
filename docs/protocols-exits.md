# 协议、落地和连接器

节点编辑窗口的“协议预设”生成实际节点配置与新密钥。按“生成并填入配置”后检查 JSON，再保存节点。编辑现有节点时不必重新生成，否则会换密钥。此功能需要配套 Caddy 的 `landing-v1` 版本。

| 需求 | 预设 / 执行方式 | 监听与客户端 |
|---|---|---|
| SOCKS5 | sing-box | TCP/UDP；用户名和密码均为用户 UUID |
| VLESS REALITY | Xray | TCP；Vision |
| Hysteria2 | sing-box | UDP；有效 TLS 证书 |
| CF Tunnel | VLESS WS + luma-connect/cloudflared | 本机 HTTP WS 源站；公网 443/TLS |
| SS / SS2022 | sing-box | TCP/UDP；SS2022 含服务端与用户密钥 |
| VLESS ENC REALITY Vision | Xray | TCP；六种认证/外观组合 |
| VLESS ENC | Xray | 原生加密；六种组合 |
| VMess WS | Xray | WS/TLS |
| VLESS WS | Xray | WS/TLS |
| AnyTLS REALITY | sing-box | TCP；REALITY |
| AnyTLS TLS | sing-box | TCP；有效 TLS 证书 |
| MTProto / MTProxy | luma-connect/mtg | 每用户一个独立 TCP 端口和密钥 |
| VLESS XHTTP REALITY | Xray | 独立路径 |
| VLESS XHTTP REALITY ENC/Vision | Xray | 独立路径；六种组合 |
| VLESS XHTTP TLS H3 | Xray | 独立端口/路径；UDP/QUIC，ALPN h3 |

这些组合的固定内核配置解析已测试；配置检查不等同于运营商、客户端版本及实际网络的联通验收。CF 还需真实 Cloudflare 账户配置。客户端必须支持所选协议及 ENC/H3 等功能。

ENC 的六种组合是 X25519 / ML-KEM-768 认证，各自配 native、xorpub、random 外观。服务端 `decryption` 和客户端 `encryption` 使用不同密钥。XHTTP 上的 Vision 预设同时启用原生 ENC，不能把普通 TCP Vision 的约束直接照搬到任意传输。独立 Node 模式必须在本地选择匹配内核；Machine 模式启动时按面板的显式内核选择。

## 粘贴落地链接

先在 `.env` 设置独立的 `LUMA_MASTER_KEY`，使用 `openssl rand -hex 32` 生成。数据库密码和此密钥不要使用同一个值。执行 `docker compose up -d`，后在“落地与路由”粘贴一条或多条分享链接。

支持 SOCKS5、HTTP(S)、VLESS、VMess、SS、Trojan、Hysteria2、AnyTLS；每行一条，最多 200 行 / 1 MiB。解析不访问链接中的服务器，也不抓取订阅 URL。预览显示行号错误和重复项，可以修改名称、地址、端口或完整参数，再保存所选落地。列表不返回密码；管理员进入编辑时才读取完整配置。

选择“节点路由”，设置默认落地，或按顺序添加域名后缀/CIDR 分流。同一行域名和 CIDR 任一命中即选中该落地；私网拦截先匹配。原有自定义规则保留在默认规则之前。协议不支持当前内核时拒绝保存；旧节点未声明 `landing-v1` 时拒绝下发，不能静默直连。停用落地后下发会报错，正在运行的原生节点保留上次成功配置，需检查节点状态。

MTG 只接受默认的普通 SOCKS5 落地，不支持域名/CIDR 分流。其他落地协议应先在本机设置合适的 SOCKS5 中转，再绑定 MTG。

## Cloudflare Tunnel

1. 在 Cloudflare 创建命名隧道，取得其运行令牌。
2. Luma 创建 `Cloudflare Tunnel + VLESS WS` 节点，填写同一服务器上的独立源站端口、独立指标端口、路径、隧道令牌。将节点关联到运行连接器的 Machine。
3. 在 Cloudflare 将公网主机名映射到 `http://127.0.0.1:源站端口`。Luma 节点的“对外域名”填写这个公网主机名。
4. 同机运行原生 Caddy 和 `luma-connect`。原生节点仅监听本机 WS；cloudflared 从私有令牌文件运行。用户订阅使用公网主机名与 443/TLS。

这里不调用 Cloudflare 管理 API，公网 DNS/隧道路由由账户所有者配置。公开 HTTP 隧道不能当作任意 UDP 转发。CF 流量由下层原生节点按用户统计，不另外把隧道接口流量再计费一次。

## MTProxy

创建 `MTProto / MTProxy` 预设，选择一个普通用户、独立监听端口和指标端口。每个实例只有该用户的一把 FakeTLS 密钥；一个逻辑节点的归属用户创建后不能变更，换用户应创建新节点。用户在“我的订阅”获取专用 `tg://proxy` 链接。MTProxy 不进入原生内核的 Machine 节点列表。

`luma-connect` 只统计 mtg 的 Telegram 方向字节，排除域名前置流量。每个采样批次先以固定身份写盘再提交；已提交但响应丢失时重试由服务器去重。队列写入失败会停止受影响实例。进程崩溃前尚未采样的字节可能丢失，不能据此承诺端到端零丢失。额度/到期在轮询时生效，可能有一个采样/刷新窗口的超额。

## 密钥与换机

AES-256-GCM 加密落地凭据及节点私密配置，密文绑定记录身份；列表仅返回公开投影。首次提供密钥时，旧的节点私密 JSON 会原子转换为加密记录。迁移必须保留相同的 `LUMA_MASTER_KEY`，新生成的密钥无法解密旧数据库。

除 PostgreSQL、`.env`、证书和部署 commit 外，使用连接器的机器还要停服务后安全迁移 `/var/lib/caddy-connect/queue`，并确保旧 worker 不再运行，避免新旧机器重复提供同一实例。启动前核对节点/Machine 身份、监听端口、域名与证书。不要将私有配置、队列或数据库备份提交到 Git。

连接器的安装和 systemd 设置见 [Caddy connector](https://github.com/chinahch/caddy/tree/main/connector)。
