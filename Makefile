# Makefile for building the Go project

# Go parameters
GOCMD=go
GOBUILD=$(GOCMD) build
GOCLEAN=$(GOCMD) clean
BINARY_NAME=xiaozhi-server
BINARY_PATH=./src/main.go

all: build

build:
	$(GOBUILD) -o $(BINARY_NAME) -v $(BINARY_PATH)

clean:
	$(GOCLEAN)
	rm -f $(BINARY_NAME)

run:
	$(GOBUILD) -o $(BINARY_NAME) -v $(BINARY_PATH)
	./$(BINARY_NAME)
