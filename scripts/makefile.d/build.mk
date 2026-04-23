##@ Build
## Go Sources
GO_SOURCES = $(shell find . -name "vendor"  -prune -o \
                            -type f -name "*.go" -print)

WATCHED_FILES+= $(GO_SOURCES) go.mod go.sum
build/docker-image: $(GO_SOURCES)

.PHONY: build
build: build/$(APP_NAME) ## Build web app

.PHONY: test
test: fmt vet ## Run test
	go test -tags noembed -v ./...

build/$(APP_NAME): $(GO_SOURCES) $(MAKEFILE_LIST) fmt vet
	@$(MAKE) build/tools/go
	@mkdir -p build
	go build -v \
		-trimpath \
		-ldflags "\
			-X '$(IMPORT_PATH)/internal/buildinfo.AppName=$(APP_NAME)' \
			-X '$(IMPORT_PATH)/internal/buildinfo.Version=$(VERSION)' \
			-X '$(IMPORT_PATH)/internal/buildinfo.BuildTime=$(shell date --rfc-3339=ns)' \
		" \
		$(OPTIONAL_BUILD_ARGS) \
		-o $@ .

build/$(APP_NAME)-$(VERSION): $(GO_SOURCES) $(MAKEFILE_LIST) fmt vet
	@$(MAKE) build/tools/go
	@mkdir -p build
	go build -v \
		-trimpath \
		-ldflags "\
			-X '$(IMPORT_PATH)/internal/buildinfo.AppName=$(APP_NAME)' \
			-X '$(IMPORT_PATH)/internal/buildinfo.Version=$(VERSION)' \
			-X '$(IMPORT_PATH)/internal/buildinfo.BuildTime=$(shell date --rfc-3339=ns)' \
		" \
		$(OPTIONAL_BUILD_ARGS) \
		-o $@ .

INSTALL_DIR?=$(HOME)/.local/bin

.PHONY: install
install: build/$(APP_NAME)-$(VERSION) ## Install binary to $(INSTALL_DIR)
	@mkdir -p $(INSTALL_DIR)
	cp $< $(INSTALL_DIR)/$(APP_NAME)-$(VERSION)
	ln -sf $(APP_NAME)-$(VERSION) $(INSTALL_DIR)/$(APP_NAME)

build-tools: build/tools/go
build/tools/go:
	@which $(notdir $@) || echo "see https://golang.org/doc/install"

.PHONY: fmt
fmt: ## Run go fmt against code
	go fmt ./...

.PHONY: vet
vet: ## Run go vet against code
	go vet -tags noembed ./...
