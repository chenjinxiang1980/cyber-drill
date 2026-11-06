# Release 0.6.0 —— 处置文字质量规则、表单模板编号与交互修正

**发布日期**：2026-11-07
**版本号**：`0.6.0`
**标签**：`v0.6.0`
**上一版本**：`0.5.1`（见 [release-0.5.1.md](release-0.5.1.md)）

---

## 一、本次变化

本版本是本轮在现网验证后首次入库的完整变更集，三块实质变化如下。

### 1.1 处置文字质量规则与匹配容错

- **规则下沉到节点**（migration `0011`）：`script_nodes.quality_rules` 以 JSON 保存该节点的必含关键词、加分关键词、最小字数、要素要求、敷衍词与扣分比例；不配则沿用旧口径。
- **只匹配文字本身**：关键词只扫描处置文字/说明类字段，记录表单的填写值不再参与匹配，避免"表单里勾了就算命中"。
- **归一化容错**：全半角、大小写、零宽字符、连续空白统一归一化后比对；数字 `1`/`一`、`0`/`零` 等写法等价；必含关键词支持同义词组（如"隔离/封堵/阻断"）。
- **字数与要素门槛**：低于最小字数按比例扣分；要素缺失按项扣分；证据要求为 `[]` 或空串的节点不再无条件扣 20%。
- **敷衍文本拦截**：`ok`、`无`、`已处理`、`见附件` 等敷衍短文本在提交接口直接 400，并提示补充要素。
- **老数据兼容**：无 `quality_rules` 的节点回落旧评分口径；`drill_score.go`、`drill_v1.go`、`script_nodes_write.go`、`bootstrap.go` 同步适配。

规则细节见 `docs/quality-rules-framework.md`、`docs/quality-rules-s01.md` … `s08`、`docs/quality-rules-a6-review.md`、`docs/quality-rules-risk-scan.md`。

### 1.2 表单模板编号三位化与保存修复

- **展示编号**（migration `0012`）：`form_templates` 新增 `code`，回填 `F001`、`F002`…，id 与既有引用不变；系统设置模板列表、剧本「关联表单」下拉均按 `code` 排序展示，不再出现 1、10、2 的乱序。
- **保存修复**：后端保存成功仅返回 `{ok:true}` 时，前端不再读取 `r.template.id` 导致报错中断；保存后按 id 局部刷新，不再整表重载。
- 模板与节点的绑定关系不受编号改造影响，升级无需人工改数据。

### 1.3 大屏与工作台交互修正

- **大屏**：语音播报开关可持久化；匿名观摩接口收敛为只读（GET 放行、写方法 403，独立限流）；三处滚动模块末位错位修正。
- **工作台**：处置弹窗改为左右分栏，前置节点与当前节点同时可见；进行中演练的脚本锁定提示更明确。
- **剧本编排 / 管理页**：节点保存不再整表删除重建、seq 重排修正；管理页字典/角色操作的容错与提示补齐。

---

## 二、升级说明（0.5.1 → 0.6.0）

1. 备份数据库（`mysqldump --single-transaction`）。
2. 拉取 `0.6.0` 镜像（或改 compose 的 `VERSION=0.6.0`）后 `docker compose up -d`。
3. 入口脚本自动执行 `drill-migrate up`：`0011` 仅加列，`0012` 加列并回填 `code`；`drill-seed` 只增不删。
4. 验证：`/api/health` 返回 `version=0.6.0`；系统设置页品牌版本显示 `v0.6.0`；老节点评分与旧口径一致。
5. **品牌版本以库为准**：系统设置读取 `settings.brand_version`，老库不会自动跟随代码默认值。本次现网已单独把该行由 `v0.2.3` 改为 `v0.6.0`（改前已备份），新装环境默认即 `v0.6.0`。

---

## 三、发布物

| 产物 | 说明 |
|------|------|
| 源码 | GitHub / Gitee `main` + `v0.6.0` 注释标签 |
| Docker 镜像 | 本地 `drill-server:0.6.0`（`sha256:18e8a5b8efd6…`），66,665,330 B，构建提交 `f125fd8` |
| GHCR | `ghcr.io/chenjinxiang1980/cyber-drill:0.6.0` / `:latest` |
| SWR | `swr.cn-east-3.myhuaweicloud.com/chenjinxiang/cyber-drill:0.6.0` / `:latest` |

**manifest digest（两仓库一致）**

```
sha256:6ade7adcb14b3e1390203710e6f6da1465f36b5179c77c59b5828d5dc20b4bd3
```

容器内 `/opt/drill/bin/drill-server`：11,587,744 B，SHA-256
`f0f605faa08a434f802465d7af735628cc4c387811ba8c2df5ee8a2ba1bd0c59`
（已写入 `server/deploy-linux/drill-server.sha256.txt`）。
镜像内 `schema_migrations` 共 12 条，最新 `0012_form_template_code`。

**可复现构建**

```bash
docker build -f deploy/docker-dist/Dockerfile \
  --build-arg VERSION=0.6.0 \
  --build-arg BUILD_TIME=<ISO8601> \
  --build-arg VCS_REF=f125fd8 \
  --build-arg GIT_COMMIT=f125fd8 \
  -t drill-server:0.6.0 .
```

构建机：UOS Server 20 / Docker 25.0.4（legacy builder，无 buildx）；需代理时追加
`--build-arg HTTP_PROXY=... --build-arg HTTPS_PROXY=...`。

**镜像内容快照（关键文件 SHA-256，小写）**

| 文件 | SHA-256 | Size |
|------|---------|------|
| server/internal/version/version.go | `9d2246f78e10b271d994aa416218ba8356530aa19e88787c22dde23cdd27d54f` | 379 |
| public/app.js | `c14346ac3e34a051b2047c0860074b12f72a81a7c513989dc56355761a581900` | 56004 |
| public/screen.html | `cebecb23923a9a3505c4a3f1028454e96a1d8c0f423f49e334982918612eaf5d` | 66395 |
| public/workbench.html | `bd8e37941fe87739e5fc4ac9a1c79989792c4bc495e998927265395154aa2ffc` | 111891 |
| public/index.html | `fa25b1e54f25a3441966e6282d11a366c4372d2aa041e771e5e9ed8697fe994c` | 13586 |
| deploy/docker-dist/Dockerfile | `cbbc5d01f028fe0a12c83ae6c565bfb65e94c23db7699bc67ca540f60b648645` | 7504 |
| install/install-linux.sh | `0c6d79a51444d0f73a1a31fd2432275d5922b444b68e699c0dc65dace6ece8c8` | 41531 |
| install/install-windows.ps1 | `c7ebfbff9596cc1196087f7483ab2d013c7550cba97a1df0554c2a26a9cee546` | 39380 |

---

## 四、镜像拉取与运行

```bash
docker pull ghcr.io/chenjinxiang1980/cyber-drill:0.6.0
docker pull swr.cn-east-3.myhuaweicloud.com/chenjinxiang/cyber-drill:0.6.0
```

两仓库均为 **public**，匿名可拉；仅在匿名失败时才需要登录：

```bash
printf '%s' "$REGISTRY_PASS" | docker login swr.cn-east-3.myhuaweicloud.com -u '<cn-east-3@AK>' --password-stdin
```

一键安装保持 `install/install-linux.sh`（`VERSION=0.6.0`）与 `install-windows.ps1`（`$Version='0.6.0'`）。

---

## 五、发布验证记录

| 项目 | 结果 |
|------|------|
| `/api/health` | 200，`{"ok":true,...,"version":"0.6.0"}` |
| `/api/system/brand` | `"version":"v0.6.0"`（镜像内新库实测） |
| `/api/auth/captcha` | 200 |
| `/screen.html` | 200，66,395 B（仓库与容器一致） |
| 容器身份 | `uid=10001(drill) gid=10001(drill)` |
| 匿名大屏 | GET `/api/screen/monitor` 200 |
| 登录 | 带算术验证码 `admin` 登录 200，`/api/auth/me` 200 |
| 迁移 | 12 条，最新 `0012_form_template_code` |
| 冒烟隔离 | 独立项目 `drill060smoke`、端口 18092/13308，验后容器与卷已清理 |
| 匿名 pull | GHCR 与 SWR 均 `Downloaded newer image`，digest 一致 |
| 现网品牌版本 | `settings.brand_version` 由 `v0.2.3` 改为 `v0.6.0`，改前备份 `drill_system-before-brand-20261107-000636.sql.gz` |

---

## 六、GitHub / Gitee Release 建议文案

```markdown
## v0.6.0

### 主要变化
- 处置文字质量规则下沉到节点：关键词/要素/字数/敷衍词，匹配做全半角与同义词容错，只匹配文字不含表单值
- 表单模板展示编号三位化（F001…，migration 0012），修复模板保存后前端报错
- 大屏语音播报与匿名只读收敛；工作台处置弹窗左右分栏；剧本编排保存修正

### 镜像
- `docker pull ghcr.io/chenjinxiang1980/cyber-drill:0.6.0`
- `docker pull swr.cn-east-3.myhuaweicloud.com/chenjinxiang/cyber-drill:0.6.0`
- 两仓库公开匿名可拉，digest 一致：sha256:6ade7adc…

### 升级
备份数据库 → 拉取新镜像 → `docker compose up -d`（自动执行 0011/0012）。
老库如需系统设置显示新版本，请单独更新 `settings.brand_version`。

完整说明见 docs/release-0.6.0.md。MIT License © 2026 陈金祥。
```
