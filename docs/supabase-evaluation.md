# 如果底层用 Supabase，会有什么问题？

> **2026-10-30 更新：** v2 已落地为 **Go + PostgreSQL** 自托管架构，没有使用 Supabase Auth/Storage/Realtime，也没有让浏览器直连数据库。本文保留为选型评估；如果未来改用 Supabase，建议只把它当作托管 PostgreSQL，Go API 仍是唯一业务入口。

> 结论：**v0.1 不要用 Supabase 做底座。** 先把静态检查器、词库和 GitHub 案例库跑起来；等出现“账号、投稿、投票、排行榜、多人维护词库”的真实需求，再把 Supabase 作为可选社区层接入。

## 0. v2 的实际选择

v2 已经验证了“账号、投稿、审核、投票、词库搜索”确实需要后端，但选择了更小、更可控的组合：

```text
浏览器 → Go HTTP API → PostgreSQL
                     └── 内存中的词库检测器
```

- 前端仍是原生 JS，无 Node 构建。
- 会话、bcrypt、同源校验和限流都在 Go 内完成。
- 检查原文不落库；只保存账号、词库、案例和投票。
- 纯静态版保留在 `lite/`，可以完全不依赖后端。
- Supabase 若要用，放到 `DATABASE_URL` 后面即可，不需要把 Supabase SDK 带进浏览器。

这条路线避开了 RLS 配置错误、`anon` key 被刷、Edge Function 冷启动和跨境 Auth 延迟等问题；代价是邮箱验证、找回密码等能力需要自己补。

## 1. 这个项目需要后端吗？

| 阶段 | 功能 | 是否需要后端 |
|---|---|---|
| v0.1 | 文案检查、词库搜索、标注稿/SSML 导出 | 不需要，纯前端 |
| v0.1 | 错读案例提交 | 不需要，导出 JSON + GitHub Issue |
| v0.2 | 用户投稿、投票、排行榜、评论 | 需要 |
| v0.2 | 多人维护词库、审核队列 | 需要 |
| v0.3 | 视频/音频上传、单句修复 | 需要对象存储 + 任务队列，Supabase 不是最优解 |

## 2. Supabase 适合什么

Supabase 的优点很明确：

- Postgres + Auth + Storage + Realtime + Edge Functions 一体，MVP 速度快。
- RLS（行级安全）适合“公开读、登录写、管理员审核”的社区数据。
- SQL 可迁移，数据不会被锁死在专有 API 里。
- 开源，可自托管；本地开发有 CLI 和 migrations。
- 对全球用户/开发者社区很友好。

如果这个项目主要面向全球用户，或者你熟悉 Postgres，Supabase 做 v0.2 的社区层是合理的。

## 3. 主要问题

| 问题 | 具体影响 | 建议 |
|---|---|---|
| 中国大陆访问 | Supabase 没有大陆区域；Auth/Storage/Realtime 走境外，登录慢、WebSocket 可能不稳定 | 静态资源走 CDN；不要依赖 Realtime；大陆优先考虑国内云或自建 |
| 登录方式不匹配 | 中国用户习惯微信/手机号；Supabase 原生无微信登录，手机号依赖海外 SMS | 先用 GitHub OAuth + 邮箱；微信登录后续自建 OAuth |
| 视频/音频存储 | Storage 适合小文件，不适合大视频；免费额度和 egress 有限 | 视频不存或放 R2/OSS/COS；Supabase 只存元数据、短音频、封面 |
| 合规 | 视频、声音、声纹可能属于个人信息/敏感个人信息；跨境存储涉及 PIPL；深度合成还需标识和授权 | 默认本地处理、不上传视频；只上传文本和脱敏案例；加隐私政策、删除机制、同意书 |
| 免费额度 | 免费项目可能因不活跃暂停；DB/存储/带宽/MAU 有上限 | 静态站 + GitHub 作为主路径；Supabase 只做可选增强；准备降级方案 |
| RLS 与密钥 | `anon` key 是公开的，RLS 配错会泄露或被刷；`service_role` 绝不能进前端 | 所有表开 RLS；写入走 RPC/Edge Function；加限流、CAPTCHA、审计日志 |
| 中文搜索 | Postgres 默认分词对中文不友好，词典搜索效果差 | 词典放前端 JSON + MiniSearch/Fuse.js；服务端只做投稿和审核 |
| 供应商锁定 | SQL 可迁移，但 Auth/Storage/Realtime 迁移麻烦；自托管是一整套 Docker 服务 | 数据模型用标准 Postgres；定期导出 SQL/CSV；核心逻辑不要写进 Edge Function |
| 内容审核 | 用户投稿可能含侵权、隐私、违规内容 | 先审后发、举报、限流、人工审核队列 |
| 云优先 vs 离线 | 小博主可能没网或不愿上传视频 | local-first；Supabase 只同步词典和投稿 |
| Edge Function 限制 | Deno 冷启动、区域延迟，不适合重媒体处理 | 音视频处理放外部 worker/GPU，Supabase 只存任务状态 |
| 邮箱送达 | 默认 SMTP 有限制，国内送达不稳定 | 配置自定义 SMTP；或先用 GitHub OAuth |

## 4. 推荐架构

### v0.1：静态核心

```text
浏览器
  ├── index.html / app.js / styles.css
  ├── data/glossary.js        # 词库直接打包
  ├── localStorage            # 草稿
  └── GitHub Issue            # 案例投稿
```

零后端、零成本、可离线、手机可用。词库通过 PR 维护，案例通过 Issue 沉淀。

### v0.2：可选 Supabase 社区层

```text
静态前端（GitHub Pages / Cloudflare Pages）
  ├── 本地词库（离线可用）
  └── 可选 Supabase
        ├── Auth：GitHub OAuth / 邮箱
        ├── Postgres：profiles / terms / cases / votes / comments / moderation_log
        ├── RLS：公开只读、登录写、管理员审核
        ├── Edge Function：校验、限流、写审计日志
        └── Storage：只存 ≤30 秒短音频和封面
```

关键原则：

- 前端没有 Supabase 也能用；有 Supabase 只是多社区功能。
- 词库仍以仓库里的 JSON/JS 为准，Supabase 只存“待审核投稿”和“投票”。
- 视频不传 Supabase；如果以后要做成片修复，视频留在本地，或放 R2/OSS/COS。
- 不依赖 Realtime；用轮询或按需刷新。
- 所有写操作走 RPC/Edge Function，不在前端直接 `insert`。
- 本地开发用 Supabase CLI + migrations + seed，贡献者不需要云账号。

## 5. 建议的数据表

| 表 | 用途 | 关键字段 |
|---|---|---|
| `profiles` | 用户资料 | `id`, `display_name`, `avatar_url`, `created_at` |
| `terms` | 词条正式数据 | `id`, `word`, `type`, `risk`, `default_reading`, `note`, `status` |
| `readings` | 候选读音 | `id`, `term_id`, `pinyin`, `note`, `is_default`, `is_wrong` |
| `cases` | 错读案例投稿 | `id`, `user_id`, `word`, `context`, `expected`, `engine`, `audio_url`, `status` |
| `votes` | 案例/词条投票 | `id`, `user_id`, `target_type`, `target_id`, `value` |
| `comments` | 讨论 | `id`, `user_id`, `case_id`, `body`, `created_at` |
| `moderation_log` | 审核记录 | `id`, `moderator_id`, `target_type`, `target_id`, `action`, `reason` |

RLS 建议：

- `terms`、`readings`、已通过的 `cases`：`select` 对 `anon` 开放。
- `cases` 的 `insert`：要求登录，且 `status = 'pending'`。
- `votes`：登录用户只能操作自己的行。
- `comments`：登录可写，先审后显示。
- `moderation_log`：仅管理员。
- Storage：短音频桶公开读，写入要求登录 + 文件类型/大小限制。

## 6. 选型对比

| 方案 | 适合 | 不适合 | 成本 |
|---|---|---|---|
| 静态 + GitHub | v0.1、词库/案例沉淀、零成本 | 非技术用户投稿体验差 | 0 |
| Supabase | 全球社区、需要 Auth/RLS/Postgres | 大陆访问、微信登录、大视频 | 免费层可起步，增长后付费 |
| PocketBase | 小团队自托管、单二进制、SQLite | 大规模、复杂权限、全球多区域 | 低 |
| 腾讯云开发 CloudBase | 大陆用户、微信登录、国内节点 | 供应商锁定、迁移成本 | 按量 |
| 自建 Postgres + API | 完全可控、合规 | 运维成本高 | 中高 |

## 7. 上线前检查清单

如果决定接 Supabase：

- [ ] 所有表开启 RLS，并逐条测试 `anon` / 登录用户 / 管理员权限。
- [ ] `service_role` key 只放在服务端/Edge Function，绝不进前端。
- [ ] 写入接口有校验、限流、CAPTCHA/人机验证。
- [ ] Storage 限制 MIME、大小、时长；音频做转码/扫描。
- [ ] 不存视频；只存元数据和短音频。
- [ ] 隐私政策、删除机制、用户同意书齐全。
- [ ] 不采集未成年人信息；不克隆未授权声音。
- [ ] 配置自定义 SMTP，测试国内邮箱送达。
- [ ] 准备导出 SQL/CSV 的迁移脚本。
- [ ] 免费项目暂停/超额的降级方案：静态站仍可用。

## 8. 一句话

**Supabase 不是不能用，而是太早用会拖慢你，太晚用会重复造轮子。** 对这个面向小博主的开源项目，最稳的是：**静态核心 + GitHub 知识库 + 可选 Supabase 社区层**。先验证“大家愿不愿意用检查器”，再决定要不要账号和数据库。
