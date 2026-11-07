# 安全与权限修复说明（2026-11）

对应评审：`docs/system-review-engineering-2026-11.md`（P0/P1 安全类条目）。
本轮范围：**只改代码与配置示例**。未重建镜像、未重启容器、未写数据库、未推送仓库、未新增迁移。

---

## 1. 权限收紧（P0-3）

### 1.1 路由层统一规则

新增/启用以下中间件（`server/internal/auth/rbac.go`）：

| 中间件 | 允许角色 | 用途 |
|---|---|---|
| `RequirePlatformBoard` | platformadmin / commander / itlead | 项目、剧本、内容写操作 |
| `RequireScenariosWrite` | platformadmin / commander / itlead | 场景增改 |
| `RequireScenariosAdmin` | platformadmin / commander / itlead | 脚本节点保存 |
| `RequireCommanderOrAdmin` | platformadmin / commander | 任务全量重置 |
| `RequireNotExternalWrite` | 所有内部角色；GET/HEAD/OPTIONS 放行 | 外部单位只读 |

`RequireNotExternalWrite` 挂载在 `content`、`tasks`、`evidence`、`drill` 四个业务分组上：

- 卫健局、网信办、公安网安等 `external=true` 角色，对这四个分组下**任何非 GET 请求**均返回 403（“外部单位仅可查看医院上报材料（分级权限隔离）”）。
- 覆盖改分、启停任务、删任务、项目状态变更、会商消息、附件上传、处置记录等全部写入口，不再依赖前端隐藏按钮。

### 1.2 14 个写接口逐项收紧

| # | 方法与路径 | 路由层 | Handler 层二次校验 |
|---|---|---|---|
| 1 | `POST /api/tasks/:id/accept` | `RequireNotExternalWrite` | `guardTaskWrite`：登录 + 非外部 + 任务存在 + 项目未归档 + 角色可处置 |
| 2 | `POST /api/tasks/:id/submit` | 同上 | `guardTaskWrite`；已完成任务返回 409 |
| 3 | `POST /api/tasks/:id/complete` | 同上 | `guardTaskWrite` |
| 4 | `DELETE /api/tasks/:id` | 同上 | `guardTaskWrite`，并写语义化审计 |
| 5 | `POST /api/tasks/reset-all` | `RequireCommanderOrAdmin` | 仅 platformadmin/commander；跳过已归档项目；写审计 |
| 6 | `POST /api/tasks/:id/forms` | `RequireNotExternalWrite` | `guardTaskWrite` |
| 7 | `POST /api/projects/:id/activate` | `RequirePlatformBoard` | `ensureBoardWrite`；已归档项目 409 |
| 8 | `PATCH /api/projects/:id/status` | `RequirePlatformBoard` | `ensureBoardWrite` + 状态白名单（见 1.3）+ 审计 |
| 9 | `PATCH /api/content/projects/:id/members` | `RequirePlatformBoard` | `ensureBoardWrite`；项目不存在 404；已归档 409 |
| 10 | `PATCH /api/content/impact/:sys` | `RequirePlatformBoard` | `ensureBoardWrite`；必须存在当前演练项目；`pct` 支持 0 值；系统不存在 404 |
| 11 | `POST /api/content/opinion` | `RequirePlatformBoard` | `ensureBoardWrite`；标题必填 |
| 12 | `PATCH /api/content/opinion/:id` | `RequirePlatformBoard` | `ensureBoardWrite`；修复原 SQL 未传参问题；不存在 404 |
| 13 | `POST /api/content/seqflow` | `RequirePlatformBoard` | `ensureBoardWrite` |
| 14 | `POST /api/content/snapshots` | `RequirePlatformBoard` | `ensureBoardWrite` |
| 15 | `POST /api/content/audit` | 保留路径 | 不再写库（见第 4 节） |

同时补齐的相邻写入口：

- `POST /api/content/evaluation/scores`（改分）：`RequirePlatformBoard`。
- `POST/PATCH/DELETE /api/content/scenarios|knowledge|projects|script-nodes|notify`：按平台统筹角色收紧。
- `POST /api/content/messages`：`RequireMessagePublisher`（commander / itlead / platformadmin）。

### 1.3 项目状态流转白名单

`PatchProjectStatus` 仅允许：

```
未开始 → 演练中
演练中 → 暂停 | 已结束
暂停   → 演练中 | 已结束
已结束 → 已归档
同状态 → 幂等放行
```

非法流转返回 409，并给出“不允许从「X」变更为「Y」”。已归档项目不可再激活、不可改成员、不可改影响面。

---

## 2. 初始口令与强制改密闭环（P0-4）

### 2.1 初始口令

- 管理员新建账号、重置密码：`server/internal/handler/admin.go` 一律调用 `auth.GenerateTempPassword()` 生成 **14 位随机口令**（至少含字母 + 数字），忽略客户端提交的密码。
- 明文只在本次响应 `initialPassword` 字段返回一次；`admin.html` 用弹窗展示并提示“仅显示一次”。
- 安装期：一键安装器为 `admin` / `platformadmin` **各生成独立随机口令**，写入部署 `.env`（Unix `600`，Windows `icacls` 仅当前用户），并在安装输出中打印一次；不使用统一口令。
- 首次部署 seed：优先读取 `ADMIN_INIT_PASSWORD` / `PLATFORMADMIN_INIT_PASSWORD`；缺失、仍是 `__REPLACE_WITH__` 占位符或强度不足时，才随机生成并写入启动日志（手工部署兜底路径）。
- 改密成功后：服务端把部署 `.env` 中对应变量置空并追加“已失效”注释（容器内路径 `DRILL_ENV_FILE_PATH`）；写失败只记录告警，不影响改密。
- 已删除 `DRILL_INIT_PWD` / `CHANGE_ME` 固定初始口令逻辑。

### 2.2 强制改密状态机

```
登录
  ├─ must_change=0 → 签发正式 token（scope 为空）
  │                   + 下发 /uploads 媒体 Cookie
  │                   → 正常页面
  │
  └─ must_change=1 → 仅签发 scope=password_change 的受限 token
                     + 不下发 /uploads 媒体 Cookie
                     → 前端跳转 password.html
                        │
                        ├─ 访问 /api/auth/change-password、/api/auth/me、
                        │  /api/auth/logout、/api/health 之外接口 → 403
                        │  {code:"PASSWORD_CHANGE_REQUIRED"}
                        │
                        └─ 改密成功
                           ├─ 校验原密码
                           ├─ 校验新密码：≥8 位，含字母 + 数字
                           ├─ 更新密码，must_change=0
                           ├─ 删除该用户全部旧 sessions（含当前受限会话）
                           ├─ 签发正式 token + 下发媒体 Cookie
                           └─ 前端 DRILL.setToken(新 token) → 进入工作台
```

关键实现：

- JWT claims 新增 `scp` 作用域；`RequireAuth` 同时检查 `must_change` 与 `scope`，即使旧 token 没有 scope，只要库里 `must_change=1` 也会被拦截。
- `OptionalAuth`（大屏匿名）不会把受限 token 注入为登录用户，按匿名视图处理。
- `password.html` 不再调用 `/api/bootstrap`（受限会话会 403），只依赖本地会话中的用户/角色渲染改密表单。

---

## 3. `/uploads` 附件目录鉴权（P1-10）

- 删除 `r.Static("/uploads", uploadDir)`，改为 `GET/HEAD /uploads/*filepath` + `uploadAuth` 中间件。
- 鉴权来源（按顺序）：
  1. `Authorization: Bearer ...`
  2. `X-Drill-Token`
  3. `?t=...`
  4. 登录/改密时下发的 `drill_media` HttpOnly Cookie（`Path=/uploads`、`SameSite=Lax`、HTTPS 下 `Secure`）
- 受限会话（待改密）不允许读取附件。
- 文件服务拒绝目录、隐藏文件、`..` 路径穿越；响应 `Cache-Control: private, no-store`，避免鉴权内容被代理缓存。
- 兼容性：浏览器 `<img src="/uploads/...">`、`<a download>`、报告内引用会自动携带同源 Cookie，工作台附件预览与报告引用不受影响。
- 退出登录、改密成功后同步刷新/清理媒体 Cookie。

---

## 4. 审计日志字段收敛（P1-6）

- 新增服务端审计中间件 `handler.AuditWrites`（`server/internal/handler/audit.go`）：
  - 对所有非只读 `/api` 请求、以及 `/uploads/*` 读取，在响应后写入 `audit_logs`。
  - `action` 由服务端拼接为 `METHOD PATH [status]`；`user` / `role_id` / `user_id` / `ip` 全部来自认证上下文或连接信息。
  - 不读取、不存储请求体，客户端无法注入自由文本。
- 关键业务 handler 通过 `writeAudit` 写入语义化动作，例如：
  - 登录成功/失败、修改密码
  - 项目状态变更、设为当前演练项目、更新成员、更新影响面
  - 删除任务、重置全部任务、更新舆情
- `POST /api/content/audit` 保留路径以兼容旧前端，但**不再写库**，客户端提交的 `action` 被直接丢弃。
- 审计写入失败只记录 `slog.Warn`，不影响业务响应。

---

## 5. 登录页与大屏匿名视图

### 5.1 登录页

- `GET /api/auth/accounts` 不再匿名开放，改为 `RequireAuth + RequireAdmin`，仅供 `admin.html` 维护用户时使用。
- `index.html` 删除账号下拉选择器，改为**手动输入账号**；不再匿名获取姓名/单位/角色。
- 登录成功后若 `user.mustChange=true`，立即跳转 `password.html`。
- 角色卡保留为可选筛选；不选角色时按账号主角色登录。

### 5.2 大屏匿名脱敏

- 大屏匿名接口、免登录只读接口保留。
- `screen_public.go` / `monitor_content.go` 在匿名请求中：
  - 时间线 `actor` 替换为角色名；`action` / `detail` 中的已知人员姓名替换为角色名。
  - 会商消息发送者显示为“院内会商·角色名”。
  - 通知标题/描述/来源中的已知人员姓名替换为角色名。
- 时间、动作、状态、级别、时间线顺序全部保留；登录态用户看到的仍是真实姓名。

---

## 6. 其他安全类 P1

| 条目 | 处理 |
|---|---|
| MySQL 3306 对外暴露 | `deploy/docker-dist`、`deploy/linux`、`deploy/docker-compose.yml` 端口改为 `127.0.0.1:...:3306`；容器内 app 仍通过 `mysql:3306` 访问。smoke 的 13306 同样绑定回环。 |
| CORS 默认 `*` | `config.go` 默认 `ALLOW_ORIGINS` 为空；仅显式配置来源时发送 CORS 头；通配模式不再发送 `Allow-Credentials`。`.env` 与示例文件已改为留空。 |
| 验证码无限获取 | `/api/auth/captcha` 单独限流（默认 20/分钟/IP，`RATE_LIMIT_CAPTCHA`）；验证码存储上限 10000，过期/超限自动淘汰；验证码绑定客户端 IP。 |
| 登录锁定 DoS | 失败锁定改为**账号 + IP** 内存双维度计数；攻击者只能锁定自己 IP 下的目标账号，不再能从任意地址锁死他人账号。 |
| 登录审计缺失 | 登录成功、密码错误、验证码错误、账号停用、角色不匹配、锁定均写入审计。 |

> 说明：验证码仍是 SVG 文本算术题，答案对能解析 SVG 的脚本可见；本轮按要求先做限流与内存上限，长期建议替换为图片验证码。

---

## 7. 变更文件

后端：

- `server/internal/auth/auth.go`：受限 token、作用域、随机口令、认证闭环
- `server/internal/auth/rbac.go`：`RequireNotExternalWrite`、`RequireCommanderOrAdmin`
- `server/internal/config/config.go`：CORS 默认关闭、验证码限流配置
- `server/internal/middleware/middleware.go`：CORS 响应头收敛
- `server/internal/router/router.go`：路由层角色收紧、`/uploads` 鉴权、审计中间件挂载
- `server/internal/handler/auth.go`：登录、登出、媒体 Cookie、登录审计
- `server/internal/handler/auth_extra.go`：改密成功签发正式 token、吊销旧会话
- `server/internal/handler/admin.go`：随机初始口令、一次性返回
- `server/internal/handler/captcha.go`：验证码上限、IP 绑定
- `server/internal/handler/missing_handlers.go`：项目/任务/内容写接口 handler 层守门、状态白名单、审计收敛
- `server/internal/handler/audit.go`（新增）：服务端审计中间件与写入辅助
- `server/internal/handler/login_guard.go`（新增）：账号 + IP 登录锁定
- `server/internal/handler/monitor_content.go`、`screen_public.go`：匿名大屏姓名脱敏
- `server/cmd/seed/main.go`：管理员账号随机初始口令、一次性日志展示

前端：

- `public/index.html`：手输账号、移除匿名账号列表、首登跳转改密
- `public/password.html`：改密成功使用新 token 直接进入工作台
- `public/app.js`：改密页不加载 bootstrap，避免受限会话卡在加载失败
- `public/admin.html`：创建/重置后弹窗展示一次性初始口令

配置/部署：

- `deploy/docker-dist/docker-compose.yml`、`deploy/linux/docker-compose.yml`、`deploy/docker-compose.yml`、`deploy/docker-dist/docker-compose.smoke.yml`：MySQL 端口仅回环
- `deploy/docker-dist/docker-compose.yml`：新增 `${DRILL_ENV_FILE:-.env}:/opt/drill/etc/drill.env` 挂载与 `DRILL_ENV_FILE_PATH`，供首次改密后清空初始口令
- `deploy/docker-dist/docker-entrypoint.sh`：首启错误提示改为“检查数据库/密钥与管理员初始口令”
- `.env`、`.env.example`、`deploy/docker-dist/.env.example`、`deploy/linux/drill.env.example`：CORS 默认留空、验证码限流、管理员初始口令模板
- `install/install-linux.sh`、`install/install-windows.ps1`：安装期生成两个管理员账号的独立随机口令，写入 `.env` 并打印一次；收紧文件权限
- `server/cmd/seed/main.go`：优先使用安装器写入的初始口令；`server/internal/handler/initial_password.go`：改密后清空 `.env` 明文
- `README.md`、`docs/install.md`、`docs/docker-image.md`：首次登录与交付说明同步

未改动（遵守并行开发约束）：

- `missing_handlers.go` 的取证/上传/备份实现（仅在 `PostTaskSubmit` 顶部增加权限守门，未改取证字段与上传逻辑）
- `drill_v1.go`、`drill_score.go`
- `public/api.js`、`public/screen.html`
- `script_nodes_write.go`
- 未新增数据库迁移

---

## 8. 验证方法

### 8.1 静态检查（本轮已执行）

```powershell
cd server
go vet ./...        # 无输出
node --check public/app.js
# index.html / password.html / admin.html 内联脚本提取后 node --check 通过
```

### 8.2 权限验证用例

准备三类账号：平台管理员（platformadmin）、院内普通角色（如 duty）、外部只读角色（healthbureau）。

1. 用外部角色登录，依次调用：
   - `POST /api/tasks/:id/accept`
   - `POST /api/tasks/:id/complete`
   - `DELETE /api/tasks/:id`
   - `POST /api/tasks/reset-all`
   - `PATCH /api/projects/:id/status`
   - `POST /api/content/evaluation/scores`
   - `POST /api/content/chat/inner` 等写接口
   期望均为 403。
2. 用院内普通角色调用项目状态变更、影响面更新、快照、舆情增改，期望 403（非平台统筹角色）。
3. 用平台管理员执行合法流转 `未开始→演练中→暂停→演练中→已结束→已归档`，期望成功；再尝试 `已归档→演练中`，期望 409。
4. 不带 token 访问 `/uploads/<任意已存在文件>`，期望 401；登录后浏览器直接打开同一 URL，期望 200。

### 8.3 强制改密验证

1. 管理员新建账号，记录响应中的 `initialPassword`。
2. 用该账号登录，响应 `user.mustChange=true`，前端跳转 `password.html`。
3. 用受限 token 请求 `/api/bootstrap`、`/api/content/monitor`、`/uploads/...`，期望 403/401。
4. 修改密码后：
   - 响应包含新 `token`；
   - 旧 token 立即失效（`sessions` 已删除）；
   - 新 token 可正常访问业务接口；
   - `users.must_change=0`。

### 8.4 审计验证

1. 执行项目状态变更、删除任务、修改密码、下载附件。
2. 查询 `audit_logs`：
   - `user` / `role_id` / `user_id` / `ip` / `action` 均为服务端生成；
   - 不再出现客户端提交的任意自由文本。
3. 调用 `POST /api/content/audit`，期望返回 `{"ok":true,"ignored":true}`，且 `audit_logs` 不新增该客户端文本记录。

### 8.5 大屏匿名验证

1. 退出登录（或使用无痕窗口）打开 `screen.html`。
2. 时间线仍显示时间与动作，但操作人显示为角色名（如“信息科值班”），不出现真实姓名。
3. 登录后打开同一大屏，显示真实姓名。

---

## 9. 0.7.1 缺陷修复：同秒登录 500 与静态资源权限

### 9.1 JWT `jti`

**根因**：同一用户同一秒内两次签发时，`iat` / `exp` / `sub` / `scp` 完全相同，JWT 字符串逐字节一致；`sessions.token` 是主键，第二次 INSERT 冲突，接口返回 500「会话创建失败」。改密成功后立刻重登正好落在同一秒。

**方案**：签发时加入 128 位随机 `jti`（`crypto/rand` → 32 位十六进制，`server/internal/auth/auth.go`）。

选择理由：
- 同一用户同一秒多次登录会自然形成多条独立会话，登出/吊销单个会话仍是精确的 `DELETE ... WHERE token=?`，不会互相误杀。
- 不做 upsert：upsert 会让同秒两次登录共用一行，任一端登出会误杀另一端，也模糊了“每次签发即一条会话”的语义。
- `jti` 每次重新生成，任一令牌不会被重复签发复用；改密后的 `DELETE FROM sessions WHERE user_id=?` 与受限令牌逻辑不变；旧令牌（无 `jti`）仍可正常解析，支持滚动升级。
- `sessions.token` 为 `VARCHAR(512)`，新增约 50 字符无需迁移。

### 9.2 验证方法（VM / 隔离环境，勿在现网执行）

前置：隔离环境临时设置 `CAPTCHA_ENABLED=false` 并重启**测试实例**；准备可登录账号（下文示例 `duty01`）。以下命令需 `jq`（或改用 Python 解析 JSON）。

1. 同秒连续登录两次：

```bash
for i in 1 2; do
  curl -sS -X POST "http://127.0.0.1:8090/api/auth/login" \
    -H "Content-Type: application/json" \
    -d '{"username":"duty01","password":"<pass>"}' | tee "/tmp/login$i.json"; echo
done
```

期望：两次均 200，两个 `token` 互不相同；服务端日志无 `sessions` 主键冲突。修复前第二次为 500「会话创建失败」。

2. 改密后立刻重登两次：

```bash
OLD=$(curl -sS -X POST "http://127.0.0.1:8090/api/auth/login" \
  -H "Content-Type: application/json" \
  -d '{"username":"duty01","password":"<old>"}' | jq -r .token)

NEW=$(curl -sS -X POST "http://127.0.0.1:8090/api/auth/change-password" \
  -H "Authorization: Bearer $OLD" -H "Content-Type: application/json" \
  -d '{"oldPassword":"<old>","newPassword":"<new>"}' | jq -r .token)

for i in 1 2; do
  curl -sS -X POST "http://127.0.0.1:8090/api/auth/login" \
    -H "Content-Type: application/json" \
    -d '{"username":"duty01","password":"<new>"}' | jq -r .token
done
```

期望：`$NEW` 与随后两次登录的 token 三者互不相同且均可用；旧密码登录 401；改密前的 `$OLD` 访问业务接口返回 401（会话已吊销）。

3. 会话表核对（只读）：

```sql
SELECT id, token, created_at FROM sessions WHERE user_id=<id> ORDER BY id DESC LIMIT 10;
SELECT COUNT(*), COUNT(DISTINCT token) FROM sessions WHERE user_id=<id>;
```

期望：两个计数相等，同秒多条记录 token 各不相同。

4. 静态资源权限（VM 上只读检查）：

```bash
cd /home/drill/drill-deploy/bundle
find public -type d ! -perm -755 -print   # 应无输出
find public -type f ! -perm -644 -print   # 应无输出
curl -s -o /dev/null -w '%{http_code}\n' http://127.0.0.1:8090/index.html  # 200
```

### 9.3 静态资源权限固化

- `scripts/sync_public_core.py`：上传完成、去 BOM 后统一执行 `find . -type d -exec chmod 755 {} +` 与 `find . -type f -exec chmod 644 {} +`，不再依赖远端 umask。
- `deploy/linux/install.sh`：入口固定 `umask 022`，并在 `cp -ru public` 后再次显式归一化，避免严格 umask 或源文件 640 导致容器 uid 10001 读不到。

---

## 10. 上线注意

- 本轮未重启容器；以上代码与配置需在下次构建/重启后生效。
- 管理员初始口令：一键安装器会在安装输出打印一次并写入 `.env`，无需再翻启动日志；手工部署未配置时才回退为“启动日志打印一次”。首次登录必须改密，改密后 `.env` 中的值会被自动清空。
- 交付客户前建议先完成首次改密并确认 `.env` 中 `ADMIN_INIT_PASSWORD` / `PLATFORMADMIN_INIT_PASSWORD` 为空；该文件仍含数据库密码与密钥，必须通过安全通道交付并提醒离线备份。
- 旧部署升级：若 `.env` 归属为 root 且容器用户无法回写，安装器会自动尝试 `chown 10001:10001`；手工部署请自行执行，否则改密后只记录告警。
- 若现网 `.env` 仍配置 `ALLOW_ORIGINS=*`，请改为留空或明确来源；本轮已将工作区 `.env` 改为留空。
