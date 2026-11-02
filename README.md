# VoxRefine 多音字检查器

面向 AI 配音和短视频文案的中文多音字检查器。粘贴文案或导入 SRT，自动标出高风险多音字，给出读音候选，并导出标注稿、腾讯云 SSML 或百度 SSML。

项目目标是让初做视频的创作者在写稿阶段发现“重庆读成 zhòng qìng”“银行读成 yín xíng”这类问题，而不是等成片后再返工。

## 两个版本

| 版本 | 技术栈 | 适合场景 |
|---|---|---|
| **v2（当前）** | Go 1.25 + PostgreSQL 16 + 原生 JS | 账号、案例库、审核、投票、词库搜索 |
| **lite** | 纯 HTML/CSS/JS，无构建、无后端 | 零成本部署、离线使用、静态托管 |

- v2 页面：`/`
- 轻量静态版：`/lite/`
- v2 的检查文本只在请求内存中处理，不写入数据库。

## 功能

- 粘贴文案或导入 `.srt` / `.txt`，自动识别 UTF-8 / GBK
- 121 条内置高频词条：地名、姓氏、成语、专名、品牌和常见易错词
- 高亮上下文，给出 2–3 个读音候选，显示带声调拼音
- 逐条确认、忽略、全部采用建议、重置选择
- 导出标注稿、腾讯云 SSML、百度 SSML
- 词库搜索：按词语、拼音、说明和类型筛选
- 账号注册/登录、真实错读案例提交、公开案例库、投票
- 管理员/审核员队列：通过或拒绝投稿
- 响应式布局，支持 390px 手机视口
- v2 不保存检查原文；lite 全部在浏览器本地运行

## 技术栈

- **后端**：Go 标准库 `net/http`、`pgx/v5`、`bcrypt`
- **数据库**：PostgreSQL 16，迁移文件随二进制嵌入
- **前端**：原生 HTML/CSS/JavaScript，无 Node、npm 或打包步骤
- **静态版**：`lite/`，可直接双击 `index.html` 使用

## 快速开始

### 环境要求

- Go **1.25+**
- PostgreSQL **16+**（推荐 Docker）
- 不需要 Node.js

### 方式 A：Docker 启动数据库

```bash
cp .env.example .env
# Windows PowerShell 使用：Copy-Item .env.example .env

# 编辑 .env，至少设置 ADMIN_EMAIL
docker compose up -d
go run .
```

打开 <http://127.0.0.1:8080/>。

### 方式 B：Windows 本地 PostgreSQL（无 Docker）

1. 将 PostgreSQL 16 的 Windows 二进制解压到：

```text
%LOCALAPPDATA%\polyphone-checker\pgsql
```

2. 启动数据库：

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File .\scripts\start-local-pg.ps1
```

3. 启动 v2：

```powershell
.\start-v2.bat
# 或：go run .
```

停止数据库：

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File .\scripts\stop-local-pg.ps1
```

> 项目路径包含中文时，PostgreSQL 的 `initdb` 可能失败。脚本因此把数据目录放在 ASCII 路径 `%LOCALAPPDATA%\polyphone-checker\pgdata-ascii`。如果 Windows 用户名本身包含中文，建议直接使用 Docker。

### 构建 v2

```powershell
.\build-v2.bat
```

产物：`bin\polyphone-server.exe`。生产环境建议把它和迁移、静态资源一起部署；二进制已内嵌前端、词库和迁移文件。

### 运行轻量静态版

```powershell
.\start-lite.bat
```

也可以直接把 `lite/` 目录部署到 Cloudflare Pages、Netlify、GitHub Pages 等静态托管。`lite/_headers` 提供 CSP 等响应头示例。

## 配置

服务启动时会读取项目根目录的 `.env`，系统环境变量优先。

| 变量 | 默认值 | 说明 |
|---|---|---|
| `APP_ENV` | `development` | 生产环境设为 `production` |
| `ADDR` | `127.0.0.1:8080` | 监听地址；公网部署建议放在反向代理后面 |
| `DATABASE_URL` | 本地 55432 端口 | PostgreSQL 连接串 |
| `DB_MAX_CONNS` | `12` | pgx 连接池上限 |
| `DB_MIN_CONNS` | `1` | pgx 连接池保底连接数 |
| `ADMIN_EMAIL` | 空 | **首次注册此邮箱的账号自动成为管理员**；上线前必须设置 |
| `SESSION_TTL` | `720h` | 登录会话有效期 |
| `SESSION_COOKIE` | `pc_session` | 会话 Cookie 名 |
| `COOKIE_SECURE` | 生产环境为 `true` | HTTPS 部署必须为 `true` |
| `TRUST_PROXY` | `false` | 只有可信反向代理覆盖 `X-Forwarded-For` 时才开启 |
| `GLOSSARY_SYNC` | `false` | 设为 `true` 时每次启动把 `data/glossary.json` upsert 到数据库 |

### 创建第一个管理员

1. 在 `.env` 中设置 `ADMIN_EMAIL=your-email@example.com`。
2. 启动服务。
3. 用这个邮箱注册第一个账号，角色会自动成为 `admin`。
4. 再按需把其他账号提升为 `moderator`（目前需要通过 SQL 或后续管理功能）。

如果该邮箱已经注册成普通用户，之后再改 `ADMIN_EMAIL` 不会自动提权。

## API

| 方法 | 路径 | 说明 |
|---|---|---|
| `GET` | `/api/health` | 健康检查，包含数据库连通性 |
| `POST` | `/api/check` | 检查文本；请求体 `{ "text": "..." }` |
| `GET` | `/api/terms` | 词库搜索；支持 `q`、`type`、`risk`、`limit`、`offset` |
| `GET` | `/api/terms/{id}` | 单个词条 |
| `GET` | `/api/stats` | 词条、案例、贡献者统计 |
| `POST` | `/api/auth/register` | 注册并自动登录 |
| `POST` | `/api/auth/login` | 登录 |
| `POST` | `/api/auth/logout` | 退出 |
| `GET` | `/api/auth/me` | 当前用户 |
| `GET` | `/api/cases` | 已通过案例 |
| `POST` | `/api/cases` | 提交案例（需登录） |
| `GET` | `/api/my/cases` | 我的投稿（需登录） |
| `POST` | `/api/cases/{id}/vote` | 投票（需登录） |
| `DELETE` | `/api/cases/{id}/vote` | 取消投票（需登录） |
| `GET` | `/api/admin/cases` | 审核队列（`moderator` / `admin`） |
| `POST` | `/api/admin/cases/{id}/moderate` | 审核案例（`moderator` / `admin`） |

`/api/check` 单次最多接收 200,000 字节，最多返回 500 处匹配；前端默认逐步展示。

## 数据与隐私

- 检查原文只在 `/api/check` 请求期间驻留内存，不写入数据库，也不写日志。
- 数据库只保存：账号、会话、词条、案例、投票。
- 前端草稿保存在本机 `localStorage`，清空输入或清理浏览器数据即可删除。
- 不接收视频、音频或文件上传。
- 公开案例中的上下文和备注会展示给其他用户，提交前请自行脱敏。

## 安全设计

- 密码使用 bcrypt（cost 12）；会话 token 只保存 SHA-256 哈希
- Cookie：`HttpOnly`、`SameSite=Lax`，生产环境加 `Secure`
- 所有 SQL 使用参数化查询
- 写接口校验同源 `Origin` / `Sec-Fetch-Site`
- CSP、`nosniff`、`DENY` iframe、Referrer-Policy、Permissions-Policy
- 请求体上限 1 MiB；JSON 禁止未知字段
- 按 IP 的令牌桶限流：
  - API 总限流 120 次/分钟，突发 40
  - 检查接口 60 次/分钟，突发 20
  - 登录/注册 12 次/分钟，突发 6
  - 投稿/投票/审核 20 次/分钟，突发 10
- 限流桶会自动清理，避免长期运行时内存增长

### 公网部署前检查

- 使用 HTTPS，并设置 `COOKIE_SECURE=true`、`APP_ENV=production`
- 修改 `ADMIN_EMAIL` 和数据库密码；不要使用示例密码
- 数据库只允许应用服务器访问，不要直接暴露 5432/55432
- 反向代理层再加一层限流和 WAF
- 定期备份 PostgreSQL；迁移前先在副本验证
- 若要开放注册，补充邮箱验证、找回密码、CAPTCHA/人机验证

## 性能与部署边界

当前 v2 的定位是单机、小社区规模：

- 检查逻辑常驻内存，单次请求不需要查询词库，主要成本是文本扫描
- 默认 PostgreSQL 连接池 12 个连接；可用 `DB_MAX_CONNS` 调整
- 会话保存在 PostgreSQL，多实例部署可共享登录状态
- 限流器在进程内，多实例时每个实例独立计数；公网高并发建议把限流放到网关
- 更大规模可加 PgBouncer、只读副本、CDN 和独立检查服务；不要先把连接池无脑调大

## 项目结构

```text
.
├── main.go                     # 启动、迁移、词库加载、优雅退出
├── internal/
│   ├── config/                 # .env 与环境变量
│   ├── database/               # pgx 连接池与迁移执行器
│   ├── glossary/               # 词库检测器与测试
│   ├── store/                  # PostgreSQL 数据访问
│   └── web/                    # HTTP 路由、鉴权、限流、安全头
├── migrations/                 # 有序 SQL 迁移
├── data/
│   ├── glossary.json           # v2 词库种子
│   └── glossary.js             # v0.1 静态版源数据（兼容保留）
├── web/                        # v2 无构建前端
├── lite/                       # 可独立部署的纯静态轻量版
├── index.html / app.js / styles.css  # v0.1 静态副本（兼容保留）
├── scripts/                    # 本地 PostgreSQL 与冒烟测试脚本
├── tools/glossary-import/      # 从 glossary.js 生成 glossary.json（可选）
├── .github/workflows/test.yml  # 全量测试与冒烟测试
├── docker-compose.yml
├── start-v2.bat / build-v2.bat
└── start-lite.bat
```

日常请使用 `lite/` 作为静态版；根目录的 v0.1 文件只为兼容旧部署保留，`start.bat` 也已改为打开 `lite/index.html`。

## 修改词库

v2 的种子文件是 `data/glossary.json`。静态版使用 `lite/data/glossary.js`。

推荐流程：

1. 修改 `lite/data/glossary.js`（静态版也会同步生效）。
2. 用 `tools/glossary-import` 生成 `data/glossary.json`，或直接手工同步 JSON。
3. 把 `GLOSSARY_SYNC=true` 后重启一次，让种子 upsert 到数据库。
4. 确认无误后改回 `GLOSSARY_SYNC=false`。

`GLOSSARY_SYNC` 不会删除数据库里已不在种子文件中的旧词条。

可选转换工具：

```powershell
cd tools\glossary-import
go run .
```

该工具当前要求 Go 1.26+（依赖 goja）；主服务仍可在 Go 1.25 下构建。

## 测试

```powershell
$env:GOTOOLCHAIN='local'
go test ./...
go test -race ./...
go test ./... -cover
go vet ./...
gofmt -l .
```

默认单元测试不需要 PostgreSQL。持久化集成测试只在设置 `TEST_DATABASE_URL` 时运行，并为每次测试创建、销毁独立 schema：

```powershell
docker compose up -d postgres
$env:GOTOOLCHAIN='local'
$env:TEST_DATABASE_URL='postgres://polyphone:polyphone@127.0.0.1:55432/polyphone?sslmode=disable'
go test ./internal/store -count=1 -v
```

启动本地服务后可运行真实 HTTP 冒烟测试（Windows 使用 Git Bash 或 WSL）：

```powershell
$env:BASE_URL='http://127.0.0.1:8080'
bash scripts/smoke-test.sh
```

冒烟测试覆盖健康检查、主页与 `/lite/`、缓存头、词库搜索、多音字检测、统计、未登录保护、注册、会话、案例提交、退出和重新登录。

检测器模糊测试可单独运行：

```powershell
go test ./internal/glossary -run=^$ -fuzz=FuzzDetectorDoesNotPanic -fuzztime=10s
```

测试分层：

- `internal/config`：环境变量、`.env` 和默认值。
- `internal/glossary`：词库结构、规则匹配、边界输入和模糊测试。
- `internal/web`：HTTP 处理、鉴权、会话、限流、安全头和错误恢复。
- `internal/store`：迁移、词库、账号、案例、投票与审核的 PostgreSQL 集成测试。

GitHub Actions 包含两个任务：全量测试任务运行竞态检测、覆盖率、`go vet` 和格式检查；冒烟任务编译并启动真实服务，连接 PostgreSQL 16 执行 `scripts/smoke-test.sh`，失败时上传服务日志。

Windows 本地运行竞态检测需要启用 CGO 并安装 MinGW-w64 等 C 编译器；没有编译器时请依赖 CI 的竞态检查。

## 后续计划

- v2.1：邮箱验证、找回密码、管理员后台、案例举报
- v2.2：词库版本化、批量导入、审计记录
- 后续评估：视频/SRT 单句重配音与音轨替换；模型、授权和成本另行验证

## 贡献

欢迎提交词条、真实错读案例、误报反馈和文档改进。请先读 [`CONTRIBUTING.md`](CONTRIBUTING.md)。

## 许可证

- 代码：MIT，见 [`LICENSE`](LICENSE)
- 词库与案例数据：CC BY 4.0，见 [`LICENSE-DATA`](LICENSE-DATA)
- 第三方依赖遵循各自许可证

## 安全报告

请不要在公开 Issue 中贴可利用细节。漏洞报告方式和威胁模型见 [`SECURITY.md`](SECURITY.md)。
