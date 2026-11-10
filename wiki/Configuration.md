# 配置项

配置来源为安装目录下的 `.env`（权限 600）。修改后需重启应用容器：

```bash
cd /opt/cyber-drill
docker compose --env-file .env up -d --force-recreate app
```

## 基础

| 变量 | 默认 | 说明 |
|---|---|---|
| `APP_ENV` | `production` | 运行环境 |
| `HTTP_ADDR` | `0.0.0.0:8090` | HTTP 监听地址 |
| `PUBLIC_BASE_URL` | 空 | 对外访问地址，用于链接生成 |
| `DB_HOST` / `DB_PORT` | `127.0.0.1` / `3306` | 数据库地址 |
| `DB_NAME` | `drill_system` | 数据库名 |
| `DB_USER` / `DB_PASSWORD` | 无 | 业务数据库账号，禁止使用 root |
| `JWT_SECRET` | 无 | JWT 签名密钥，`openssl rand -hex 32` |
| `JWT_TTL` | `480` | 会话有效期（分钟） |
| `SESSION_SECRET` | 无 | 会话密钥，`openssl rand -hex 32` |

## 登录与安全

| 变量 | 默认 | 说明 |
|---|---|---|
| `LOGIN_MAX_FAILURES` | `5` | 同一账号 + IP 连续失败锁定阈值 |
| `LOGIN_LOCK_MINUTES` | `15` | 锁定分钟数 |
| `CAPTCHA_ENABLED` | `true` | 算术 SVG 验证码开关；错误也计入锁定 |
| `CAPTCHA_TTL_SECONDS` | `120` | 验证码有效期；0 或 >300 回落 120 |
| `RATE_LIMIT_GLOBAL` | `300` | `/api/*` 每分钟限流 |
| `RATE_LIMIT_AUTH` | `30` | 登录接口独立限流 |
| `RATE_LIMIT_CAPTCHA` | `20` | 验证码接口限流 |
| `AUDIT_RETENTION_DAYS` | `365` | 审计日志保留天数，0 = 不清理 |
| `TRUSTED_PROXIES` | 空 | 反向代理网段，用于取真实客户端 IP |
| `ALLOW_ORIGINS` | 空 | 默认同源；禁止 `*` |

## TLS

| 变量 | 默认 | 说明 |
|---|---|---|
| `TLS_ENABLED` | `false` | 是否启用应用内 HTTPS |
| `TLS_ADDR` | `:8443` | HTTPS 监听端口 |
| `HTTPS_CERT_FILE` | 空 | 证书 PEM 全链路径 |
| `HTTPS_KEY_FILE` | 空 | 私钥 PEM 路径 |
| `HTTP_REDIRECT_TO_HTTPS` | `false` | HTTP 301 跳转到 HTTPS |

对外交付必须启用 TLS。证书缺失时服务直接拒绝启动，不会静默降级。

## 上传与备份

| 变量 | 默认 | 说明 |
|---|---|---|
| `UPLOAD_ROOT` | `uploads` | 上传文件目录 |
| `UPLOAD_MAX_MB` | `20` | 单文件上限 |
| `DATA_DIR` | `data` | 数据目录 |
| `BACKUP_PASSPHRASE` | 无 | 备份加密口令；未配置时备份接口拒绝执行 |
| `RESTORE_PASSWORD` | 无 | 恢复入口独立口令；未设置时禁止恢复 |

备份文件为 AES-256-GCM 分块加密（`DRLBK1`），口令丢失则备份无法恢复。

## 大屏

| 变量 | 默认 | 说明 |
|---|---|---|
| `SCREEN_PUBLIC_VIEW` | `true` | 大屏匿名只读；关闭会导致大屏打不开。需要收口请在外层防火墙/反代限制来源 IP |

## 版本

镜像 tag 与仓库根目录 `VERSION` 一致。升级时由 `deploy/upgrade/upgrade.sh --to <版本>` 控制，不要手工改镜像 tag。

页面底部等 `data-brand="version"` 位置显示的是**运行时版本**：前端优先读 `/api/health` 返回的 `version`，取不到才回退系统设置里的 `brand_version`。因此补丁版本不需要靠新增数据库迁移推进 `settings.brand_version`；改动 `public/` 下含内联脚本的文件后，CSP 哈希在服务启动时重算，需重启容器（或重建镜像）才生效。
