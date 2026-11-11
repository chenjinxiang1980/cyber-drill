# =============================================================================
# Drill System —— Docker 多阶段构建（可分发形态）
#
# 构建（仓库根作为 context）：
#   DOCKER_BUILDKIT=1 docker build -f deploy/docker-dist/Dockerfile \
#       -t drill-server:1.0.2 .
#
# 带代理（UOS Server 20 / VMware NAT 下，宿主 Fclash）：
#   DOCKER_BUILDKIT=1 docker build -f deploy/docker-dist/Dockerfile \
#       --build-arg HTTP_PROXY=http://<proxy-host>:<port> \
#       --build-arg HTTPS_PROXY=http://<proxy-host>:<port> \
#       -t drill-server:1.0.2 .
#
# arm64 跨平台：
#   DOCKER_BUILDKIT=1 docker buildx build --platform linux/arm64 \
#       -f deploy/docker-dist/Dockerfile -t drill-server:1.0.2 . --load
#
# 关键设计（踩坑后规避）：
#   1. 不使用 COPY --chown / COPY --chmod —— 旧版 builder（非 BuildKit）不生效；
#      统一用 COPY + RUN chown，兼容所有 builder 版本。
#   2. 不使用 /docker-entrypoint-initdb.d 挂载 migrations —— 宿主目录权限
#      不对会导致 MySQL 无限重启。改由 app entrypoint 等 DB 就绪后调用
#      内置的 drill-migrate 子二进制跑迁移（幂等）。
#   3. 零凭据内置：config.Load() 启动时严格校验 DB_USER / JWT_SECRET /
#      SESSION_SECRET，缺失直接 os.Exit(2)，绝不静默降级。
#   4. 非 root 运行：uid=10001, gid=10001（drill:drill）。
#   5. 静态编译（CGO_ENABLED=0）：amd64 / arm64 / loong64 跨发行版通用。
# =============================================================================

# --- 全局代理 ARG（两个阶段都能用）---
ARG HTTP_PROXY
ARG HTTPS_PROXY
ARG NO_PROXY=localhost,127.0.0.1

ARG BUILDER_BASE=golang:1.27-alpine
ARG RUNTIME_BASE=alpine:3.20

# ---------- Stage 1: Builder ----------
FROM ${BUILDER_BASE} AS builder

# 让 go mod download 用代理
ARG HTTP_PROXY
ARG HTTPS_PROXY
ARG NO_PROXY
ENV HTTP_PROXY=${HTTP_PROXY}
ENV HTTPS_PROXY=${HTTPS_PROXY}
ENV NO_PROXY=${NO_PROXY}

# module root 与原仓库一致：/src/server（避免 go.mod 嵌套导致 go build 找不到包）
WORKDIR /src/server

# 依赖缓存（go.mod / go.sum 不变则复用）
COPY server/go.mod server/go.sum ./
RUN go mod download

# 源码（整个 server/ 目录，含 main.go + internal/ + cmd/）
COPY server/ ./

# migrations 与 public 放 module root 外（只要 runtime 阶段能 COPY 到就行）
COPY migrations/  /src/migrations/
COPY public/      /src/public/

# 编译参数（buildx 自动注入 TARGETOS/TARGETARCH；纯 docker build 默认 linux/amd64）
ARG TARGETOS=linux
ARG TARGETARCH=amd64
ARG VERSION=1.0.2
ARG BUILD_TIME=unknown
ARG GIT_COMMIT=unknown

RUN echo "[build] target=${TARGETOS}/${TARGETARCH} CGO_ENABLED=0 version=${VERSION}" \
 && mkdir -p /out/bin \
 && CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
    go build -trimpath \
    -ldflags "-s -w -X drill-system/internal/version.Version=${VERSION} -X drill-system/internal/version.BuildTime=${BUILD_TIME} -X drill-system/internal/version.GitCommit=${GIT_COMMIT}" \
    -o /out/bin/drill-server . \
 && CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
    go build -trimpath \
    -ldflags "-s -w" \
    -o /out/bin/drill-migrate ./cmd/migrate \
 && CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
    go build -trimpath \
    -ldflags "-s -w" \
    -o /out/bin/drill-seed ./cmd/seed \
 && echo "[build] DONE: $(ls -lh /out/bin/)"

# ---------- Stage 2: Runtime ----------
FROM ${RUNTIME_BASE} AS runtime

# alpine apk add 也需要代理（UOS Server 20 宿主 VMware NAT 场景下 alpine 默认源拉不动）
ARG HTTP_PROXY
ARG HTTPS_PROXY
ARG NO_PROXY
ENV HTTP_PROXY=${HTTP_PROXY}
ENV HTTPS_PROXY=${HTTPS_PROXY}
ENV NO_PROXY=${NO_PROXY}

ENV TZ=Asia/Shanghai
ENV APP_ROOT=/opt/drill

# 运行时依赖：
#   tini            —— init 进程，正确转发 SIGTERM
#   wget            —— entrypoint 等 MySQL 探测 TCP + HEALTHCHECK
#   ca-certificates —— HTTPS 出站（如果将来要连外部服务）
#   tzdata          —— 时区支持，配合 TZ=Asia/Shanghai
# mariadb-client  —— mysqldump / mysql 客户端，备份与恢复必需（P0-2）
RUN apk add --no-cache tini wget ca-certificates tzdata mariadb-client \
 && cp /usr/share/zoneinfo/Asia/Shanghai /etc/localtime \
 && echo "Asia/Shanghai" > /etc/timezone \
 && addgroup -S -g 10001 drill \
 && adduser  -S -u 10001 -G drill -h /opt/drill -s /sbin/nologin drill

# 目录结构
RUN mkdir -p /opt/drill/bin \
              /opt/drill/public \
              /opt/drill/migrations \
              /opt/drill/data \
              /opt/drill/uploads \
              /opt/drill/etc \
 && chown -R drill:drill /opt/drill

WORKDIR /opt/drill

# --- 二进制 + 静态资源 + 迁移脚本 ---
# 注意：故意不使用 COPY --chown / --chmod（旧 builder 不生效），全靠后面的 RUN chown + chmod
COPY --from=builder /out/bin/drill-server  /opt/drill/bin/drill-server
COPY --from=builder /out/bin/drill-migrate /opt/drill/bin/drill-migrate
COPY --from=builder /out/bin/drill-seed    /opt/drill/bin/drill-seed
COPY --from=builder /src/public/           /opt/drill/public/
COPY --from=builder /src/migrations/       /opt/drill/migrations/

# --- 入口脚本 ---
COPY deploy/docker-dist/docker-entrypoint.sh /opt/drill/docker-entrypoint.sh

# --- 权限统一修正（保证所有 builder 版本下都生效）---
RUN chown -R drill:drill /opt/drill \
 && chmod 755 /opt/drill/bin/drill-server \
 && chmod 755 /opt/drill/bin/drill-migrate \
 && chmod 755 /opt/drill/bin/drill-seed \
 && chmod 755 /opt/drill/docker-entrypoint.sh \
 && find /opt/drill/public -type d -exec chmod 755 {} \; \
 && find /opt/drill/public -type f -exec chmod 644 {} \; \
 && find /opt/drill/migrations -type f -name '*.sql' -exec chmod 644 {} \; \
 && echo "[runtime] permissions OK"

# --- 容器内运行时代理（可选，默认不设；用户 compose 里可覆盖）---
ENV HTTP_PROXY=
ENV HTTPS_PROXY=

# --- OCI Label —— 给发布用的元数据（docs 里说明了如何 build 时注入 revision）---
# 用户可以在 build 时 --label org.opencontainers.image.revision=$(git rev-parse HEAD)
# 或在 .env / docker-compose.yml 里用 build.args 注入 VERSION
ARG VERSION=1.0.2
ARG VCS_REF=unknown
ARG BUILD_TIME=unknown
# 发布元数据：全部可由 --build-arg 覆盖，避免源码里残留占位值
ARG SOURCE_URL=https://github.com/chenjinxiang1980/cyber-drill
ARG KEYFILES_SHA256=unknown

LABEL org.opencontainers.image.title="cyber-drill-system" \
      org.opencontainers.image.description="示例医院网络安全应急演练系统 —— Drill Server" \
      org.opencontainers.image.version=${VERSION} \
      org.opencontainers.image.source=${SOURCE_URL} \
      org.opencontainers.image.licenses="MIT" \
      org.opencontainers.image.revision=${VCS_REF} \
      org.opencontainers.image.created=${BUILD_TIME} \
      org.opencontainers.image.authors="Cyber-Drill Team" \
      # 自定义标签：让使用者能一眼看到这份镜像对应哪些关键文件（签名文件末行哈希）
      drill.system.keyfiles.sha256=${KEYFILES_SHA256}

EXPOSE 8090

HEALTHCHECK --interval=30s --timeout=5s --start-period=20s --retries=3 \
    CMD wget -qO- http://127.0.0.1:8090/api/health || exit 1

USER drill:drill

# tini 作为 init：正确转发容器停止信号到 drill-server
ENTRYPOINT ["/sbin/tini", "--", "/opt/drill/docker-entrypoint.sh"]
CMD ["/opt/drill/bin/drill-server", "-root=/opt/drill"]
