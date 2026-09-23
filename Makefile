# The MCP server is pure Go. The plug-in links libgimp and therefore needs
# cgo, GIMP's headers and a matching pkg-config path.
GIMP_PREFIX ?= $(shell $(MAKE) -s gimp-prefix)

.PHONY: help
help:
	@echo "build          build the MCP server"
	@echo "build-plugin   build the GIMP plug-in (needs libgimp)"
	@echo "install-plugin build and install the plug-in into GIMP"
	@echo "test           run all tests"
	@echo "lint           run golangci-lint over both modules"
	@echo "tidy           tidy both modules"
	@echo "introspect     snapshot what the running GIMP accepts (needs GIMP + plug-in)"
	@echo "verify-tools   check the tool definitions against the GIMP snapshot"
	@echo "snapshot       build a local goreleaser snapshot of the server"
	@echo "plugin-snapshot build the downloadable plug-in archive for this host"
	@echo "clean          remove build output"

# gimp-prefix locates GIMP's pkgconfig directory on macOS and Linux.
.PHONY: gimp-prefix
gimp-prefix:
	@if [ -d /Applications/GIMP.app/Contents/lib/pkgconfig ]; then \
		echo /Applications/GIMP.app/Contents; \
	else \
		echo /usr; \
	fi

.PHONY: build
build:
	go build -o mcp-gimp ./cmd/mcp-gimp

.PHONY: build-plugin
build-plugin:
	cd plugin && \
		PKG_CONFIG_PATH="$(GIMP_PREFIX)/lib/pkgconfig:$$PKG_CONFIG_PATH" \
		CGO_ENABLED=1 \
		CGO_LDFLAGS="-Wl,-rpath,$(GIMP_PREFIX)/lib" \
		go build -o ../gimp-mcp-plugin .

# GIMP loads a plug-in from a directory named after the executable.
.PHONY: install-plugin
install-plugin: build-plugin
	@dir="$$(./scripts/gimp-plugin-dir.sh)"; \
	mkdir -p "$$dir/gimp-mcp-plugin" && \
	cp gimp-mcp-plugin "$$dir/gimp-mcp-plugin/" && \
	chmod +x "$$dir/gimp-mcp-plugin/gimp-mcp-plugin" && \
	echo "installed to $$dir/gimp-mcp-plugin"

.PHONY: test
test:
	go test -race -coverprofile coverage.out ./...
	cd plugin && \
		PKG_CONFIG_PATH="$(GIMP_PREFIX)/lib/pkgconfig:$$PKG_CONFIG_PATH" \
		CGO_LDFLAGS="-Wl,-rpath,$(GIMP_PREFIX)/lib" \
		go test -race ./internal/commands/... ./internal/protocol/... ./internal/socket/...

.PHONY: lint
lint:
	golangci-lint run ./...
	cd plugin && PKG_CONFIG_PATH="$(GIMP_PREFIX)/lib/pkgconfig:$$PKG_CONFIG_PATH" golangci-lint run ./...

.PHONY: tidy
tidy:
	go mod tidy
	cd plugin && go mod tidy

# introspect asks the running GIMP what every procedure and operation the
# plug-in calls accepts, and writes it to internal/snapshot/gimp.json. GIMP must be
# running with the MCP plug-in started.
.PHONY: introspect
introspect:
	go run ./internal/snapshot/introspect

# verify-tools checks the tool definitions against the committed snapshot. It
# needs no GIMP, and runs as part of `make test`.
.PHONY: verify-tools
verify-tools:
	go test ./internal/server -run 'Snapshot|Constraint|CompareToGIMP' 

.PHONY: cover
cover:
	gocover-cobertura < coverage.out > coverage.xml

# PLUGIN_TARGET selects which plug-in build to run; it must match the host,
# because cgo against libgimp cannot cross-compile.
PLUGIN_TARGET ?= $(shell go env GOOS)_$(shell go env GOARCH)

.PHONY: snapshot
snapshot:
	goreleaser --clean --snapshot

# Builds the downloadable plug-in archive for this machine.
.PHONY: plugin-snapshot
plugin-snapshot:
	PLUGIN_TARGET="$(PLUGIN_TARGET)" \
		PKG_CONFIG_PATH="$(GIMP_PREFIX)/lib/pkgconfig:$$PKG_CONFIG_PATH" \
		CGO_ENABLED=1 \
		CGO_LDFLAGS="-Wl,-rpath,$(GIMP_PREFIX)/lib" \
		goreleaser release -f .goreleaser.plugin.yml --clean --snapshot --skip=publish

.PHONY: release
release:
	goreleaser --clean

.PHONY: clean
clean:
	rm -rf dist/
	rm -f mcp-gimp gimp-mcp-plugin coverage.out coverage.xml

.PHONY: install-go-tools
install-go-tools:
	go install -v github.com/t-yuki/gocover-cobertura@latest
