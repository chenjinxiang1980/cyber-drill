# 安全部署指南

> 本指南覆盖公网部署与内网加固两个场景。所有示例占位符不含真实值，部署时替换。

---

## 一、必须修改的项（启动前）

### 1.1 MySQL 业务账号（最小权限）

**不要用 root 跑生产**。创建专用账号：

```sql
CREATE USER 'drill_app'@'%' IDENTIFIED BY '__STRONG_PASSWORD__';
GRANT SELECT, INSERT, UPDATE, DELETE, CREATE, ALTER, DROP, REFERENCES
    ON drill_system.* TO 'drill_app'@'%';
FLUSH PRIVILEGES;
```

`__STRONG_PASSWORD__` 要求 ≥ 16 字符、含大小写+数字+符号、不含字典词。

### 1.2 JWT 密钥 + Session 密钥

```bash
# Linux / WSL
openssl rand -hex 32

# Windows PowerShell（不需要 openssl）
-join ((1..32) | ForEach-Object { '{0:x2}' -f (Get-Random -Min 0 -Max 256) })
```

生成 64 字符 hex 串，填到 `.env` 的 `JWT_SECRET` 和 `SESSION_SECRET`。

### 1.3 默认账号密码策略（开源红线）

> **仓库不预置任何可用凭证**。管理员初始口令由安装器随机生成；其余账号由服务端随机生成一次性口令后写入数据库，首次登录强制改密。

**密码来源**：
- `.env.example` 中仅出现 `__REPLACE_WITH__` 占位符，不含任何真实口令
- 管理员初始口令由安装器为 `admin` / `platformadmin` **各生成独立随机值**，写入本地 `.env`（不提交仓库）并在安装输出打印一次；手工部署缺失时由 `drill-seed` 随机生成并写入启动日志
- 后台新建/重置账号：服务端随机生成一次性口令，仅在响应中返回一次
- 所有账号 `must_change=1`，首次登录必须改密；未改密前除改密接口外一律拒绝。`DRILL_INIT_PWD` 统一默认口令已移除

**安全要求**：
- 口令必须 ≥ 16 字符，含大小写 + 数字 + 符号，不含字典词
- 任何文档、脚本、示例、迁移文件都**不得出现真实口令明文**
- `scripts\reset_default_passwords.ps1` 从 `-Password` 参数或 `DRILL_DEFAULT_PASSWORD` 环境变量读取口令，缺失则直接报错退出，绝不使用内置默认值

### 1.4 HTTPS 证书

局域网：自签即可。公网：必须 Let's Encrypt 或付费证书。

---

## 二、密钥生成方法

| 密钥 | 长度 | 生成命令 |
|------|------|----------|
| JWT_SECRET | 32 字节 hex（64字符） | `openssl rand -hex 32` |
| SESSION_SECRET | 32 字节 hex | 同上 |
| MySQL 密码 | ≥ 16 字符 | 密码管理器生成 |
| HTTPS 私钥 | EC P256 / RSA 2048 | 见下 |

### 自签证书（局域网用）

```bash
# Linux / WSL
openssl req -x509 -newkey ec -pkeyopt ec_paramgen_curve:prime256v1 \
  -keyout server.key -out server.crt -days 365 \
  -subj "/CN=your-server-hostname" -nodes
```

生成后复制到 `deploy/certs/`，设置 `.env`：
```
TLS_ENABLED=true
TLS_ADDR=:8443
HTTPS_CERT_FILE=deploy/certs/server.crt
HTTPS_KEY_FILE=deploy/certs/server.key
HTTP_REDIRECT_TO_HTTPS=true
```

### Let's Encrypt（公网用）

```bash
# certbot（nginx / standalone）
certbot certonly --standalone -d drill.example.com -d www.drill.example.com \
  --agree-tos --email ops@example.com --cert-path /etc/letsencrypt
```

---

## 三、HTTPS 强制建议

应用层已支持 TLS（`TLS_ENABLED=true` + `HTTP_REDIRECT_TO_HTTPS=true`），但**强烈建议同时在反向代理层加 HSTS**：

### Nginx

```nginx
server {
    listen 80;
    server_name drill.example.com;
    return 301 https://$host$request_uri;
}

server {
    listen 443 ssl http2;
    server_name drill.example.com;

    ssl_certificate     /etc/letsencrypt/live/drill.example.com/fullchain.pem;
    ssl_certificate_key /etc/letsencrypt/live/drill.example.com/privkey.pem;
    ssl_protocols       TLSv1.2 TLSv1.3;
    ssl_ciphers         HIGH:!aNULL:!MD5;

    add_header Strict-Transport-Security "max-age=31536000; includeSubDomains" always;
    add_header X-Content-Type-Options "nosniff" always;
    add_header X-Frame-Options "SAMEORIGIN" always;

    location / {
        proxy_pass http://127.0.0.1:8090;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto https;
    }
}
```

应用侧 `.env` 设置：
```
ALLOW_ORIGINS=https://drill.example.com
TRUSTED_PROXIES=127.0.0.1/32
PUBLIC_BASE_URL=https://drill.example.com
```

---

## 四、公网暴露风险清单

### 当前系统的已知短板（公网部署必须补）

| 风险点 | 严重度 | 说明 | 缓解 |
|--------|--------|------|------|
| 登录暴力破解 | 低（已部分缓解） | 应用层已内置图形验证码（默认开启）+ 账号级失败锁定（默认 5 次 / 15 分钟）+ 两级 IP 限流 | 公网部署建议同时在反向代理层加 fail2ban / 登录失败 IP 黑名单；内网自动化测试需关验证码时务必加 IP 白名单 |
| IP 限流（应用层） | 中 | 进程内 token bucket，多实例/NAT 场景有限制 | 见下方"应用层限流详解"；公网部署必须反向代理层加 `limit_req_zone` |
| 初始密码弱 | 低（已缓解） | 仓库不预置任何可用凭证，所有密码由部署方通过 `.env` 设置后写入 | 部署脚本内检查密码是否仍为占位符，未设置拒绝对外开放 |
| `/api/admin/system/reset` | 高 | platformadmin + confirm=RESET 即可清空 | 反向代理层对白名单 IP 放行，或直接禁用该路由 |
| JWT 无设备绑定 | 中 | 泄露后可在任意设备使用 | 加 `jti` + server-side 黑名单 |
| 无 CSRF token | 中 | Cookie 依赖，SPA 受同站保护但需确认 | 加 SameSite=Strict |

### 反向代理层必须加白名单的端点

```
/api/admin/system/reset                    # 仅允许运维 IP
/api/admin/users/*                         # 仅允许运维 IP
/api/admin/users/cleanup-test-accounts     # 仅允许运维 IP（一键清理测试账号接口）
/api/admin/config/*                        # 仅允许运维 IP
/api/auth/login                            # 建议加验证码接口 + 失败计数
```

#### `/api/admin/users/cleanup-test-accounts` 接口安全说明

该接口用于一键删除演练过程中临时创建的测试账号，仅保留基线 12 个账号。

| 安全机制 | 实现方式 |
|----------|----------|
| 角色权限 | 仅 `platformadmin` 可调用 |
| 默认安全 | `dryRun` 默认 `true`，不带 confirm 的请求只返回计划，不执行删除 |
| 执行确认 | `confirm="DELETE-TEST-ACCOUNTS"` 才真正执行 |
| 事务内 | 事务内先删 `user_roles` 再删 `users`，保证外键约束和一致性 |
| 审计日志 | 所有调用写入 `audit_logs`（操作者、时间、影响行数） |
| 白名单保护 | 反向代理层仅放运维 IP，与 `/api/admin/*` 其他端点同策略 |

管理后台将在系统设置页提供对应按钮，**命令行兜底方案**为 `scripts\cleanup_test_accounts.ps1`（默认 dry-run，`-Execute` 才真正删除）。接口和脚本**互为补充**：API 用于日常运维，脚本用于紧急恢复或无 JWT 场景。

---

---

## 五、应用层限流详解

### 架构

应用层使用 `golang.org/x/time/rate` token bucket 实现两级 IP 限流：

```
请求 → SecurityHeaders → CORS → BodyLimit → RateLimit(GLOBAL) → [路由分组] → RateLimitByGroup(AUTH) → handler
                                          ↑ 挂在 root router                ↑ 挂在 POST /api/auth/login
                                          ↑ 覆盖 /public/* /uploads/* /api/* ↑ 独立 store
```

两级 **独立** 的 `rateStore`，各自维护 `map[ip]*rate.Limiter`，互不干扰。算法参数：

- `rps = rpm / 60`（token 恢复速率）
- `burst = rpm`（突发容量）
- bucket 初始状态 burst 满额

### 进程内计数限制

**每个 Go 进程独立计数**。如果部署 N 个 drill-server 实例（负载均衡），每个实例维护独立的 limiter map：

| 实例数 | RATE_LIMIT_GLOBAL | 单 IP 实际总上限 | 说明 |
|--------|------------------|-----------------|------|
| 1      | 300              | 300 req/min     | 单机，正常 |
| 3      | 300              | **900 req/min** | 每个实例 300，独立计数 |
| 5      | 300              | **1500 req/min**| 集群部署的上限被放大 |

如需多实例共享计数，必须引入 Redis 或改用反向代理层限流。

### NAT 场景（医院内网最常见）

所有终端经一个 NAT 网关出口，服务器看到的都是同一个 IP。所有终端共享同一 limiter bucket。

**实例**：35 台终端，每台平均 7 req/min
- 稳态消耗 = 35 × 7 = **245 req/min**（rps ≈ 4.08）
- 首屏一次性额外消耗 = 20 个静态资源（**仅第一波，之后稳态**）
- `RATE_LIMIT_GLOBAL=300`（rps=5.0，burst=300）→ 稳态消耗 245/min < 恢复速率 300/min，**稳态不会耗尽 bucket**；但若 35 台同时首屏加载，35×20=700 个瞬时请求远超 burst=300，会出现大量 429

**建议**：>30 台终端 NAT 出口时，`RATE_LIMIT_GLOBAL` 建议 ≥ **1000**。

### RATE_LIMIT_AUTH 与全局同值的陷阱

当 `RATE_LIMIT_AUTH == RATE_LIMIT_GLOBAL` 时（例如两者都设为 300）：
- 登录请求经过两个参数完全相同的 bucket 串联
- 全局 bucket 先消耗，分组 bucket 再消耗
- 分组 bucket 永远不会在全局之前耗尽
- **分组限流形同虚设**——登录暴力破解的额外保护层消失

**正确配置**：`RATE_LIMIT_AUTH` 必须 **严格小于** `RATE_LIMIT_GLOBAL`（模板默认 30），让分组 bucket 成为瓶颈：

```
登录请求 → [全局 bucket: burst=1000] ✓ → [分组 bucket: burst=30] ✗ 第 31 次 429
```

### 静态资源计入全局限流

`/public/*`（HTML/JS/CSS）和 `/uploads/*` 静态文件也会被 `RateLimit(GLOBAL)` 覆盖。监视大屏 `screen.html` 首屏一次性加载 15-20 个资源，全部消耗全局配额。如需豁免，应在反向代理层（nginx）用 `location /public/` 单独放开限流。

### 反向代理层加固（公网部署必做）

应用层限流是"宽松保护"，公网部署必须在 nginx 前置 `limit_req_zone`：

```nginx
http {
    # 全局：IP 级 10 req/s
    limit_req_zone $binary_remote_addr zone=global:10m rate=10r/s;
    # 登录接口：严格 2 req/s
    limit_req_zone $binary_remote_addr zone=login:10m rate=2r/s;

    server {
        location / {
            limit_req zone=global burst=30 nodelay;
            proxy_pass http://127.0.0.1:8090;
        }
        location /api/auth/login {
            limit_req zone=login burst=5 nodelay;
            proxy_pass http://127.0.0.1:8090;
        }
        location /public/ {
            limit_req zone=global burst=50 nodelay;
            proxy_pass http://127.0.0.1:8090;
        }
    }
}
```

反向代理层限流的优势：
1. **多实例共享计数**——nginx 作为唯一入口，天然解决进程内计数限制
2. **攻击流量在 nginx 层被挡住**——Go 进程不被无效请求占用
3. **可以对静态资源单独配置更宽松的 burst**

---

## 六、备份与恢复

### 自动备份（Linux / WSL）

```bash
#!/bin/bash
# /etc/cron.d/drill-backup
TS=$(date +%Y%m%d-%H%M)
mysqldump -u drill_app -pXXX drill_system > /backup/drill_system-${TS}.sql
find /backup -name "*.sql" -mtime +7 -delete
```

### 恢复流程

```bash
mysql -u drill_app -p drill_system < /backup/drill_system-YYYYMMDD.sql
```

**主数据基线恢复**（以 `data/drill.db` 为真源，仅覆盖漂移的 impact/timeline/projects 行）：

```bash
# 只恢复核心表的模板行和 P2026-001 实例行
mysql -u drill_app -p drill_system < data/drill_system-baseline.sql
```

---

## 七、MySQL 安全加固

### my.cnf

```ini
[mysqld]
# 禁止 root 远程登录
skip-name-resolve
bind-address = 127.0.0.1

# 禁止 LOAD_FILE / SELECT ... INTO OUTFILE（防止任意文件读写）
secure-file-priv = /var/lib/mysql-files/

# 记录所有可疑登录
log_warnings = 2
```

### 应用侧最小权限

```sql
-- drill_app 账号只能操作 drill_system
-- 不要给 PROCESS / FILE / SUPER / RELOAD 等全局权限
GRANT SELECT, INSERT, UPDATE, DELETE, CREATE, ALTER, REFERENCES
    ON drill_system.* TO 'drill_app'@'127.0.0.1';
```

---

## 八、内网部署（推荐）

如果你不打算暴露到公网，**最简单的加固就是不暴露**：

1. 服务器只绑定内网 IP（`0.0.0.0:8090` 保留，或改 `10.0.0.x:8090`）
2. 防火墙只放行 `8090`（HTTP）或 `8443`（HTTPS）到允许的网段
3. 使用 VPN 或堡垒机访问
4. 管理员密码定期轮换（90 天）

---

## 九、登录安全基线（锁定 + 验证码 + 默认账号策略）

### 9.1 登录失败锁定语义详解

应用层内置账号级登录失败计数与自动锁定，配置项为 `LOGIN_MAX_FAILURES`（默认 5）和 `LOGIN_LOCK_MINUTES`（默认 15）。

**判定规则**：

```
每次登录请求：
  1. 检查 locked_until 是否 > now()
     → 是：返回"账号已锁定，剩余 X 分钟"，**不累加** login_attempts
  2. 验证密码
     → 成功：login_attempts 清零，locked_until 置 NULL，返回 token
     → 失败：login_attempts + 1
  3. 检查 login_attempts >= LOGIN_MAX_FAILURES
     → 是：locked_until = now + LOGIN_LOCK_MINUTES，返回"账号已锁定"
     → 否：返回"账号或密码错误，还可尝试 N 次"
```

**关键安全设计**：
- **连续失败计数**：每次密码验证失败才 +1，验证码失败不计入（验证码错误返回单独的"验证码错误"提示，不消耗密码尝试次数）
- **成功后清零**：任何一次密码验证成功都会把 `login_attempts` 重置为 0
- **锁定期内不累加**：锁定期间的尝试只返回"已锁定"提示，不增加计数器，防止攻击者在锁定期内耗尽计数器导致永远无法解锁
- **自动解锁**：`locked_until` 到期后，下一次登录请求会自动识别为未锁定状态，计数器从当前值（通常已被上次成功清零）继续或从 0 开始

> 实现依赖 `users` 表的 `login_attempts`（INT, DEFAULT 0）和 `locked_until`（DATETIME, NULLABLE）列。这两列由后端任务的数据库迁移脚本添加，Go 后端在登录 handler 中执行上述判定逻辑。

### 9.2 图形验证码安全边界

| 维度 | 设计 |
|------|------|
| 生成 | Go 后端 `image/draw` + `golang.org/x/image/font` 在内存中生成 SVG，不依赖第三方服务 |
| 存储 | 服务端内存 map（key = captcha_id，value = 验证码文本 + 过期时间） |
| 内容 | 4 字符混合（大写字母 A-Z + 数字 2-9，剔除易混淆字符 0/O/1/I/L） |
| **一次性使用** | 验证成功或失败后立即从服务端 map 中删除该验证码条目，防止重放 |
| **严格过期** | 超过 `CAPTCHA_TTL_SECONDS`（默认 120 秒）未使用自动失效 |
| 接口 | `GET /api/auth/captcha` 返回 SVG + captcha_id；`POST /api/auth/login` 携带 captcha_id + captcha_text |
| 关闭条件 | 仅当 `CAPTCHA_ENABLED=false` 时跳过验证码校验 |

**⚠️ CAPTCHA_ENABLED=false 的风险与约束**：

| 场景 | 允许？ | 附加防护 |
|------|--------|----------|
| 公网部署 | ❌ **禁止** | — |
| 内网自动化测试（CI/CD / 接口回归） | ⚠️ 仅测试机 | 必须同时在反向代理层加 IP 白名单（仅允许测试机 IP 访问登录接口） |
| 内网手动测试 | ⚠️ 临时 | 测试完成后立即恢复 `CAPTCHA_ENABLED=true` |

**正确做法**：公网部署**必须保持** `CAPTCHA_ENABLED=true`。自动化测试场景建议：
1. 反向代理层对测试机 IP 放行时跳过验证码校验（nginx `map` 变量 + Go 后端 `X-Test-Ip` header 识别）
2. 或在测试环境单独部署（`APP_ENV=test` 时自动关验证码）

### 9.3 默认账号密码策略重申

**这是开源红线**，任何文档、脚本、示例、迁移文件都不得违反：

| 禁止 | 允许 |
|------|------|
| 在仓库任何文件中出现真实口令明文 | 仅在 `.env`（本地不提交）中出现真实口令 |
| 在 `.env.example` 中出现真实口令 | 在 `.env.example` 中出现 `__REPLACE_WITH__` 占位符 |
| 脚本内置硬编码默认密码 | 脚本从参数或环境变量读取口令，缺失则报错退出 |
| 迁移 SQL 中 `INSERT INTO users ... VALUES ('admin', ..., 'admin123')` | 迁移 SQL 只插入用户名和角色，密码列留 NULL 或占位符 |

**初始口令来源**（现行实现）：
```
1. 管理员：安装器写入 .env 的 ADMIN_INIT_PASSWORD / PLATFORMADMIN_INIT_PASSWORD（随机、每账号独立）
2. 手工部署兜底：drill-seed 随机生成，仅写入启动日志一次
3. 后台新建/重置账号：服务端随机生成，仅响应返回一次
4. 任何路径都不使用统一口令或内置默认值；首次登录强制改密
```

**密码哈希算法**（所有写入路径统一遵循）：
```
scrypt(N=16384, r=8, p=1, dkLen=32)
salt = 32 字符随机 hex 字符串 → []byte(saltHex) 直接喂入（禁止 hex.DecodeString）
hash = scrypt([]byte(passwordUTF8), []byte(saltHex), N, r, p, dkLen)
存储：pwd_salt = saltHex, pwd_hash = hex.EncodeToString(hash)
```

---

## 十、快速检查表（生产 Go-Live 前）

- [ ] `.env` 里所有 `__REPLACE_WITH__` 占位符已替换
- [ ] JWT_SECRET / SESSION_SECRET 已用 `openssl rand -hex 32` 生成
- [ ] MySQL 已创建专用业务账号，只有 `drill_system.*` 权限
- [ ] HTTPS 已启用（局域网自签 / 公网 Let's Encrypt）
- [ ] HTTP_REDIRECT_TO_HTTPS=true + HSTS
- [ ] 所有账号初始密码已通过 bootstrap 或 `scripts\reset_default_passwords.ps1` 写入（无仓库内置明文）
- [ ] 登录安全配置已显式设置或确认默认值：
  - [ ] `LOGIN_MAX_FAILURES` / `LOGIN_LOCK_MINUTES`（默认 5 / 15，公网建议调严）
  - [ ] `CAPTCHA_ENABLED=true`（公网**不得**为 false）
  - [ ] `CAPTCHA_TTL_SECONDS`（默认 120）
- [ ] 反向代理已加白名单保护 reset / admin 端点
- [ ] 自动备份已配置 + 恢复演练已做
- [ ] 防火墙只放行允许的 IP 段
- [ ] `/api/health` 和 `/api/admin/system/reset` 已在测试库验证白名单保护
