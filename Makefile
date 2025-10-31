# Makefile for building the Go project

# Go parameters
GOCMD=go
GOBUILD=$(GOCMD) build
GOCLEAN=$(GOCMD) clean
MAIN_PKG=./cmd/xiaozhi-server
BINARY_NAME=xiaozhi-server
SWAG_MAIN=main.go
SWAG_DIRS=cmd/xiaozhi-server,src/httpsvr/webapi,src/httpsvr/vision,src/httpsvr/ota,src/models
SWAG_OUT=internal/platform/docs
SWAG_FLAGS=--parseDependency=false --parseGoList=false

BUILD_DEPS := swag

all: build

build: $(BUILD_DEPS)
	$(GOBUILD) -o $(BINARY_NAME) -v $(MAIN_PKG)

clean:
	$(GOCLEAN)
	rm -f $(BINARY_NAME)

run: $(BUILD_DEPS)
	$(GOCMD) run $(MAIN_PKG)

test:
	$(GOCMD) test ./...

# 生成 Swagger 文档；若未安装 swag 或失败，忽略错误继续
swag:
	swag init -g $(SWAG_MAIN) -d $(SWAG_DIRS) -o $(SWAG_OUT) $(SWAG_FLAGS) || (echo "swag init failed, continuing..." && exit 0)

.PHONY: all build clean run test swag
