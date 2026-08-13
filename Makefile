BINARY := codedocs
DIST := dist
CMD := ./cmd/codedocs
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -s -w -X codedocs/internal/config.Version=$(VERSION)

.PHONY: all build test test-race vet run windows-resources windows build-all clean release help

all: test vet build

build:
	go build -trimpath -ldflags="$(LDFLAGS)" -o $(BINARY) $(CMD)

test:
	go test -count=1 ./...

test-race:
	go test -race -count=1 ./...

vet:
	go vet ./...

run:
	go run $(CMD)

windows-resources:
	go run github.com/tc-hib/go-winres@v0.3.3 make --in winres/winres.json --arch amd64,arm64 --out cmd/codedocs/rsrc

windows: windows-resources
	mkdir -p $(DIST)
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -trimpath -ldflags="-H=windowsgui $(LDFLAGS)" -o $(DIST)/codedocs_windows_amd64.exe $(CMD)
	CGO_ENABLED=0 GOOS=windows GOARCH=arm64 go build -trimpath -ldflags="-H=windowsgui $(LDFLAGS)" -o $(DIST)/codedocs_windows_arm64.exe $(CMD)

build-all: windows
	CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build -trimpath -ldflags="$(LDFLAGS)" -o $(DIST)/codedocs_darwin_arm64 $(CMD)
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags="$(LDFLAGS)" -o $(DIST)/codedocs_linux_amd64 $(CMD)

clean:
	rm -rf $(BINARY) $(BINARY).exe $(DIST)

release:
	@test -n "$(V)" || (echo "Usage: make release V=v1.8.0" && exit 1)
	git tag -a "$(V)" -m "CodeDocs $(V)"
	git push origin "$(V)"

help:
	@echo "make test | test-race | vet | build | windows | build-all | release V=vX.Y.Z"
