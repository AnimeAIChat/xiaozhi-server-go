# Makefile for building the Go project

# Go parameters
GOCMD=go
GOBUILD=$(GOCMD) build
GOCLEAN=$(GOCMD) clean
GOTEST=$(GOCMD) test
GOGET=$(GOCMD) get
BINARY_NAME=webServer
BINARY_PATH=./cmd/main.go

all: test build

build:
	$(GOBUILD) -o $(BINARY_NAME) -v $(BINARY_PATH)

clean:
	$(GOCLEAN)
	rm -f $(BINARY_NAME)

test:
	$(GOTEST) -v ./...

run:
	$(GOBUILD) -o $(BINARY_NAME) -v $(BINARY_PATH)
	./$(BINARY_NAME)