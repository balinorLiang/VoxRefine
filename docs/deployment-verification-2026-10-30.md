# 多音字检查器 v2 部署与功能验收报告

- 验收日期：2026-10-30
- 目标服务器：`192.168.21.145`
- 访问地址：<http://192.168.21.145:8080/>
- 轻量版地址：<http://192.168.21.145:8080/lite/>
- 验收结论：**通过。接口与核心业务流程共执行 33 项检查，33 项通过，0 项失败。**

## 1. 部署环境

| 项目 | 结果 |
|---|---|
| 操作系统 | CentOS Stream 9 |
| 内核 | Linux 5.14.0-354.el9.x86_64 |
| CPU 架构 | x86_64 |
| 内存 | 21 GiB |
| 根分区 | 62 GiB，已用约 85%，剩余约 9.7 GiB |
| 容器运行时 | Docker 29.6.1 |
| 服务管理 | systemd 252 |
| SELinux | Disabled |
| firewalld | inactive |
| 应用版本 | 2.0.0 |
| 应用运行用户 | `polyphone`（系统用户，无登录 Shell） |

## 2. 部署架构

```text
局域网浏览器
    │  HTTP :8080
    ▼
polyphone-checker.service
    │  Go 1.25.5 / 静态编译 / 单二进制
    │  127.0.0.1:55432
    ▼
Docker 容器 polyphone-postgres
    │  PostgreSQL 16.15
    ▼
命名卷 polyphone_pgdata
```

- 前端、词库、数据库迁移文件均已编译进 Go 二进制，不依赖 Node.js。
- 应用直接监听 `0.0.0.0:8080`，当前没有 Nginx、HTTPS 或 WAF。
- PostgreSQL 仅发布到宿主机回环地址 `127.0.0.1:55432`，未对局域网暴露。
- 数据库容器设置了健康检查和 `restart=unless-stopped`。

## 3. 部署结果

| 项目 | 值 |
|---|---|
| 应用目录 | `/opt/polyphone-checker` |
| 二进制 | `/opt/polyphone-checker/polyphone-server` |
| 配置文件 | `/opt/polyphone-checker/.env`，权限 `0640 root:polyphone` |
| systemd 单元 | `/etc/systemd/system/polyphone-checker.service` |
| 数据库容器 | `polyphone-postgres` |
| 数据库卷 | `polyphone_pgdata` |
| PostgreSQL | 16.15，镜像 `postgres:16-alpine` |
| 镜像摘要 | `sha256:0cefaabd882e42bcb2a2f3dc56d3ce4770560ba62fd7f197f1a2e008fd5731ab` |
| Linux 二进制 SHA-256 | `95148B483EFE05DD32B04D19B23C6EC7AADB2543752211A3F9870CC70781DC83` |
| 自动启动 | systemd enabled，Docker restart policy 已启用 |

正式配置的无密摘要：

```env
APP_ENV=production
ADDR=0.0.0.0:8080
DB_MAX_CONNS=12
DB_MIN_CONNS=1
ADMIN_EMAIL=
SESSION_TTL=720h
SESSION_COOKIE=pc_session
COOKIE_SECURE=false
TRUST_PROXY=false
GLOSSARY_SYNC=false
```

数据库口令由部署时随机生成，仅写入远程配置文件，未写入本报告或验收产物。

## 4. 功能验收

### 4.1 页面与静态资源

| 验证项 | 预期 | 实际 | 结果 |
|---|---|---|---|
| 健康检查 | `200`，数据库 `ok` | `{"database":"ok","status":"ok","version":"2.0.0"}` | 通过 |
| v2 主页 | `200`，加载新版界面 | `200`，15,215 字节 | 通过 |
| 轻量版 `/lite/` | `200` | `200`，7,168 字节 | 通过 |
| 桌面布局 | 页面正常渲染、无溢出 | 1440×1000 截图正常 | 通过 |
| 手机布局 | 390px 正常渲染、无横向滚动 | `innerWidth=390`、`scrollWidth=390` | 通过 |

截图：[桌面 1440px](verification-2026-10-30/desktop.png) · [手机 390px](verification-2026-10-30/mobile-390.png)

### 4.2 API 与业务

| 类别 | 验证项 | 预期 | 实际 | 结果 |
|---|---|---|---|---|
| 基础 | 词库统计 | `terms=121` | `200 / terms=121` | 通过 |
| 基础 | 搜索“重庆” | 1 条，默认 `chong2 qing4` | 1 条，读音正确 | 通过 |
| 基础 | 词条详情 | `200 / word=重庆` | 一致 | 通过 |
| 基础 | 多音字检测 | 5 处命中，首项“重庆” | 5 处，首项“重庆” | 通过 |
| 账号 | 管理员注册 | `201 / admin` | `201 / role=admin` | 通过 |
| 账号 | 会话 Cookie | HttpOnly、SameSite=Lax | 标志正确，HTTP 下无 Secure | 通过 |
| 账号 | 当前用户 | `200 / admin` | 一致 | 通过 |
| 账号 | 退出登录 | `200 / user=null` | 一致 | 通过 |
| 账号 | 重新登录 | `200 / admin` | 一致 | 通过 |
| 账号 | 重复邮箱注册 | `409` | `409` | 通过 |
| 账号 | 错误密码登录 | `401` | `401` | 通过 |
| 投稿 | 提交案例 | `201 / pending` | 一致 | 通过 |
| 投稿 | 我的投稿 | 包含新案例 | 包含且状态为 pending | 通过 |
| 投稿 | 待审核不公开 | 公共列表不可见 | 不可见 | 通过 |
| 投稿 | 管理员审核队列 | 包含待审核案例 | 包含 | 通过 |
| 权限 | 普通用户访问审核接口 | `403` | `403` | 通过 |
| 权限 | 未登录写操作 | `401` | `401` | 通过 |
| 审核 | 待审核案例投票 | `404` | `404` | 通过 |
| 审核 | 管理员通过案例 | `200 / approved` | 一致 | 通过 |
| 审核 | 已通过案例公开 | 公共列表可见 | 可见 | 通过 |
| 投票 | 投票 | `votes=1 / voted=true` | 一致 | 通过 |
| 投票 | 取消投票 | `votes=0 / voted=false` | 一致 | 通过 |
| 校验 | 跨站 Origin | `403` | `403` | 通过 |
| 校验 | 未知 JSON 字段 | `400` | `400` | 通过 |
| 校验 | 空文本 | `400` | `400` | 通过 |
| 校验 | 非法词条类型 | `400` | `400` | 通过 |
| 校验 | 超过 200,000 字节文本 | `413` | `413` | 通过 |
| 安全头 | CSP、nosniff、DENY 等 | 全部存在 | 全部存在 | 通过 |
| 跨域 | 默认 CORS | 不开放 | 无 `Access-Control-Allow-Origin` | 通过 |
| 性能 | 10 次顺序检测 | 均为 `200` | 平均 6.9ms，最小 6.1ms，最大 7.6ms | 通过 |
| 限流 | 快速重复检测 | 出现 `429` | 出现 `429` | 通过 |

原始结果：[api-results.json](verification-2026-10-30/api-results.json)

> 管理员流程验证时临时设置了验收邮箱 `deploy-verify-admin@example.invalid`，验收结束后已删除测试账号并恢复 `ADMIN_EMAIL=`。

### 4.3 重启与迁移

- 多次重启应用后健康检查均恢复为 `200`。
- 重启 PostgreSQL 容器后，数据卷仍保留 121 条词条和迁移记录。
- 应用和数据库分别重启后，`/api/health` 再次返回 `database=ok`。
- `schema_migrations` 记录保持为 `0001_init.sql`，迁移没有重复执行导致错误。

## 5. 隐私验证

使用唯一标记 `UNIQUE_CHECK_20261030_ABC123` 发起检测，然后检查数据库导出和应用日志：

| 检查对象 | 结果 |
|---|---|
| PostgreSQL 逻辑导出中的检测原文 | 0 处命中 |
| `journalctl -u polyphone-checker` 中的检测原文 | 0 处命中 |
| 应用日志字段 | 仅方法、路径、状态码、字节数、耗时、IP，不记录请求正文 |
| 数据库表 | `terms`、`readings`、`users`、`sessions`、`cases`、`case_votes`；检测原文没有对应存储字段 |

结论：在本次测试路径中，`/api/check` 的原文只参与请求内存处理，没有写入应用数据库或应用日志。

## 6. 安全评估

已验证的控制：

- 密码使用 bcrypt（cost 12）；会话 token 只在服务端保存 SHA-256 哈希。
- 会话 Cookie 使用 `HttpOnly`、`SameSite=Lax`；当前 HTTP 环境下 `Secure=false`。
- 所有写接口均校验登录态；审核接口额外校验 `moderator/admin` 角色。
- 写请求校验同源 `Origin` 与 `Sec-Fetch-Site`，跨站请求返回 `403`。
- 请求体限制 1 MiB，JSON 禁止未知字段，检测文本限制 200,000 字节。
- 已返回 CSP、`X-Content-Type-Options: nosniff`、`X-Frame-Options: DENY`、Referrer-Policy、Permissions-Policy、CORP。
- 检测、登录、注册、写操作分别有限流桶；快速突发请求已验证返回 `429`。
- PostgreSQL 不暴露到局域网，仅绑定 `127.0.0.1:55432`。
- 应用以 `polyphone` 系统用户运行；systemd 已启用 `NoNewPrivileges`、`ProtectSystem=strict`、`ProtectHome`、`PrivateTmp`、`UMask=0027` 等限制。
- 未发现开放 CORS；没有把数据库口令或管理员口令放入前端。

本次部署仍存在的风险：

1. **当前没有 HTTPS**：流量和会话 Cookie 在局域网内为明文；`COOKIE_SECURE=false`，也不发送 HSTS。
2. **8080 对所有网卡开放**，且宿主机 firewalld 未启用；应限制为仅局域网访问，或通过防火墙/反向代理收口。
3. **没有配置正式管理员邮箱**：当前 `ADMIN_EMAIL=` 为空；首次正式注册前需要设置，否则审核角色无法通过正常注册获得。
4. **注册能力仍较基础**：没有邮箱验证、找回密码、CAPTCHA；开放公网注册前应补齐。
5. **限流是单进程内存状态**：多实例部署时每个实例独立计数；公网高并发场景应把限流放到网关/Redis。
6. **磁盘余量约 9.7 GiB（85% 已用）**：需要监控日志、数据库增长和备份空间。
7. `systemd-analyze security` 仍给出 7.0 MEDIUM 的抽象评分；这不等同于漏洞结论，但说明网络安全层仍不能缺位。

本次没有进行外部渗透测试，以上是配置审查与黑盒功能验证结论，不等同于安全认证。

## 7. 性能与并发结论

- 检测词库常驻内存，单次检测不查询数据库；10 次局域网顺序请求平均 6.9ms。
- 当前定位适合单机、小社区、小博主群体，不是公网高并发架构。
- 单 IP 检测限制为每分钟 60 次、突发 20 次；超过后返回 `429`。
- 未进行长时间压测、故障注入或多实例压测，不能据此承诺高并发容量。
- 若要公开推广，建议先增加反向代理/WAF、接出指标监控、做压测，再考虑多实例和共享限流。

## 8. 清理状态

验收结束后已删除 `deploy-verify-%` 测试账号及其会话、投稿、投票，并恢复正式配置：

| 数据 | 最终数量 |
|---|---:|
| 词条 | 121 |
| 用户 | 0 |
| 会话 | 0 |
| 案例 | 0 |
| 投票 | 0 |

最终健康检查：

```json
{"database":"ok","status":"ok","version":"2.0.0"}
```

最终统计：

```json
{"terms":121,"cases":0,"pending":0,"contributors":0}
```

## 9. 运维命令

在服务器上执行：

```bash
systemctl status polyphone-checker
journalctl -u polyphone-checker -f
curl http://127.0.0.1:8080/api/health
docker ps --filter name=polyphone-postgres
docker logs --tail 100 polyphone-postgres
```

设置首个正式管理员邮箱：

```bash
sed -i 's/^ADMIN_EMAIL=.*/ADMIN_EMAIL=你的邮箱@example.com/' /opt/polyphone-checker/.env
systemctl restart polyphone-checker
```

然后用该邮箱完成首次注册，账号会自动获得 `admin` 角色。

数据库备份示例：

```bash
mkdir -p /var/backups/polyphone
docker exec polyphone-postgres pg_dump -U polyphone -d polyphone -Fc \
  > /var/backups/polyphone/polyphone-$(date +%F-%H%M).dump
```

检查接口复现：

```bash
curl -s http://192.168.21.145:8080/api/health
curl -s -X POST http://192.168.21.145:8080/api/check \
  -H 'Content-Type: application/json' \
  -d '{"text":"欢迎来到重庆。我们公司在银行行业工作，行长今天出差了。"}'
```

## 10. 未覆盖项

- HTTPS、证书申请、HTTP 强制跳转和 `COOKIE_SECURE=true` 的浏览器验证。
- 公网 DNS、外网暴露和真实互联网链路测试。
- 邮箱验证、找回密码、CAPTCHA 等尚未实现的能力。
- 正式管理员邮箱配置后的完整人工审核体验。
- 长时间稳定性、故障注入、多实例共享会话、Redis 限流和数据库主备。
- 备份恢复演练和整机重启验证（应用与数据库已分别重启验证）。
- 多浏览器手工点击回归；本次使用 Chrome 桌面/移动视口截图与 HTTP/API 验收。

## 11. 交付文件

- 部署验收报告：`docs/deployment-verification-2026-10-30.md`
- 接口结果：`docs/verification-2026-10-30/api-results.json`
- 桌面截图：`docs/verification-2026-10-30/desktop.png`
- 移动截图：`docs/verification-2026-10-30/mobile-390.png`

