# 安全说明

## 支持范围

- v2：Go + PostgreSQL 服务端版本
- lite：`lite/` 下的纯静态版本
- v0.1 根目录静态文件仅作兼容保留，不再作为主要维护目标

## 部署形态

### v2

```text
浏览器
  └── HTTPS / 反向代理
        └── Go HTTP 服务
              ├── PostgreSQL
              └── 内存中的词库检测器
```

浏览器不会直接连接 PostgreSQL。数据库连接串、密码和会话管理只存在于服务端。

### lite

```text
浏览器 → 静态 HTML/CSS/JS → 本地计算
```

没有账号、API、Cookie 或数据库；主要风险是静态托管账号、仓库权限和浏览器环境。

## 数据边界

- `/api/check` 的输入文本只在请求期间驻留内存，不写数据库，也不写日志。
- 数据库保存账号、密码哈希、会话哈希、词条、案例和投票。
- 不接收视频、音频、附件或任意文件上传。
- 前端草稿保存在浏览器 `localStorage`，用户清空输入或清理浏览器数据即可删除。
- 公开案例会展示用户提交的上下文、读音和备注；提交者应自行脱敏。

## 已实现的控制

### 账号与会话

- 密码使用 bcrypt，cost 12；密码长度限制 8–72 字节。
- 不存在的邮箱也会执行一次 dummy bcrypt，降低账号枚举的时序差异。
- 会话 token 为 32 字节 CSPRNG 随机值；数据库只保存 SHA-256 哈希。
- Cookie 使用 `HttpOnly`、`SameSite=Lax`；生产环境应启用 `Secure`。
- 过期会话由后台任务每小时清理。

### 请求与接口

- 所有 SQL 使用参数化查询。
- JSON 请求体上限 1 MiB，并禁止未知字段。
- 写接口校验 `Origin`；跨站 `Sec-Fetch-Site: cross-site` 直接拒绝。
- 未启用 CORS；默认只允许同源页面访问 API。
- API 响应使用 `Cache-Control: no-store`。
- 按 IP 的令牌桶限流覆盖全部 `/api/`，检查、登录和写接口还有独立限额。
- 检查文本最多 200,000 字节，匹配结果最多 500 处。
- 词库正则由 Go RE2 执行，不支持回溯引用和 lookahead，避免 ReDoS。

### 浏览器安全

- CSP：`default-src 'self'; script-src 'self'; ...; object-src 'none'; base-uri 'none'; frame-ancestors 'none'`
- `X-Content-Type-Options: nosniff`
- `X-Frame-Options: DENY`
- `Referrer-Policy: strict-origin-when-cross-origin`
- `Permissions-Policy` 关闭摄像头、麦克风和定位
- 生产环境且启用 Secure Cookie 时发送 HSTS

### 权限

- 注册默认角色为 `user`。
- 只有 `moderator` 和 `admin` 能读取审核队列、通过或拒绝案例。
- 管理员身份通过 `ADMIN_EMAIL` 在首次注册时授予。
- 审核记录保存审核人和审核时间。
## 已知限制

- 没有邮箱验证、找回密码、双因素认证和登录设备管理。
- 没有 CAPTCHA；开放公网注册时应加人机验证。
- 限流器在单个进程内存中；多实例时每个实例独立计数，分布式攻击需要网关或 Redis 限流。
- IP 限流无法区分 NAT 后的不同用户；阈值过严会误伤，过松会被刷。
- 只有进程日志，没有独立的审核审计表；案例表记录审核人、时间和备注。
- 没有文件上传，因此没有病毒扫描链路；未来接收音视频时必须补齐。
- 不提供密码修改和会话“全部下线”功能。
- 单个 Go 进程默认最多 12 个数据库连接，适合小社区；高并发需要独立调优和压测。

### `TRUST_PROXY` 注意事项

只有当你确定反向代理会**覆盖** `X-Forwarded-For` 时才开启。若服务可被直接访问且 `TRUST_PROXY=true`，攻击者可以伪造该头绕过按 IP 限流。默认值为 `false`。

### 管理员引导

`ADMIN_EMAIL` 必须在第一次注册前设置。若使用公开示例邮箱或上线后不修改，其他人可能抢注管理员账号。生产环境必须：

1. 设置一个只有你能控制的邮箱。
2. 在服务公开前完成管理员注册。
3. 之后清空 `ADMIN_EMAIL` 或保留但确保该邮箱已验证。

当前没有“通过环境变量提升已有用户角色”的逻辑。

## 公网上线检查清单

- [ ] HTTPS 全站启用，`APP_ENV=production`、`COOKIE_SECURE=true`
- [ ] 修改数据库密码，数据库端口不对公网开放
- [ ] 设置真实 `ADMIN_EMAIL`，先完成管理员注册
- [ ] 关闭 `TRUST_PROXY`，或在可信代理后正确配置
- [ ] 网关/WAF 层限流、防暴力破解
- [ ] 定期备份 PostgreSQL，并演练恢复
- [ ] 添加隐私政策、用户协议、内容投诉和删除渠道
- [ ] 审核队列有人值守；公开案例先审后发
- [ ] 如需开放注册，补邮箱验证、找回密码和 CAPTCHA
- [ ] 不记录原文、密码、Cookie 或数据库连接串
- [ ] 监控 5xx、登录失败率、数据库连接数和磁盘空间

## 使用 Supabase 时

推荐把 Supabase 仅作为托管 PostgreSQL，Go 服务仍是唯一 API：

- `DATABASE_URL` 放服务端环境变量或密钥管理系统，绝不进前端。
- 使用 Supabase Pooler 连接，注意连接数上限；Go 侧用 `DB_MAX_CONNS` 限流。
- 前端不使用 `service_role` key；如果使用 `anon` key 直连，必须逐表配置并测试 RLS。
- 本项目当前的账号、审核、投票逻辑在 Go 内；切换为 Supabase Auth 会改变会话模型，应单独评估。
- 检查文本不写入数据库；不要把视频/音频默认上传到 Storage。
- 跨境存储个人信息、声纹或音视频前，先评估适用法律与合规义务。

## 静态版（lite）的安全说明

- 没有网络请求；词库、脚本和草稿都在本机。
- 主要风险是静态托管账号或仓库被攻破、浏览器扩展读取页面、以及共享电脑读取 `localStorage`。
- 部署时使用 `lite/_headers` 中的 CSP；GitHub Pages 不支持 `_headers`，需要改用 Cloudflare Pages/Netlify 或自行加 meta CSP。
- 词库 `pattern` 仍应人工审核；Go 版已用 RE2，静态版使用浏览器正则，恶意超长/嵌套正则可能影响本机页面。

## 不在范围

- 用户自己的浏览器扩展、共享电脑、恶意输入法或系统级恶意软件。
- 用户主动粘贴并公开的敏感内容。
- 托管商、GitHub 账号或第三方 DNS 被攻破。
- 第三方模型、TTS、字体或云服务的许可和安全问题。
- 用户把自己的数据库密码泄露到公开仓库。

## 报告漏洞

请优先使用 GitHub 仓库的 **Security → Report a vulnerability** 私密报告，或在私下联系维护者。不要先在公开 Issue 里贴可利用细节。

报告请包含：

- 影响版本和部署方式
- 复现步骤
- 影响范围（XSS、越权、限流绕过、数据泄露等）
- 建议修复方式，或最小复现

当前没有漏洞赏金计划；负责任的披露会在修复后致谢（如你愿意署名）。