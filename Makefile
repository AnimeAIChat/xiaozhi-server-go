# Go parameters
GOCMD = go
GOBUILD = $(GOCMD) build
GOCLEAN = $(GOCMD) clean
BINARY_NAME = xiaozhi-server
BINARY_PATH = ./src/main.go
OUTPUT_DIR = bin

# Default target
all: build-all

# Build for the default platform (e.g., Ubuntu)
build:
	$(GOBUILD) -o $(BINARY_NAME) -v $(BINARY_PATH)

# Cross-compilation for different platforms

build-linux:
	mkdir -p $(OUTPUT_DIR)
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 $(GOBUILD) -o $(OUTPUT_DIR)/$(BINARY_NAME)-linux -v $(BINARY_PATH)

build-windows:
	mkdir -p $(OUTPUT_DIR)
	GOOS=windows GOARCH=amd64 CGO_ENABLED=0 $(GOBUILD) -o $(OUTPUT_DIR)/$(BINARY_NAME)-windows.exe -v $(BINARY_PATH)

build-arm64:
	mkdir -p $(OUTPUT_DIR)
	GOOS=linux GOARCH=arm64 CGO_ENABLED=0 $(GOBUILD) -o $(OUTPUT_DIR)/$(BINARY_NAME)-arm64 -v $(BINARY_PATH)

build-macos:
	mkdir -p $(OUTPUT_DIR)
	GOOS=darwin GOARCH=amd64 CGO_ENABLED=0 $(GOBUILD) -o $(OUTPUT_DIR)/$(BINARY_NAME)-macos -v $(BINARY_PATH)

build-macos-arm64:
	mkdir -p $(OUTPUT_DIR)
	GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 $(GOBUILD) -o $(OUTPUT_DIR)/$(BINARY_NAME)-macos-arm64 -v $(BINARY_PATH)

# Build for all platforms
build-all: build-linux build-windows build-arm64 build-macos build-macos-arm64

# Clean up generated files
clean:
	$(GOCLEAN)
	rm -f $(BINARY_NAME)
	rm -rf $(OUTPUT_DIR)

# Run the default build on the local machine
run:
	$(GOBUILD) -o $(BINARY_NAME) -v $(BINARY_PATH)
	./$(BINARY_NAME)
