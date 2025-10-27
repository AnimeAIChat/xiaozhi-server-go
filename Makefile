# Makefile for building the Go project

# Go parameters
GOCMD=go
GOBUILD=$(GOCMD) build
GOCLEAN=$(GOCMD) clean
BINARY_NAME=xiaozhi-server
BINARY_PATH=./src/main.go

BUILD_DEPS := swag

all: build

build: $(BUILD_DEPS)
	$(GOBUILD) -o $(BINARY_NAME) -v $(BINARY_PATH)

clean:
	$(GOCLEAN)
	rm -f $(BINARY_NAME)

run: $(BUILD_DEPS)
	$(GOCMD) run ./src/main.go

# 生成 Swagger 文档（可在 Makefile 外部通过 SWAG_AUTO 控制是否运行）
swag:
	@cd src && swag init -g main.go || (echo "swag init failed, continuing..." && exit 0)

.PHONY: all build clean run swag
