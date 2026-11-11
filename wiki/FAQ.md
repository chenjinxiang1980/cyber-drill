# 常见问题

## 安装时报“已存在安装目录”

说明本机已经安装过，或上次安装未完成。安装器不会覆盖数据。

```bash
# A. 删除重装（会删除容器与数据卷，输入 DELETE 确认）
sudo bash deploy/install/install.sh --reinstall

# B. 保留数据升级
sudo bash deploy/upgrade/upgrade.sh --to 1.0.3
```

不要直接 `rm -rf /opt/cyber-drill` 后重装：旧 MySQL 数据卷还在，旧库口令与新 `.env` 不一致，会出现“装完却登不上”。

## 装完登录提示账号或密码错误

1. 确认使用的是安装结束打印的 `admin` / `platformadmin` 初始口令；
2. 口令在 `/opt/cyber-drill/.env`（权限 600），字段为 `ADMIN_INIT_PASSWORD` / `PLATFORMADMIN_INIT_PASSWORD`；
3. 若字段已为空，说明首次登录已改密，原初始口令失效；
4. 若确实遗忘，走“重置管理员口令”流程（见下方）。

## 忘记管理员口令

在服务器上使用数据库工具重置，或联系交付人员按 `docs/security-guide.md` 的流程处理。重置属于敏感操作，必须留审计记录。

## 验证码看不清 / 输入总错

- 验证码为算术题，结果范围 0–98；
- 验证码错误也计入登录锁定，阈值是密码阈值的 2 倍；
- 连续失败会触发“账号 + IP”锁定，默认 5 次 / 15 分钟；
- 锁定状态保存在内存，重启应用容器后清零。

## 大屏打不开

- `SCREEN_PUBLIC_VIEW` 必须保持 `true`；
- 大屏是匿名只读页面，不要通过关闭该开关来“收口”；
- 需要限制来源时，在 firewalld / 反向代理层限制 IP。

## 端口被占用

```bash
sudo bash deploy/install/install.sh --port 18090 --mysql-port 13306
```

安装前可用 `ss -lnt | grep 8090` 检查。

## 镜像拉取失败

- 国内网络优先使用 `--image-source swr`；
- 国外网络使用 `--image-source ghcr`；
- 完全无外网时使用离线包 `--offline-dir`。

## 修改了前端页面但不生效

前端静态资源已打进镜像。修改 `public/` 后需要重新构建镜像并重启容器，单独替换容器内文件在重建后会被覆盖。

## 升级失败

升级工具失败会自动回滚。若需人工处理：

1. `sudo bash deploy/upgrade/upgrade.sh --status` 查看状态；
2. 查看升级日志与容器日志 `docker logs <项目名>-app`（默认项目为 `drill-app`）；
3. 含 DDL 的迁移只能整库恢复到升级前备份；
4. 恢复前确认 `BACKUP_PASSPHRASE` / `RESTORE_PASSWORD` 可用。

## 备份文件能不能只拷文件

不能。备份为 AES-256-GCM 加密，恢复必须同时具备备份文件与口令。口令与备份应分离保管、双人控制。

## 日志在哪里

```bash
docker logs --tail 200 drill-app      # 容器名 = <项目名>-app，默认项目 drill
docker logs --tail 200 drill-mysql    # 容器名 = <项目名>-mysql
```

审计日志在数据库 `audit_logs` 表，按 `AUDIT_RETENTION_DAYS` 清理。
