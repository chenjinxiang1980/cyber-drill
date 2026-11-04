# 应急演练指挥系统（Cyber-Drill-System）

面向医院 / 企事业单位内网的 **桌面推演 + 实战演练** 指挥调度平台。内置 8 类高频安全事件剧本（勒索病毒、横向渗透、DDoS、钓鱼邮件、存储故障等），支持值班员、指挥员、管理员等多角色协同、剧本时序控制、处置步骤记录、1920×1080 监视大屏、自动考评与复盘报告。

> 当前版本：**0.5.0**（Go 版首发）
> 许可证：MIT © 2026 陈金祥

---

## 一、为什么重构为 Go（最重要的一节）

### 1.1 问题：纯 JS + HTML 的源码一旦公开，等于把底层业务规则一起交出去

本系统早期采用 “Node.js + 纯静态 HTML/JS” 实现，所有业务逻辑——包括剧本节点编排、评分口径、混淆节点的门控时机、任务下发条件、演练计时基准、站内消息播报规则——都写在可直接阅读、可直接修改的前端脚本里。

源码公开后，任何拿到仓库的人都可以：

- 直接读到每一分的评分公式与扣分规则，**按规则反向构造“满分操作”**；
- 在浏览器里改掉 `app.js` / `scenarios.html` 里的事件处理，**跳过 `before/after` 门控**，让混淆节点提前或延后触发；
- 伪造任务完成请求、绕过“仅演练中才下发任务”的项目状态门控；
- 改写计时起算点，让演练时长与处置时效失真；
- 改掉播报逻辑，让关键告警不播、重复播或只播标题。

换句话说：**前端可读 = 规则可读；前端可改 = 规则可绕过。** 对一套用于考核与复盘的演练系统，这比“代码被抄”更严重——它直接摧毁评分与流程的可信度。

### 1.2 方案：把规则收进服务端编译产物，前端只保留展示层

0.5.0 起，后端整体重构为 **Go（Gin）** 单体服务：

- 剧本编排、节点门控（`before` / `after`）、任务下发状态门控、计时基准（`projects.started_at`）、评分与扣分、消息播报过滤等**规则全部下沉到服务端**；
- 前端只负责渲染与服务端返回结果的展示，**不再持有判定逻辑**；
- 服务端以 Linux ELF / Docker 镜像交付，源码公开也不等于规则可被前端篡改或绕过；
- 同时保留 `node/` 目录作为**历史实现与行为基准**（仅用于对照，不参与部署）。

### 1.3 重构前 / 重构后对比

| 维度 | 重构前（Node + 纯前端 JS） | 重构后（Go 服务端 + 静态前端） |
|------|--------------------------|------------------------------|
| 规则位置 | 剧本编排、评分、门控、计时、播报均在前端脚本 | 全部在服务端编译产物内，前端只展示 |
| 前端可否篡改规则 | 可直接改 JS / 改请求绕过 | 改前端只影响显示，判定仍由服务端执行 |
| 可否绕过评分 | 可读公式、可伪造节点完成 | 评分与节点状态由服务端校验，前端无法自证 |
| 任务下发门控 | 前端按项目状态自行判断 | 服务端按项目状态强制门控 |
| 计时基准 | 前端本地计时，易被改写 | 服务端以 `projects.started_at` 为唯一基准 |
| 部署形态 | Node 运行时 + 依赖树 | 单个 Linux ELF 或 Docker 镜像，零运行时依赖 |

> 结论：**开源的是“系统”，不是“评分答案”。** 规则在服务端，源码公开不会暴露与绕过核心判定。

---

## 二、功能清单

- **演练监视大屏**：1920×1080 大屏，实时操作流、参演角色处置状态、告警与消息三栏滚动，自动重排避免末位错位。
- **角色工作台**：值班员 / 指挥员 / 管理员等角色工作台，处置弹窗、任务清单、业务系统影响态势。
- **剧本编排**：节点化剧本编辑、`before` / `after` 混淆门控、版本号策略（大版本 / 小版本 / 不更新）。
- **演练项目管理**：项目状态机（未开始 / 演练中 / 已暂停 / 已结束等）、基础版 / 实战版、参演单位与观察员。
- **复盘与考评**：按节点时序自动评分、扣分项说明、演练报告导出、时序回放。
- **站内消息**：按项目隔离的站内消息、未读计数、可选的语音播报（按消息 id 去重、只播未读、只播当前项目、只念正文）。
- **会商频道**：演练期间的多角色会商与消息留痕。
- **取证与台账**：处置证据归档、操作台账、知识库与字典。
- **通知与公告**：全局公告与项目消息分流，公告不触发语音播报。

---

## 三、架构与技术栈

```
┌──────────────────────────────────────────────────────┐
│ 浏览器（纯静态 HTML / CSS / JS，无构建步骤）              │
│ index · workbench · screen(1920×1080) · admin · ...   │
│                    REST / JSON                       │
├──────────────────────────────────────────────────────┤
│ Go 后端（Gin）  server/main.go                        │
│ JWT + Session · RBAC · 限流 · 安全头 · 防注入 · 静态托管  │
├──────────────────────────────────────────────────────┤
│ MySQL 8.x（utf8mb4，单库多表）                          │
└──────────────────────────────────────────────────────┘
```

- **后端**：Go 1.22+（当前在 go1.27 工具链下构建），Gin、`database/sql` + MySQL driver；单文件编译，零外部运行时。
- **前端**：原生 HTML + CSS + JavaScript，无打包步骤；由 Go 服务直接托管 `public/`。
- **数据库**：MySQL 8.x，utf8mb4；迁移脚本在 `migrations/`。
- **大屏适配**：1920×1080 基准 + 缩放，手机端响应式布局。

---

## 四、目录结构

```
Emergency/
├── server/                  # Go 后端（当前生产实现）
│   ├── main.go              # 入口：配置、MySQL、路由、HTTP(S)、优雅关闭
│   ├── internal/
│   │   ├── auth/            # JWT + Session + RBAC
│   │   ├── config/          # .env 加载与必填校验
│   │   ├── db/              # MySQL 连接池
│   │   ├── handler/         # /api/* 业务 handler
│   │   ├── middleware/      # 安全头 / CORS / 限流 / BodyLimit
│   │   ├── router/          # 路由组装与安全静态文件服务
│   │   ├── util/
│   │   └── version/         # 版本号（0.5.0）
│   ├── cmd/                 # bootstrap / migrate / verify_* 工具
│   ├── winres/              # Windows 资源清单
│   └── deploy-linux/        # Linux 构建产物与签名文本（二进制不入库）
├── node/                    # 历史 Node 实现（行为基准，不参与部署）
├── public/                  # 前端静态页面（不构建，直接托管）
├── deploy/
│   ├── docker-dist/         # 多阶段 Docker 构建（推荐）
│   ├── linux/               # systemd / compose / nginx / caddy
│   └── docker-desktop/      # 本机开发用 compose
├── migrations/              # 0001_init / 0002_seed_impact / 0003_login_lock
├── scripts/                 # 构建、打包、启动、发版辅助脚本
├── docs/                    # 安全部署指南、Linux / VMware 部署、发版说明
├── .env.example             # 环境变量模板（只有占位符）
├── LICENSE                  # MIT（版权人：陈金祥）
└── README.md
```

> `server/deploy-linux/drill-server` 为交叉编译产物，**不进入仓库**；仓库只保留签名文本 `drill-server.sha256.txt`。

---

## 五、快速开始

### 5.1 前置

- Go 1.22+（或直接使用 Docker 方式，无需本机 Go）
- MySQL 8.x（Docker 方式由 compose 自带）

### 5.2 方式一：本地 / Windows 服务器

```powershell
git clone https://github.com/chenjinxiang1980/cyber-drill.git Emergency
cd Emergency

# 1) 准备配置（模板只含占位符）
Copy-Item .env.example .env
#    编辑 .env，替换所有 __REPLACE_WITH__：
#      DB_USER / DB_PASSWORD
#      JWT_SECRET / SESSION_SECRET   （openssl rand -hex 32）
#      ADMIN_INIT_PASSWORD           （首次部署管理员密码）

# 2) 初始化数据库（一次性）
go run ./cmd/bootstrap -root=..

# 3) 启动
powershell -ExecutionPolicy Bypass -File scripts\start.ps1 -Detach

# 4) 健康检查
curl http://127.0.0.1:8090/api/health
# {"ok":true,"service":"cyber-drill-system","version":"0.5.0","time":"..."}
```

### 5.3 方式二：Docker Compose（推荐）

```bash
cp deploy/docker-dist/.env.example deploy/docker-dist/.env
# 编辑 .env：DB_PASSWORD / MYSQL_ROOT_PASSWORD / JWT_SECRET /
#            SESSION_SECRET / ADMIN_INIT_PASSWORD 全部替换为真实值

cd deploy/docker-dist
docker compose up -d --build
docker compose run --rm app /opt/drill/bin/drill-server-bootstrap -root=/opt/drill
curl http://127.0.0.1:8090/api/health
```

镜像默认标签为 `drill-server:0.5.0`（可用 `VERSION` 覆盖）。

### 5.4 方式三：Linux 物理机 / 虚拟机（systemd）

```bash
# 在 Windows 上交叉编译
powershell -ExecutionPolicy Bypass -File scripts\build-linux.ps1

# 拷到目标机后安装
scp server/deploy-linux/drill-server deploy/linux/*.sh deploy/linux/*.service user@host:/tmp/
sudo bash /tmp/install.sh
sudo vi /etc/drill/drill.env
sudo systemctl enable --now drill-server
curl http://127.0.0.1:8090/api/health
```

更完整的部署前检查表、HTTPS、代理、国产化基础镜像说明见：

- [docs/linux-deployment.md](docs/linux-deployment.md)
- [docs/security-guide.md](docs/security-guide.md)
- [docs/vmware-vm-deployment.md](docs/vmware-vm-deployment.md)
- [docs/docker-image.md](docs/docker-image.md)

---

## 六、安全默认值与首次部署必改项

**仓库不预置任何可用凭证。** `.env.example` 中所有敏感项均为 `__REPLACE_WITH__` 占位符；服务启动时严格校验，缺失直接 `exit 2`，不静默降级。

| 必改项 | 说明 | 设置方式 |
|--------|------|----------|
| `ADMIN_INIT_PASSWORD` | 首次 bootstrap 写入的管理员密码 | 自行设置强口令，**不要使用示例值** |
| `JWT_SECRET` | JWT 签名密钥 | `openssl rand -hex 32` |
| `SESSION_SECRET` | Session 签名密钥 | `openssl rand -hex 32` |
| `DB_PASSWORD` / `MYSQL_ROOT_PASSWORD` | 数据库账号口令 | 自行设置强口令 |
| `CAPTCHA_ENABLED` | 登录图形验证码 | 公网部署保持 `true` |
| `ALLOW_ORIGINS` | CORS 白名单 | 生产改为明确域名，勿留 `*` |
| `TLS_ENABLED` + 证书 | HTTPS | 公网部署必须开启并挂载真实证书 |

内置安全能力：

- **鉴权 / RBAC**：JWT + 服务端 Session 校验，角色分组鉴权（`server/internal/auth`）。
- **登录保护**：图形验证码（一次性、TTL 可配）+ 账号失败锁定 + 登录分组限流。
- **限流**：全局限流与 `/api/auth` 分组限流（`golang.org/x/time/rate`）。
- **防注入 / XSS / 越权**：参数化 SQL、输出转义、服务端角色校验；静态文件服务拒绝 `..` 与隐藏文件。
- **安全响应头 / BodyLimit**：`middleware.SecurityHeaders`、请求体大小上限。
- **默认零凭证**：未配置真实密钥时拒绝启动。

> 本项目不含 WebSocket 通道；实时刷新由前端轮询 REST 接口完成，因此不存在独立 WS 鉴权面。若后续引入 WS，需复用同一 JWT/Session 校验。

---

## 七、免责声明与合规使用

1. 本项目仅用于**授权的网络安全应急演练、教学与内部培训**。请勿用于未经授权的攻击、探测或任何违法用途。
2. 演练剧本中的攻击描述、节点与话术均为**模拟内容**，不构成真实攻击工具；使用者应确保演练环境与生产环境隔离。
3. 部署方对 `.env`、数据库、证书、账号口令的保管负全部责任；因配置不当（弱口令、公网暴露、关闭验证码等）造成的损失由部署方承担。
4. 本项目按 MIT 许可证“按原样”提供，作者不对可用性、适用性或任何直接/间接损失作出担保。
5. 使用本项目即表示你已阅读并同意上述条款，并遵守所在国家/地区的法律法规。

---

## 八、版本与更新日志（0.5.0）

**新增**

- Go 版服务端首发：规则、评分、门控、计时全部下沉服务端。
- `/api/health` 增加 `version` 字段；Docker 构建注入 `internal/version`。
- Linux 离线部署包与 SHA256 验签流程（`scripts/pack-linux-bundle.ps1`）。

**修复**

- 演练计时基准统一为项目开始时刻 `projects.started_at`，大屏 / 看板 / 工作台一致重算；启动演练真正落库。
- 监视大屏三处滚动模块末位错位修复。
- 角色工作台处置弹窗仅允许“提交成功自动关闭”和“× 关闭”。
- 基础版任务按项目状态门控，仅“演练中”下发。
- 站内消息语音播报：按消息 id 去重、只播未读、只播当前项目、全局公告不播、只念正文。
- 角色工作台“业务系统影响态势”重复行合并去重。
- 打包脚本改为读取签名文件末行哈希；`build-linux.ps1` 构建后自动重签。
- 删除项目时清理关联数据。

**安全**

- 仓库零预置凭证；`.env.example` 全占位符，启动强校验。
- `.gitignore` 覆盖 `.env*`、证书私钥、构建二进制、归档包、备份、日志与临时目录。
- mysqldump / mysql CLI 调用改用 `MYSQL_PWD` 环境变量，避免口令出现在进程命令行。
- 静态文件服务路径穿越与隐藏文件防护。
- 文档与脚本中的内网地址、真实账号信息已脱敏；发布二进制不入库。

完整说明见 [docs/release-0.5.0.md](docs/release-0.5.0.md)。

---

## 九、License 与致谢

- 许可证：**MIT**，版权人 **陈金祥**，见 [LICENSE](LICENSE)。
- 致谢：感谢所有参与演练脚本设计、内网部署验证与安全评审的同事；感谢 Gin、MySQL 及开源社区。

---

## English Summary

Cyber-Drill-System is an intranet tabletop/field exercise command platform for hospitals and enterprises. In v0.5.0 the backend was rewritten in Go so that **scoring rules, node gating, task gating, timing and notification rules live in the server binary**, not in publicly readable frontend scripts. The frontend remains plain static HTML/CSS/JS and is display-only. Requires Go 1.22+ and MySQL 8.x, or Docker. No credentials are preconfigured. Licensed under MIT © 2026 Chen Jinxiang.
