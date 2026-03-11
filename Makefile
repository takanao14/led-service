.PHONY: all help clean proto-version setup-led-matrix clean-led-matrix install install-service enable-service uninstall-service

PREFIX ?= /opt/led-service
BUILD_DIR := bin
CLIENT_CMD := ./cmd/led-client
SERVER_CMD := ./cmd/led-server
CLIENT_BIN := $(BUILD_DIR)/led-client
SERVER_BIN := $(BUILD_DIR)/led-server

PROTO_MODULE := github.com/takanao14/led-image-api
LED_MATRIX_DIR := rpi-rgb-led-matrix
LED_MATRIX_REPO := https://github.com/hzeller/rpi-rgb-led-matrix.git
DEMO_BIN := scripts/demo
IMAGE_VIEWER_BIN := scripts/led-image-viewer

# Go source dependencies for incremental rebuilds.
CLIENT_SOURCES := $(shell find cmd/led-client -type f -name '*.go' 2>/dev/null)
SERVER_SOURCES := $(shell find cmd/led-server -type f -name '*.go' 2>/dev/null)
SHARED_SOURCES := $(shell find pkg -type f -name '*.go' 2>/dev/null)
INTERNAL_SERVER_SOURCES := $(shell find internal/server -type f -name '*.go' 2>/dev/null)
GO_MOD_FILES := go.mod go.sum

# OS detection
OS := $(shell uname -s)
ARCH := $(shell uname -m)
IS_RPI := $(shell if [ "$(OS)" = "Linux" ] && ([ "$(ARCH)" = "armv7l" ] || [ "$(ARCH)" = "aarch64" ]); then echo true; else echo false; fi)

all: $(CLIENT_BIN) $(SERVER_BIN) ## Build grpc-client and grpc-server

define check_rpi
	@if [ "$(IS_RPI)" != "true" ]; then \
		echo "Error: This target is only supported on Raspberry Pi (Linux ARM). Current OS: $(OS), ARCH: $(ARCH)"; \
		exit 1; \
	fi
endef

$(CLIENT_BIN): $(CLIENT_SOURCES) $(SHARED_SOURCES) $(GO_MOD_FILES) ## Build grpc-client binary
	@mkdir -p $(BUILD_DIR)
	go build -o $(CLIENT_BIN) $(CLIENT_CMD)

$(SERVER_BIN): $(SERVER_SOURCES) $(SHARED_SOURCES) $(INTERNAL_SERVER_SOURCES) $(GO_MOD_FILES) ## Build grpc-server binary
	@mkdir -p $(BUILD_DIR)
	go build -o $(SERVER_BIN) $(SERVER_CMD)

$(LED_MATRIX_DIR):
	@echo "Cloning rpi-rgb-led-matrix repository..."
	git clone $(LED_MATRIX_REPO)

$(DEMO_BIN): $(LED_MATRIX_DIR) ## Build demo binary from rpi-rgb-led-matrix
	@echo "Building demo in rpi-rgb-led-matrix..."
	@mkdir -p scripts
	$(MAKE) -C $(LED_MATRIX_DIR)
	cp $(LED_MATRIX_DIR)/examples-api-use/demo $(DEMO_BIN)
	@echo "Copied demo to $(DEMO_BIN)"

$(IMAGE_VIEWER_BIN): $(LED_MATRIX_DIR) ## Build led-image-viewer from rpi-rgb-led-matrix
	@echo "Building led-image-viewer in rpi-rgb-led-matrix/utils..."
	@mkdir -p scripts
	$(MAKE) -C $(LED_MATRIX_DIR)/utils led-image-viewer
	cp $(LED_MATRIX_DIR)/utils/led-image-viewer $(IMAGE_VIEWER_BIN)
	@echo "Copied led-image-viewer to $(IMAGE_VIEWER_BIN)"

setup-led-matrix: $(DEMO_BIN) $(IMAGE_VIEWER_BIN) ## Setup LED matrix binaries (clone repo and build)

help: ## Show this help message
	@echo 'Usage: make [target]'
	@echo ''
	@echo 'Available targets:'
	@awk 'BEGIN {FS = ":.*?## "} /^[a-zA-Z_-]+:.*?## / {printf "  %-15s %s\n", $$1, $$2}' $(MAKEFILE_LIST)

clean: ## Remove built binaries
	rm -rf $(BUILD_DIR)

clean-led-matrix: ## Remove cloned rpi-rgb-led-matrix repository and binaries
	rm -rf $(LED_MATRIX_DIR) $(DEMO_BIN) $(IMAGE_VIEWER_BIN)

proto-version: ## Show currently resolved external proto module version
	@go list -m $(PROTO_MODULE)

install: $(SERVER_BIN) $(CLIENT_BIN) $(DEMO_BIN) $(IMAGE_VIEWER_BIN)
	$(call check_rpi)
	install -d $(PREFIX)/bin $(PREFIX)/scripts $(PREFIX)/assets
	install -m 755 $(SERVER_BIN) $(PREFIX)/bin/
	install -m 755 $(CLIENT_BIN) $(PREFIX)/bin/
	install -m 755 scripts/display_image.sh $(PREFIX)/scripts/
	install -m 755 $(DEMO_BIN) $(PREFIX)/scripts/
	install -m 755 $(IMAGE_VIEWER_BIN) $(PREFIX)/scripts/
	cp -r assets/* $(PREFIX)/assets/

install-service: $(SERVER_BIN) ## Install systemd service file (requires sudo)
	$(call check_rpi)
	@if [ ! -f $(SERVER_BIN) ]; then \
		echo "Error: $(SERVER_BIN) not found. Run 'make all' first."; \
		exit 1; \
	fi
	@echo "Installing systemd service file..."
	cp led-server.service /etc/systemd/system/
	@echo "Service installed. Run 'make enable-service' to enable and start."

enable-service: ## Enable and start the led-server systemd service (requires sudo)
	$(call check_rpi)
	@echo "Reloading systemd daemon..."
	systemctl daemon-reload
	@echo "Enabling led-server service..."
	systemctl enable led-server
	@echo "Starting led-server service..."
	systemctl restart led-server
	@echo "Service started. Check status with: systemctl status led-server"

uninstall-service: ## Disable and remove the systemd service file (requires sudo)
	$(call check_rpi)
	@echo "Stopping led-server service..."
	systemctl stop led-server || true
	@echo "Disabling led-server service..."
	systemctl disable led-server || true
	@echo "Removing service file..."
	rm -f /etc/systemd/system/led-server.service
	@echo "Reloading systemd daemon..."
	systemctl daemon-reload
	@echo "Service uninstalled."
