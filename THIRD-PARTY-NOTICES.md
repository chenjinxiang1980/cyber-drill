# 第三方许可证清单（THIRD-PARTY NOTICES）

| 项目 | 内容 |
|---|---|
| 产品 | 应急演练指挥系统（Cyber-Drill-System） |
| 版本 | 1.0.5 |
| 生成日期 | 2026-11-11 |
| 生成范围 | Go 服务端实际构建依赖 + 前端静态资源 + 容器运行时组件 + 外部数据库 |
| 本项目许可证 | MIT（见仓库根目录 [LICENSE](LICENSE)） |

> 本清单基于仓库真实依赖生成，不使用通用模板。Go 依赖以 `go list -deps ./...` 的**实际构建依赖图**为准；`go.mod` 中声明但未进入构建的模块不列入（见第六节）。
>
> 本清单不构成法律意见。对外交付前，建议由客户法务/合规部门复核，尤其是第七节标注的 MPL-2.0 与 GPL-2.0 组件。

---

## 一、Go 服务端构建依赖

生成命令（在 `server/` 目录执行）：

```bash
go list -deps -f '{{if .Module}}{{.Module.Path}}\t{{.Module.Version}}{{end}}' ./... | sort -u
```

共 **20** 个外部模块（不含 Go 标准库）。许可证按各模块上游 `LICENSE` / `COPYING` / `NOTICE` 原文判定。

| # | 组件 | 版本 | 许可证 | 引入方式 | 上游 / 许可证原文 |
|---:|---|---|---|---|---|
| 1 | `filippo.io/edwards25519` | v1.1.0 | BSD-3-Clause | 间接（MySQL 驱动） | <https://github.com/FiloSottile/edwards25519> |
| 2 | `github.com/gabriel-vasile/mimetype` | v1.4.3 | MIT | 间接（Gin 参数绑定） | <https://github.com/gabriel-vasile/mimetype> |
| 3 | `github.com/gin-contrib/sse` | v0.1.0 | MIT | 间接（Gin） | <https://github.com/gin-contrib/sse> |
| 4 | `github.com/gin-gonic/gin` | v1.10.0 | MIT | 直接 | <https://github.com/gin-gonic/gin> |
| 5 | `github.com/go-playground/locales` | v0.14.1 | MIT | 间接（validator） | <https://github.com/go-playground/locales> |
| 6 | `github.com/go-playground/universal-translator` | v0.18.1 | MIT | 间接（validator） | <https://github.com/go-playground/universal-translator> |
| 7 | `github.com/go-playground/validator/v10` | v10.20.0 | MIT | 间接（Gin 参数校验） | <https://github.com/go-playground/validator> |
| 8 | `github.com/go-sql-driver/mysql` | v1.8.1 | **MPL-2.0** | 直接 | <https://github.com/go-sql-driver/mysql> |
| 9 | `github.com/golang-jwt/jwt/v5` | v5.2.1 | MIT | 直接 | <https://github.com/golang-jwt/jwt> |
| 10 | `github.com/leodido/go-urn` | v1.4.0 | MIT | 间接（validator） | <https://github.com/leodido/go-urn> |
| 11 | `github.com/mattn/go-isatty` | v0.0.20 | MIT | 间接（Gin） | <https://github.com/mattn/go-isatty> |
| 12 | `github.com/pelletier/go-toml/v2` | v2.2.2 | MIT | 间接（Gin 配置） | <https://github.com/pelletier/go-toml> |
| 13 | `github.com/ugorji/go/codec` | v1.2.12 | MIT | 间接（Gin 序列化） | <https://github.com/ugorji/go> |
| 14 | `golang.org/x/crypto` | v0.31.0 | BSD-3-Clause | 直接 | <https://cs.opensource.google/go/x/crypto> |
| 15 | `golang.org/x/net` | v0.25.0 | BSD-3-Clause | 间接 | <https://cs.opensource.google/go/x/net> |
| 16 | `golang.org/x/sys` | v0.28.0 | BSD-3-Clause | 间接 | <https://cs.opensource.google/go/x/sys> |
| 17 | `golang.org/x/text` | v0.21.0 | BSD-3-Clause | 间接 | <https://cs.opensource.google/go/x/text> |
| 18 | `golang.org/x/time` | v0.9.0 | BSD-3-Clause | 直接 | <https://cs.opensource.google/go/x/time> |
| 19 | `google.golang.org/protobuf` | v1.34.1 | BSD-3-Clause | 间接 | <https://github.com/protocolbuffers/protobuf-go> |
| 20 | `gopkg.in/yaml.v3` | v3.0.1 | **MIT / Apache-2.0（双许可）** | 间接 | <https://github.com/go-yaml/yaml/tree/v3.0.1> |

### 1.1 Go 标准库与工具链

| 组件 | 版本 | 许可证 | 说明 |
|---|---|---|---|
| Go 标准库（`std`） | go1.27.3 | BSD-3-Clause | 随 `drill-server` 静态编译进二进制；许可证见 <https://go.dev/LICENSE> |
| Go 工具链 / `golang:1.27-alpine` 构建镜像 | 1.27.x | BSD-3-Clause | 仅构建阶段使用，不进入运行时镜像 |

> `go version` 实测为 `go1.27.3`。若使用不同补丁版本构建，请同步更新本表。

---

## 二、前端静态资源

`public/` 为**无构建链的纯静态前端**，直接由 Go 服务托管。经仓库检索确认：

- 无 `package.json`、无 `node_modules`、无打包器配置；
- 未引入 React / Vue / jQuery / ECharts / Chart.js / lodash / moment / dayjs / Bootstrap / Tailwind / Font Awesome / axios / marked / DOMPurify / xlsx 等第三方库；
- 未引用任何外部 CDN、外部字体或远程脚本；
- 图标与图形为仓库内联 SVG / CSS 绘制；字体使用操作系统本地字体栈。

| 组件 | 版本 | 许可证 | 说明 |
|---|---|---|---|
| 本项目前端代码（`public/*.html`、`*.js`、`*.css`） | 1.0.5 | MIT | 版权人：陈金祥；无第三方前端库 |

> 如后续引入任何前端库、字体、图标包或 CDN 资源，必须同步更新本节并在交付前重新生成清单。

---

## 三、容器运行时组件（`deploy/docker-dist/Dockerfile`）

运行时基础镜像为 `alpine:3.20`，构建阶段使用 `golang:1.27-alpine`。运行时通过 `apk add --no-cache` 安装以下组件：

| 组件 | 版本（v3.20 仓库，截至 2026-11-10） | 许可证 | 用途 | 来源 |
|---|---|---|---|---|
| Alpine Linux 基础镜像 | 3.20（补丁号未锁定） | 各子包分别授权（含 MIT、GPL-2.0、GPL-3.0 等） | 运行时根文件系统 | <https://alpinelinux.org/> / <https://gitlab.alpinelinux.org/alpine/aports> |
| musl libc | 1.2.5-r4 | MIT | C 运行库 | <https://musl.libc.org/> |
| BusyBox | 1.36.1-r28 | **GPL-2.0-only** | 基础命令与部分 applet | <https://busybox.net/> |
| `wget`（GNU Wget） | 1.24.5-r0 | **GPL-3.0-or-later WITH OpenSSL-Exception** | 健康检查与 DB 就绪探测 | <https://www.gnu.org/software/wget/> |
| apk-tools | 2.14.13-r0 | GPL-2.0-only | 包管理器（仅构建/安装期） | <https://gitlab.alpinelinux.org/alpine/apk-tools> |
| `tini` | 0.19.0-r3 | MIT | init 进程，转发 SIGTERM | <https://github.com/krallin/tini> |
| `ca-certificates` | 20240226-r0 | **MPL-2.0 AND MIT** | CA 根证书 | <https://gitlab.alpinelinux.org/alpine/aports/-/tree/master/main/ca-certificates> |
| `tzdata` | 2024a-r1 | Public-Domain | 时区数据 | <https://www.iana.org/time-zones> |
| `mariadb-client` | 10.11.18-r0 | **GPL-2.0-or-later** | `mysqldump` / `mysql` 客户端，备份与恢复必需 | <https://mariadb.org/> |

> **版本说明**：上述 apk 包未在 Dockerfile 中锁定具体版本，实际版本由构建时的 `alpine:3.20` 仓库决定。若交付要求可复现构建，建议在构建记录中保存 `apk info -v` 输出，或固定到具体 `alpine:3.20.x` 补丁镜像与包版本。
>
> 运行时镜像的 OCI 标签声明本项目代码许可证为 MIT：
> `org.opencontainers.image.licenses="MIT"`。该标签不改变基础镜像与系统包各自的许可证。

---

## 四、外部数据库

| 组件 | 版本 | 许可证 | 说明 |
|---|---|---|---|
| MySQL | 8.0.x（`mysql:8.0` 镜像 / 客户自建 8.0） | **GPL-2.0**（含 FOSS Exception） | 独立进程/容器，通过 TCP 连接；**不链接进本项目代码**，属于聚合分发 |

> MySQL 由客户自行部署或使用官方镜像，通常不随本项目镜像一起分发。若交付包中一并分发 MySQL 镜像或二进制，分发方需自行履行 GPL-2.0 义务（提供对应源代码或有效的书面要约），并保留 MySQL 的许可证与版权声明。Oracle 的 FOSS Exception 仅影响“将 MySQL 客户端库链接到 FOSS 项目”的特定场景，不改变 MySQL 服务端本身的 GPL-2.0。

---

## 五、开发/构建期工具（不进入交付物）

| 组件 | 版本 | 许可证 | 说明 |
|---|---|---|---|
| Go 工具链 | 1.27.x | BSD-3-Clause | 编译 `drill-server` / `drill-migrate` / `drill-seed` |
| Docker / BuildKit | 客户环境版本 | Apache-2.0 | 构建镜像 |
| `python` + `paramiko`（仅作者验证脚本） | 客户环境版本 | PSF / LGPL-2.1 | 仅本地测试脚本使用，不进入交付物、不进入仓库运行路径 |

---

## 六、未列入清单的模块

`server/go.mod` 中声明了 `modernc.org/sqlite v1.33.1` 等模块，但截至 1.0.5，它们**未出现在 `go list -deps ./...` 的实际构建依赖图中**，因此不进入二进制，也不列入本清单。

判断标准：

```bash
# 实际进入构建的模块（本清单依据）
go list -deps -f '{{if .Module}}{{.Module.Path}}\t{{.Module.Version}}{{end}}' ./... | sort -u

# 模块图（包含未被引用的 require，不能直接当作交付依赖）
go list -m all
```

> 如果后续在代码中启用 SQLite、测试依赖或其他当前未使用的模块，必须重新生成清单并补充许可证。

---

## 七、需要特别注意的许可证

### 7.1 MPL-2.0：`github.com/go-sql-driver/mysql v1.8.1`

- 属于**文件级弱 copyleft**：对本项目**未修改**的 MPL 文件，只需保留许可证与版权声明；若修改了 MPL 覆盖的文件，修改后的文件需以 MPL-2.0 公开。
- 本项目以未修改的上游版本使用该驱动，未修改其源文件。
- 分发二进制时，应随附本清单，并可提供该组件的上游源码地址：
  <https://github.com/go-sql-driver/mysql/tree/v1.8.1>
- MPL-2.0 全文：<https://www.mozilla.org/en-US/MPL/2.0/>

> 注意：MPL-2.0 文本中包含“GPL 二次许可”条款（Exhibit B），但该组件本身是 MPL-2.0，**不是 GPL**。许可证扫描工具若仅按关键字匹配，容易误报为 GPL。

### 7.2 GPL-2.0 / GPL-3.0：运行时系统包与 MySQL

以下组件为 **GPL-2.0 / GPL-3.0**，但均以**独立可执行文件/独立进程**形式使用，不与本项目代码链接：

- BusyBox（GPL-2.0）
- apk-tools（GPL-2.0，构建/安装期）
- `wget` / GNU Wget（GPL-3.0-or-later）
- `mariadb-client`（GPL-2.0，`mysqldump` / `mysql`）
- MySQL 8.0（GPL-2.0，独立数据库服务）

分发含上述组件的镜像时，分发方需履行对应 GPL 版本的相应义务（保留声明、提供对应源代码或书面要约等）。本项目源代码仍按 MIT 分发，不因聚合分发而被要求改为 GPL。

### 7.3 未发现的许可证

- 本次实际 Go 构建依赖中**未发现 GPL / AGPL / LGPL** 组件。
- 前端**未引入**任何第三方库，因此不存在前端 GPL/AGPL 风险。
- 如后续新增依赖，请在合并前重新扫描并在本清单中登记。

---

## 八、标准许可证文本

### 8.1 MIT License

以下为 MIT 许可证标准文本。各 MIT 组件的具体版权行以其上游 `LICENSE` 文件为准（见第一节表格中的上游链接）。

```text
MIT License

Copyright (c) <year> <copyright holders>

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.
```

### 8.2 BSD 3-Clause License

以下为 BSD-3-Clause 标准文本。各组件版权行以上游 `LICENSE` 为准。

```text
BSD 3-Clause License

Copyright (c) <year>, <copyright holder>
All rights reserved.

Redistribution and use in source and binary forms, with or without
modification, are permitted provided that the following conditions are met:

1. Redistributions of source code must retain the above copyright notice, this
   list of conditions and the following disclaimer.

2. Redistributions in binary form must reproduce the above copyright notice,
   this list of conditions and the following disclaimer in the documentation
   and/or other materials provided with the distribution.

3. Neither the name of the copyright holder nor the names of its
   contributors may be used to endorse or promote products derived from
   this software without specific prior written permission.

THIS SOFTWARE IS PROVIDED BY THE COPYRIGHT HOLDERS AND CONTRIBUTORS "AS IS"
AND ANY EXPRESS OR IMPLIED WARRANTIES, INCLUDING, BUT NOT LIMITED TO, THE
IMPLIED WARRANTIES OF MERCHANTABILITY AND FITNESS FOR A PARTICULAR PURPOSE ARE
DISCLAIMED. IN NO EVENT SHALL THE COPYRIGHT HOLDER OR CONTRIBUTORS BE LIABLE
FOR ANY DIRECT, INDIRECT, INCIDENTAL, SPECIAL, EXEMPLARY, OR CONSEQUENTIAL
DAMAGES (INCLUDING, BUT NOT LIMITED TO, PROCUREMENT OF SUBSTITUTE GOODS OR
SERVICES; LOSS OF USE, DATA, OR PROFITS; OR BUSINESS INTERRUPTION) HOWEVER
CAUSED AND ON ANY THEORY OF LIABILITY, WHETHER IN CONTRACT, STRICT LIABILITY,
OR TORT (INCLUDING NEGLIGENCE OR OTHERWISE) ARISING IN ANY WAY OUT OF THE USE
OF THIS SOFTWARE, EVEN IF ADVISED OF THE POSSIBILITY OF SUCH DAMAGE.
```

### 8.3 Apache License 2.0

适用于 `gopkg.in/yaml.v3`（双许可中的 Apache-2.0 选项，亦可按 MIT 使用）等组件。

- 许可证全文：<https://www.apache.org/licenses/LICENSE-2.0>
- 上游声明：<https://github.com/go-yaml/yaml/blob/v3.0.1/LICENSE>

### 8.4 Mozilla Public License 2.0

适用于 `github.com/go-sql-driver/mysql`。

- 许可证全文：<https://www.mozilla.org/en-US/MPL/2.0/>
- 上游声明：<https://github.com/go-sql-driver/mysql/blob/v1.8.1/LICENSE>

### 8.5 GNU General Public License v2.0 / v3.0

适用于 BusyBox、apk-tools、`mariadb-client`、MySQL 8.0（GPL-2.0）以及 GNU Wget（GPL-3.0-or-later）等独立组件。

- GPL-2.0 全文：<https://www.gnu.org/licenses/old-licenses/gpl-2.0.html>
- GPL-3.0 全文：<https://www.gnu.org/licenses/gpl-3.0.html>
- BusyBox：<https://busybox.net/license.html>
- GNU Wget：<https://www.gnu.org/software/wget/>
- MariaDB：<https://mariadb.com/kb/en/mariadb-license/>
- MySQL：<https://www.mysql.com/about/legal/licensing/>

---

## 九、重新生成与维护

1. 更新 Go 依赖后，在 `server/` 执行：

   ```bash
   go mod tidy
   go list -deps -f '{{if .Module}}{{.Module.Path}}\t{{.Module.Version}}{{end}}' ./... | sort -u
   ```

2. 逐个核对新增模块的 `LICENSE` / `COPYING` / `NOTICE`，更新第一节表格与第七节风险提示。
3. 如新增前端库、字体、图标包或 CDN，更新第二节与第七节。
4. 如修改运行时基础镜像或 `apk add` 包列表，更新第三节，并保存一次 `apk info -v` 作为构建记录附件。
5. 发布前将本清单与 [隐私与数据处理说明](docs/privacy-data-processing.md) 一并交付客户。

---

## 十、联系

| 事项 | 联系方式 |
|---|---|
| 开源合规 / 许可证咨询 | 【待填写：供应商联系人 / 邮箱】 |
| 安全与数据保护 | 【待填写：供应商安全负责人 / 邮箱】 |

> 本清单随版本更新；1.0.5 为当前基线。
