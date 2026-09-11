# Use a smaller base image for the build stage
#
# 【--platform=$BUILDPLATFORM 不能删】没有它，buildx 构建 linux/arm64 时会把
# 整个 builder 阶段（连同 Go 编译器）放进 QEMU 模拟里跑。踩过：v1.0.11 的
# Release 卡在这一步两小时没出来，而它本该是两分钟的事。
# 加上之后 builder 始终跑在原生架构上，靠下面的 GOARCH=${TARGETARCH} 交叉编译
# ——CGO_ENABLED=0 的 Go 本来就不需要模拟。
FROM --platform=$BUILDPLATFORM golang:1.27.1-alpine AS builder

LABEL stage=gobuilder

ARG TARGETARCH
ARG VERSION=unknown
ARG CHANNEL=dev
ENV CGO_ENABLED=0 GOOS=linux GOARCH=${TARGETARCH}

# Combine apk commands into one to reduce layer size
RUN apk update --no-cache && apk add --no-cache tzdata ca-certificates

WORKDIR /build

# Copy go.mod and go.sum first to take advantage of Docker caching
COPY go.mod go.sum ./
RUN go mod download

# Copy the rest of the application code
COPY . .

# Build the binary with version and build time
RUN BUILD_TIME=$(date -u +"%Y-%m-%d %H:%M:%S") && \
    go build -ldflags="-s -w -X 'github.com/perfect-panel/server/internal/app/buildinfo.Version=${VERSION}' -X 'github.com/perfect-panel/server/internal/app/buildinfo.BuildTime=${BUILD_TIME}' -X 'github.com/perfect-panel/server/internal/app/buildinfo.Channel=${CHANNEL}'" -o /app/ppanel main.go

# Final minimal image
FROM scratch

# Copy CA certificates and timezone data
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=builder /usr/share/zoneinfo/Asia/Shanghai /usr/share/zoneinfo/Asia/Shanghai

ENV TZ=Asia/Shanghai

# Set working directory and copy binary
WORKDIR /app

COPY --from=builder /app/ppanel /app/ppanel
COPY --from=builder /build/etc /app/etc

# Expose the port (optional)
EXPOSE 8080

# Specify entry point
ENTRYPOINT ["/app/ppanel"]
CMD ["run", "--config", "etc/ppanel.yaml"]
