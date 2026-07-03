# Kashy — Build & Release
# Usage:
#   make build          — build kashy.exe for current platform
#   make test           — run all tests
#   make release V=1.1.0 — bump version, tag, build release binary

MODULE  = github.com/nicodolas/kashy
PKG     = ./cmd/kashy/
BINARY  = kashy.exe
BIN_DIR = $(USERPROFILE)\bin

# Injected at build time
GIT_COMMIT = $(shell git rev-parse --short HEAD 2>NUL || echo dev)
BUILD_DATE = $(shell powershell -Command "Get-Date -Format 'yyyy-MM-dd'" 2>NUL || echo unknown)
LDFLAGS    = -s -w \
  -X $(MODULE)/internal/version.GitCommit=$(GIT_COMMIT) \
  -X $(MODULE)/internal/version.BuildDate=$(BUILD_DATE)

.PHONY: build test release install clean version

## build: compile kashy binary
build:
	go build -ldflags="$(LDFLAGS)" -o $(BINARY) $(PKG)
	@echo Built $(BINARY) [$(GIT_COMMIT), $(BUILD_DATE)]

## test: run full test suite
test:
	go test ./... -count=1

## install: copy binary to ~/bin
install: build
	copy /Y $(BINARY) "$(BIN_DIR)\$(BINARY)"
	@echo Installed to $(BIN_DIR)\$(BINARY)

## version: show current version
version:
	@powershell -Command "(Get-Content internal\version\version.go | Select-String 'Major|Minor|Patch') -join ' '"
	@.\$(BINARY) --version 2>NUL || echo "(build first)"

## clean: remove build artifacts
clean:
	del /Q $(BINARY) 2>NUL
	go clean -cache

## release V=x.y.z: bump version, create git tag, build release binary
## Example: make release V=1.1.0
release:
	@if "$(V)"=="" (echo "Usage: make release V=x.y.z" && exit 1)
	@echo Releasing v$(V)...
	@powershell -Command "\
		$$content = Get-Content internal\version\version.go -Raw; \
		$$parts = '$(V)'.Split('.'); \
		$$content = $$content -replace 'Major = \d+', ('Major = ' + $$parts[0]); \
		$$content = $$content -replace 'Minor = \d+', ('Minor = ' + $$parts[1]); \
		$$content = $$content -replace 'Patch = \d+', ('Patch = ' + $$parts[2]); \
		Set-Content internal\version\version.go $$content; \
		Write-Host 'Version bumped to $(V)'"
	go build -ldflags="$(LDFLAGS)" -o $(BINARY) $(PKG)
	@echo Built release binary
	@echo Next steps:
	@echo   1. Update CHANGELOG.md
	@echo   2. git add -A
	@echo   3. git commit -m "chore: release v$(V)"
	@echo   4. git tag v$(V)
	@echo   5. git push origin main --tags
