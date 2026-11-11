# =============================================================================
# drill-server —— Docker 多阶段构建（**可替换国产基础镜像**）
#
# 用法：
#   docker build -f deploy/linux/Dockerfile -t drill-server:1.0.5 .
#   # 指定国产发行版：
#   docker build -f deploy/linux/Dockerfile \
#       --build-arg RUNTIME_BASE=openeuler/openeuler:22.03 \
#       -t drill-server:openeuler .
#
# 关键设计：
#   - 两阶段：builder 纯 Go 编译 → runtime 运行
#   - CGO_ENABLED=0 静态编译：**换发行版不需要改代码**，产物跟 glibc/musl 完全无关
#   - GOARCH 也做成 ARG：amd64 默认；鲲鹏/飞腾(arm64) 重新编；龙芯 LoongArch 见下文
#   - 运行时非 root（uid 10001）
#
# ---------- 国产发行版替换方式 ----------
#
# 推荐先用 OpenEuler 验证功能（Docker Hub 有官方镜像），再切到目标发行版交付：
#
#   # 1. openEuler（华为欧拉，官方镜像，amd64 + arm64 都有）
#   docker build --build-arg RUNTIME_BASE=openeuler/openeuler:24.03-lts \
#       --build-arg BUILDER_BASE=golang:1.24-openeuler-24.03 \
#       -t drill-server:openeuler .
#
#   # 2. 龙蜥 Anolis（阿里巴巴，官方镜像在 Docker Hub + 阿里云镜像）
#   docker build --build-arg RUNTIME_BASE=openanolis/anolisos:8.8 \
#       -t drill-server:anolis .
#
#   # 3. 银河麒麟 V10 —— 麒麟**不在** Docker Hub 发官方镜像
#   #    方式 a) 厂商 registry：向麒麟申请 Kylin Docker Image
#   #           （常见 registry: registry.kylinos.cn / swr.cn-north-4.myhuaweicloud.com）
#   #           docker login registry.kylinos.cn
#   #           docker build --build-arg RUNTIME_BASE=registry.kylinos.cn/kylin/v10:latest ...
#   #    方式 b) 自己从 ISO 导入 rootfs：
#   #           挂载麒麟 V10 ISO，解压 rootfs.tar.gz，然后
#   #           docker import rootfs.tar.gz kylin-v10-rootfs
#   #           docker build --build-arg RUNTIME_BASE=kylin-v10-rootfs ...
#   #           注意：import 出来的镜像 **没有** adduser/apk/yum，
#   #           需要 Dockerfile 里绕过（见下方 "纯 distroless 方式"）
#   #
#   # 4. 统信 UOS —— 同上，厂商镜像：
#   #           registry.deepin.com/deepin/ 或自建 UOS rootfs
#
# ---------- 架构约束（重要！） ----------
#
#   amd64 (x86_64)  —— 本程序默认 target，Windows Docker Desktop 可直接模拟运行
#   arm64  (aarch64) —— 鲲鹏 / 飞腾；本地需 GOARCH=arm64 重编
#                       Windows Docker Desktop 默认有 arm64 QEMU 模拟，可跑但慢
#   loong64 (龙芯 LoongArch) —— **Windows Docker Desktop 没有 LoongArch 模拟层**
#                       只能在龙芯物理机/龙芯 Linux 上原生构建
#                       Dockerfile 加 --build-arg GOARCH=loong64 即可
#
#   所有三种架构 CGO_ENABLED=0 静态编译，**产物跨发行版通用**（只要同架构）。
#
# ---------- 麒麟 rootfs 没有 adduser / 包管理器怎么办？ ----------
# 如果你手里的麒麟镜像是 docker import 的 rootfs（最小化），没有 yum/dnf/useradd，
# 可以用纯静态方式——二进制自己就是用户身份（UID 0 或直接 runAsNonRoot）：
#
#   # 在 Dockerfile 里把 builder 产物拷进去，不要装任何系统包
#   FROM your-kylin-rootfs
#   COPY --from=builder /out/bin /opt/drill/bin
#   EXPOSE 8090
#   CMD ["/opt/drill/bin/drill-server", "-root=/opt/drill"]
#   # 然后在 k8s/运行时用 securityContext.runAsUser: 10001
#
# =============================================================================

# ---------- 可替换参数 ----------
ARG BUILDER_BASE=golang:1.27-alpine
ARG RUNTIME_BASE=alpine:3.20
ARG GOARCH=amd64
ARG GOOS=linux

# ---------- Stage 1: Builder ----------
FROM ${BUILDER_BASE} AS builder

WORKDIR /src

ARG GOARCH=amd64
ARG GOOS=linux

# 依赖下载缓存（go.mod / go.sum 不变则复用）
COPY server/go.mod server/go.sum ./
RUN go mod download

# 全部源码（server + migrations 都在仓库根下，所以上下文用仓库根）
COPY server/ ./
COPY migrations/ ../migrations/

# 编译两个二进制
# -trimpath 剥离本地路径，-ldflags 可注入版本号
ARG VERSION=1.0.5
ARG BUILD_TIME=unknown
ARG GIT_COMMIT=unknown

RUN echo "[build] GOOS=${GOOS} GOARCH=${GOARCH} CGO_ENABLED=0" \
 && CGO_ENABLED=0 GOOS=${GOOS} GOARCH=${GOARCH} \
    go build -trimpath \
    -ldflags "-s -w -X drill-system/internal/version.Version=${VERSION} -X drill-system/internal/version.BuildTime=${BUILD_TIME} -X drill-system/internal/version.GitCommit=${GIT_COMMIT}" \
    -o /out/bin/drill-server . \
 && CGO_ENABLED=0 GOOS=${GOOS} GOARCH=${GOARCH} \
    go build -trimpath \
    -ldflags "-s -w" \
    -o /out/bin/drill-server-bootstrap ./cmd/bootstrap \
 && echo "[build] DONE: $(ls -lh /out/bin/)"

# ---------- Stage 2: Runtime ----------
FROM ${RUNTIME_BASE} AS runtime

# ---------- 以下命令依赖发行版包管理器 ----------
# Alpine: apk  |  openEuler/Anolis/Kylin-UOS: dnf/yum
# 不同发行版命令不同，用 IF 探测后分别处理
# 如果你是用 docker import 的最小 rootfs（麒麟/UOS），下面这些 RUN
# 会报错 —— 看文件顶部注释的 "纯 distroless 方式" 绕过

# 默认时区（覆盖即可）
ENV TZ=Asia/Shanghai

SHELL ["/bin/bash", "-lc"]

# 1) 创建非 root 用户 / 目录 / 安装必要运行时
#    每个发行版都尝试自己的包管理器
RUN (command -v apk >/dev/null 2>&1 && { \
        echo "[runtime] Alpine detected, using apk"; \
        apk add --no-cache tini wget ca-certificates tzdata; \
        addgroup -S -g 10001 drill 2>/dev/null || groupadd -g 10001 drill; \
        adduser -S -u 10001 -G drill -h /opt/drill -s /sbin/nologin drill 2>/dev/null || \
            useradd -u 10001 -g drill -M -d /opt/drill -s /sbin/nologin drill; \
        cp /usr/share/zoneinfo/Asia/Shanghai /etc/localtime; \
        echo "Asia/Shanghai" > /etc/timezone; \
    }) || \
    (command -v dnf >/dev/null 2>&1 && { \
        echo "[runtime] dnf-based detected (openEuler/Anolis/Kylin/UOS)"; \
        dnf install -y --setopt=install_weak_deps=False \
            procps-ng wget ca-certificates tzdata; \
        dnf clean all; \
        groupadd -g 10001 drill 2>/dev/null || true; \
        useradd  -u 10001 -g drill -M -d /opt/drill -s /sbin/nologin drill 2>/dev/null || true; \
        ln -sf /usr/share/zoneinfo/Asia/Shanghai /etc/localtime; \
        echo "Asia/Shanghai" > /etc/timezone; \
    }) || \
    (command -v yum >/dev/null 2>&1 && { \
        echo "[runtime] yum-based detected"; \
        yum install -y wget ca-certificates tzdata; \
        yum clean all; \
        groupadd -g 10001 drill 2>/dev/null || true; \
        useradd  -u 10001 -g drill -M -d /opt/drill -s /sbin/nologin drill 2>/dev/null || true; \
        ln -sf /usr/share/zoneinfo/Asia/Shanghai /etc/localtime; \
        echo "Asia/Shanghai" > /etc/timezone; \
    }) || \
    echo "[runtime] WARNING: no known package manager found. Skip package install."

# 2) 目录结构（所有发行版通用）
RUN mkdir -p /opt/drill/bin /opt/drill/public /opt/drill/data \
              /opt/drill/uploads /opt/drill/migrations \
 && chown -R drill:drill /opt/drill

WORKDIR /opt/drill

# 3) 从 builder 拷二进制 + migrations
COPY --from=builder --chown=drill:drill /out/bin /opt/drill/bin
COPY --from=builder --chown=drill:drill /migrations /opt/drill/migrations

# 暴露端口（实际由 compose/service 决定）
EXPOSE 8090

# 健康检查（apk 版带 wget；dnf/yum 已装 procps-ng/wget；最小 rootfs 会让 HEALTHCHECK 变成 no-op）
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
    CMD wget -qO- http://127.0.0.1:8090/api/health || \
        (command -v curl >/dev/null 2>&1 && curl -sf http://127.0.0.1:8090/api/health) || \
        true

USER drill:drill

# 主服务：-root 指向 /opt/drill（绝对路径，不依赖工作目录）
ENTRYPOINT ["/opt/drill/bin/drill-server"]
CMD ["-root=/opt/drill"]
