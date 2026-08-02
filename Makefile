# ==============================================================================
# Codebase-to-Docs Generator (codedocs) Makefile
# ==============================================================================

BINARY_NAME=codedocs
DIST_DIR=dist
CMD_PATH=./cmd/codedocs

.PHONY: all build build-all test vet run clean help

all: test build

## build: Build single binary for host OS
build:
	@echo "🔨 Building $(BINARY_NAME)..."
	go build -ldflags="-s -w" -o $(BINARY_NAME) $(CMD_PATH)
	@echo "✅ Build complete: ./$(BINARY_NAME)"

## build-all: Cross-compile binaries for Windows, macOS, and Linux (amd64 + arm64)
build-all: clean
	@echo "🚀 Cross-compiling for all target platforms..."
	@mkdir -p $(DIST_DIR)
	
	# Windows
	GOOS=windows GOARCH=amd64 go build -ldflags="-s -w" -o $(DIST_DIR)/$(BINARY_NAME)_windows_amd64.exe $(CMD_PATH)
	GOOS=windows GOARCH=arm64 go build -ldflags="-s -w" -o $(DIST_DIR)/$(BINARY_NAME)_windows_arm64.exe $(CMD_PATH)
	
	# macOS (Darwin)
	GOOS=darwin GOARCH=amd64 go build -ldflags="-s -w" -o $(DIST_DIR)/$(BINARY_NAME)_darwin_amd64 $(CMD_PATH)
	GOOS=darwin GOARCH=arm64 go build -ldflags="-s -w" -o $(DIST_DIR)/$(BINARY_NAME)_darwin_arm64 $(CMD_PATH)
	
	# Linux
	GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o $(DIST_DIR)/$(BINARY_NAME)_linux_amd64 $(CMD_PATH)
	GOOS=linux GOARCH=arm64 go build -ldflags="-s -w" -o $(DIST_DIR)/$(BINARY_NAME)_linux_arm64 $(CMD_PATH)
	
	@echo "✅ All binaries compiled in $(DIST_DIR)/"

## test: Run unit tests
test:
	@echo "🧪 Running unit tests..."
	go test -v ./...

## vet: Run go vet static analysis
vet:
	@echo "🔍 Running go vet..."
	go vet ./...

## run: Run application locally
run:
	go run $(CMD_PATH)

## clean: Remove build artifacts and temp files
clean:
	@echo "🧹 Cleaning build artifacts..."
	@rm -rf $(BINARY_NAME) $(BINARY_NAME).exe $(DIST_DIR) temp_docs
	@echo "✅ Cleaned."

## release: Tag and publish a new GitHub Release (e.g. make release V=v1.0.1)
release:
	@if [ -z "$(V)" ]; then echo "❌ Error: Please specify version, e.g. make release V=v1.0.1"; exit 1; fi
	@echo "🚀 Creating git release tag $(V)..."
	git tag $(V)
	git push origin $(V)
	@echo "✅ Release tag $(V) pushed! GitHub Actions is building and uploading release binaries."

## help: Show Makefile target help
help:
	@echo "Available Makefile targets:"
	@grep -E '^## ' $(MAKEFILE_LIST) | sed -e 's/## //'
