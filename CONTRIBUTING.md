# 贡献指南

这个项目的核心资产不只有代码，还包括**公开、可复核、可积累的中文读音词库和真实错读案例**。欢迎从最小的词条或案例开始。

## 可以贡献什么

- 新词条：多音字、地名、姓氏、品牌、专名、成语
- 真实错读案例：AI 配音里的实际错误，最好带上下文和 TTS 引擎
- 误报反馈：检查器把正常读音标成了风险
- 代码：后端、前端、可访问性、性能、安全和测试
- 文档：新手教程、剪辑/配音工作流、部署说明

## 开发环境

- Go 1.25+
- PostgreSQL 16+（推荐 Docker）
- 不需要 Node.js、npm 或前端构建

```powershell
Copy-Item .env.example .env
docker compose up -d
go run .
```

没有 Docker 的 Windows 环境可使用：

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File .\scripts\start-local-pg.ps1
.\start-v2.bat
```

第一次启动前在 `.env` 设置 `ADMIN_EMAIL`；用该邮箱注册的第一个账号会成为管理员。

## 目录约定

| 路径 | 说明 |
|---|---|
| `internal/glossary/` | 词库检测器和核心测试 |
| `internal/web/` | 路由、鉴权、限流、安全头 |
| `internal/store/` | PostgreSQL 数据访问 |
| `web/` | v2 无构建前端 |
| `lite/` | 可独立部署的纯静态版 |
| `data/glossary.json` | v2 词库种子 |
| `lite/data/glossary.js` | 静态版词库 |
| `migrations/` | 有序 SQL 迁移，只追加不改旧文件 |

## 修改词库

v2 和 lite 使用不同的词库文件格式：

1. 修改 `lite/data/glossary.js`。
2. 运行 `tools/glossary-import` 生成 `data/glossary.json`，或手工同步两份文件。
3. 本地把 `GLOSSARY_SYNC=true` 后重启，让种子写入数据库。
4. 验证检查结果后再提交。

可选转换工具：

```powershell
cd tools\glossary-import
go run .
```

该工具当前使用 Go 1.26+ 和 goja；主服务要求 Go 1.25+。

### 词条字段

```js
{
  id: "chongqing",
  word: "重庆",
  type: "place",
  risk: "high",
  default: "chong2 qing4",
  readings: [
    { pinyin: "chong2 qing4", note: "地名" },
    { pinyin: "zhong4 qing4", note: "误读", wrong: true }
  ],
  note: "“重”在地名重庆中读 chóng，不读 zhòng。"
}
```

- 拼音使用数字声调：`1` 阴平、`2` 阳平、`3` 上声、`4` 去声、`5` 轻声。
- `ü` 用 `v` 表示，例如 `nv3 wa1`。
- 一个词条只描述一个语境；多语境请拆条或用 `pattern` 限定。
- 不要用单字做全局替换。
- 姓氏等需要上下文的词，应该加明确的后续词限制。
- 读音有争议时在 `note` 里说明语境和依据，不要硬下结论。

### 关于 `pattern`

Go 使用 RE2，不支持 lookahead/backreference。静态版可以使用 JavaScript 正则，但两边需要保持兼容。

v2 种子里的姓氏 lookahead 会在导入时转换为 `context_after` 后缀列表，例如“曾先生/曾女士/曾老师”。不要写嵌套量词、超长分支或回溯爆炸正则。

## 代码规范

- 后端保持 Go 标准库优先，新增依赖要有明确理由。
- 前端保持无构建、无第三方运行时依赖；不要引入 Node 构建链。
- 所有 SQL 使用参数化查询。
- 动态 HTML 必须经过 `escapeHtml`；不要直接拼接未转义的用户输入。
- 新写接口必须考虑鉴权、同源校验、请求体上限和限流。
- 不提交密钥、`.env`、数据库转储、真实用户隐私或大体积二进制。
- 保持 390px 手机视口无横向滚动。

## 跑测试

```powershell
$env:GOTOOLCHAIN='local'
go test ./...
go test -race ./...
go test ./... -cover
go vet ./...
gofmt -l .
```

提交前至少保证全量测试、竞态检测、`go vet` 和格式检查通过。涉及 `internal/store` 时，再运行 PostgreSQL 集成测试：

```powershell
docker compose up -d postgres
$env:TEST_DATABASE_URL='postgres://polyphone:polyphone@127.0.0.1:55432/polyphone?sslmode=disable'
go test ./internal/store -count=1 -v
```

未设置 `TEST_DATABASE_URL` 时集成测试会自动跳过；测试使用唯一临时 schema，不会清空开发库。

## 手动验收清单

1. 打开 `http://127.0.0.1:8080/?demo=1`，应检出 28 处。
2. 页面应停在顶部，不应自动滚到编辑框。
3. 确认/忽略/全部采用建议/重置选择均可用。
4. 标注稿、腾讯云 SSML、百度 SSML 可切换并复制。
5. 词库搜索“重庆”返回 1 条。
6. 登录、提交案例、管理员审核、公开案例显示正常。
7. `/lite/` 可独立加载，`?demo=1` 可检出 28 处。
8. 390px 手机视口无横向滚动。

## 提交 PR

- 一个 PR 尽量只做一件事：加词、修 bug、改文档分开。
- 词库 PR 请在描述里列出新增词条和依据。
- 不要提交音频、视频大文件；用链接或 Issue 附件。
- 不要修改已应用的迁移文件；新增迁移请使用下一个递增编号。
- 不要写死 API key、数据库密码或 Supabase `service_role` key。

## 许可证

提交贡献即表示同意按项目许可证发布：

- 代码：MIT
- 词库和案例：CC BY 4.0

第三方模型、字体、音频和 TTS 服务遵循各自许可证。
