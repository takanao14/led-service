# LED Service

A gRPC-based service for controlling Raspberry Pi RGB LED matrix displays.

## Overview

`led-service` provides a gRPC interface to display images on RGB LED matrix panels connected to a Raspberry Pi. The service handles image reception, temporary storage, and display rendering via the underlying `rpi-rgb-led-matrix` library.

## Features

- gRPC server for remote image display requests
- Support for multiple image formats (PPM, JPEG, PNG, GIF, etc.)
- Configurable display duration and LED matrix parameters
- Logging via structured logging (slog)
- Systemd integration for automatic startup

## Hardware Requirements

- Raspberry Pi (v3, or v4 recommended)
- Hub75 RGB LED matrix panel (32x64 or compatible)
- GPIO pins for LED matrix control
- 5V power supply for the LED panel

## Building

### Prerequisites

- Go 1.21 or later
- C/C++ compiler (for rpi-rgb-led-matrix compilation)
- Standard development tools (make, git)

### Build Steps

```bash
# Clone the repository
git clone <repo-url>
cd led-service

# Build all binaries
make all

# Setup LED matrix dependencies (clone and build rpi-rgb-led-matrix)
make setup-led-matrix

# Verify build
ls -la bin/
```

The `make setup-led-matrix` command will clone the rpi-rgb-led-matrix repository and build the required `demo` and `led-image-viewer` binaries that are called by the display script.

## Running

### Manual Execution

```bash
# Build first
make all
make setup-led-matrix

# Run the server directly
./bin/led-server
```

Environment variables can be used to customize behavior:

- `GRPC_ADDR` (default: `:50051`) - gRPC server listen address
- `DISPLAY_SCRIPT` (default: `scripts/display_image.sh`) - Path to display script
- `WORKER_SCRIPT_TIMEOUT` (default: `30s`) - Max execution time per queued display script run (`time.ParseDuration` format)

Example:
```bash
GRPC_ADDR=:9999 DISPLAY_SCRIPT=/custom/path/display.sh WORKER_SCRIPT_TIMEOUT=45s ./bin/led-server
```

### Systemd Integration

For automatic startup on system boot, install the service:

```bash
# Build everything first
make all
make setup-led-matrix

# Install the systemd service file
make install-service

# Enable and start the service
make enable-service
```

The service will now:
- Start automatically on system boot
- Restart automatically if it crashes
- Log to the system journal

#### Service Management

```bash
# Check service status
systemctl status led-server

# View logs
journalctl -u led-server -f

# Restart the service
sudo systemctl restart led-server

# Stop the service
sudo systemctl stop led-server

# Disable auto-start (but keep service installed)
sudo systemctl disable led-server
```

#### Removing Systemd Service

```bash
# Disable and remove the service
make uninstall-service
```

## Architecture

- `cmd/led-server/` - Server entry point
- `internal/server/` - Core server logic:
  - `runner.go` - Main gRPC server setup and lifecycle
  - `service.go` - Image service implementation
  - `config.go` - Configuration resolution from environment
  - `display_runner.go` - Display script execution
  - `storage.go` - Temporary image storage
  - `logging.go` - Structured logging setup

## Client Usage

### Using grpcurl

```bash
# Send an image for 10 seconds
grpcurl -plaintext \
  -d '{"image":{"mimeType":"image/png","imageData":"<base64-encoded-png>"}, "durationSeconds":10}' \
  localhost:50051 image.v1.ImageService/SendImage
```

### Using led-client

```bash
./bin/led-client -addr=localhost:50051 -file=path/to/image.png -duration=10
```

## Permissions

**Important**: The LED matrix library requires **root privileges** to access GPIO hardware registers. The systemd service is configured to run as root. The gRPC server provides the security boundary through input validation and sandboxed display script execution.

## Troubleshooting

### Service won't start
```bash
# Check service status and errors
systemctl status led-server
journalctl -u led-server -n 50

# Verify binary exists and is executable
ls -la bin/led-server
file bin/led-server
```

### LED display not working
- Verify `scripts/display_image.sh` and dependencies exist
- Check LED matrix wiring and power
- Review logs: `journalctl -u led-server -f`
- Test manually: `./bin/led-server`
- If a display run fails or times out, the input file is preserved in `/tmp/grpc-image-server-*` with suffix `-failed` or `-timeout`

Manual cleanup example:
```bash
rm -f /tmp/grpc-image-server-*/led-image-*-failed.* /tmp/grpc-image-server-*/led-image-*-timeout.*
```

### gRPC connection errors
- Verify server is running: `systemctl status led-server`
- Check GRPC_ADDR is correct: `journalctl -u led-server | grep "listening"`
- Verify firewall rules allow port 50051 (or configured GRPC_ADDR port)

## Configuration

Service configuration is managed via:

1. **Systemd service file** (`led-server.service`):
   - `WorkingDirectory` - Project root directory
   - `ExecStart` - Path to led-server binary
   - `Environment` - gRPC listening address and display script path

2. **Environment variables**:
   - `GRPC_ADDR` - gRPC server address
   - `DISPLAY_SCRIPT` - Path to display script
   - `WORKER_SCRIPT_TIMEOUT` - Per-request worker timeout (for example: `30s`, `1m`); invalid or non-positive values cause startup failure

To customize settings:

```bash
# Edit the systemd service file
sudo nano /etc/systemd/system/led-server.service

# Reload and restart
sudo systemctl daemon-reload
sudo systemctl restart led-server
```

## Development

```bash
# Build only client
make $(BUILD_DIR)/led-client

# Build only server
make $(BUILD_DIR)/led-server

# Clean binaries
make clean

# Clean everything including LED matrix repo
make clean-led-matrix
```

## License

See [LICENSE](LICENSE) for details.
