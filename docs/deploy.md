# 部署、升级与迁移

## 初次安装
使用官方 Docker Engine / Compose V2。README 的命令为源码构建路径，两个镜像在本机生成。建议先在隔离测试服务器验证；最低内存和并发容量尚无压测结论，不把节点 128 MiB 目标套用于 PostgreSQL+面板整套服务。

记录 `git rev-parse HEAD` 与 `.env`，`POSTGRES_PASSWORD` 使用 URL 安全的随机十六进制字符串（它会进入数据库 URL）。已有库更改该环境值不会自动修改 PostgreSQL 内部密码，需按数据库管理流程同步调整。

默认端口只绑定本机；通过 SSH tunnel 或宿主机 HTTPS Nginx 访问。正式域名设置 PUBLIC_URL 和 COOKIE_SECURE=true。外层代理示例（证书由你现有流程配置）：

```nginx
location / {
    proxy_pass http://127.0.0.1:8088;
    proxy_set_header Host $http_host;
    proxy_read_timeout 30s;
}
```

不要把普通 access log 应用于 `/sub/` 或节点 API 查询串；其内容含凭据。内置 Nginx 禁用访问日志，只保留 critical 级错误。应用不记录请求体与查询串。

## 数据与备份
机器管理提供“安装 / 接入”命令，节点端使用原生二进制，无需 Docker。先创建并关联逻辑节点，再执行机器命令；执行时按提示粘贴令牌。旧令牌不可读回，丢失后可确认重置，随后更新原生节点及同身份连接器的凭据。删除机器仅允许无关联节点的记录，保留节点与账本，不会远程卸载或停止旧服务；应先在节点机停止对应进程。面板地址必须从节点可达，不能用浏览器 SSH 转发的 localhost 地址。

- PostgreSQL volume `postgres_data` 是持久业务和账本；不在源码仓库。
- Redis 是可重建缓存，无唯一计费队列，丢失缓存不会删除原始用量。
- `.env`、TLS 配置和当前源码 commit 与数据库 dump 一起保存到受控的加密备份位置；不要提交到公开仓库。
- `sh scripts/backup.sh /secure/path/luma.dump` 输出 pg_dump 自定义格式；拒绝覆盖同名文件。备份运行期间其他写入继续，数据按 PostgreSQL 一致快照处理。迁移切换前如要求最终余额一致，应短暂停止节点上报接收/写入并做最终备份。

## 新服务器恢复
1. 检出备份对应的应用 commit，准备私有配置与同版本 PostgreSQL。
2. 仅启动 db/redis：`docker compose up -d db redis --wait`。
3. 执行 restore.sh。若 API 曾启动并建表，脚本会拒绝恢复；改用全新的部署实例/新 volume，不删除现有数据试图绕过。
4. 启动完整应用，验证管理员/普通用户登录、节点归属、用户额度、原始用量与账本合计。
5. 测试节点完成握手、下发和上报后，切换域名。域名不变可避免修改所有节点地址。

单机 Compose 并不提供无停机迁移。旧节点没有持久上报队列，停机窗口可能丢用量；不要将它称为零损失计费迁移。

## 更新与回滚
更新前备份并记录当前 commit。检出目标版本，`docker compose build` 后启动；API 用 PostgreSQL advisory lock 串行执行迁移。当前数据库版本为 3（v2 落地/路由，v3 节点私密配置）；遇到不支持的版本会拒绝启动，不尝试任意降级。旧 v1 程序无法直接读取 v3 数据库，回滚应使用对应备份恢复到新实例。

启用协议密钥/落地后必须同时备份相同的 `LUMA_MASTER_KEY`。连接器还需停机迁移私有待上报队列；具体步骤见 [协议、落地和连接器](protocols-exits.md)。

应用回滚必须与数据库兼容；若涉及不兼容迁移，使用更新前备份和匹配代码恢复到新的实例。备份之后产生的数据需要单独核对，不承诺自动合并。

不要运行 `docker compose down -v` 作为日常更新命令；它会删除持久卷。排障先查看 `docker compose ps`、`docker compose logs --tail 100 api` 与 `/healthz`。避免把含凭据的环境/完整配置粘贴到公开 issue。

## 验证状态
本地已使用独立 PostgreSQL 验证事务与鉴权，前端可构建并有本机浏览器 QA。当前 Windows 环境无 Docker；Compose 启动、镜像运行、pg_dump/pg_restore 的完整恢复演练需由 Linux CI 或测试机实际运行后才算通过。见 tests/与 CI 的真实结果，不以本文存在代替验收。
