FROM alpine:latest

# 安装运行时依赖
RUN apk add --no-cache \
    ca-certificates \
    sqlite-libs \
    procps

WORKDIR /app

CMD ["./xiaozhi-server"]
