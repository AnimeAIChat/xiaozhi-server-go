# Makefile for building the Go project

# Go parameters
GOCMD=go
GOBUILD=$(GOCMD) build
GOCLEAN=$(GOCMD) clean
MAIN_PKG=./src
BINARY_NAME=xiaozhi-server
SWAG_MAIN=main.go
SWAG_DIRS=src,src/httpsvr/webapi,src/httpsvr/vision,src/httpsvr/ota,src/models
SWAG_OUT=src/docs
SWAG_FLAGS=--parseDependency=false --parseGoList=false

BUILD_DEPS := swag deps

all: build

# 安装依赖
deps:
	$(GOCMD) mod download
	$(GOCMD) mod tidy

# 检查依赖是否缺失，如果缺失则安装
check-deps:
	@$(GOCMD) list -m github.com/joho/godotenv > /dev/null || $(GOCMD) get github.com/joho/godotenv
	@$(GOCMD) list -m github.com/swaggo/files > /dev/null || $(GOCMD) get github.com/swaggo/files
	@$(GOCMD) list -m github.com/swaggo/gin-swagger > /dev/null || $(GOCMD) get github.com/swaggo/gin-swagger

build: check-deps $(BUILD_DEPS)
	$(GOBUILD) -o $(BINARY_NAME) -v $(MAIN_PKG)

clean:
	$(GOCLEAN)
	rm -f $(BINARY_NAME)

run: check-deps $(BUILD_DEPS)
	$(GOCMD) run $(MAIN_PKG)

test:
	$(GOCMD) test ./...

# 生成 Swagger 文档；若未安装 swag 或失败，忽略错误继续
swag:
	swag init -g $(SWAG_MAIN) -d $(SWAG_DIRS) -o $(SWAG_OUT) $(SWAG_FLAGS) || (echo "swag init failed, continuing..." && exit 0)

# 格式化代码
fmt:
	$(GOCMD) fmt ./...

# 检查代码
vet:
	$(GOCMD) vet ./...

# 完整的代码检查
check: fmt vet test

# 构建发布版本
release: check-deps $(BUILD_DEPS)
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 $(GOBUILD) -o $(BINARY_NAME)-linux-amd64 -v $(MAIN_PKG)
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 $(GOBUILD) -o $(BINARY_NAME)-windows-amd64.exe -v $(MAIN_PKG)
	CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 $(GOBUILD) -o $(BINARY_NAME)-darwin-amd64 -v $(MAIN_PKG)

# 帮助信息
help:
	@echo "可用命令:"
	@echo "  make build     - 编译项目"
	@echo "  make run       - 运行项目"
	@echo "  make clean     - 清理编译文件"
	@echo "  make test      - 运行测试"
	@echo "  make fmt       - 格式化代码"
	@echo "  make vet       - 检查代码"
	@echo "  make check     - 完整代码检查"
	@echo "  make release   - 构建发布版本"
	@echo "  make swag      - 生成Swagger文档"
	@echo "  make deps      - 安装依赖"
	@echo "  make help      - 显示帮助信息"

.PHONY: all build clean run test swag fmt vet check release deps check-deps help
