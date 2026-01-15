# Daaya Video Server

A Go-based educational video streaming server with taxonomic classification.

## Overview

Daaya Video Server provides REST API for streaming educational videos with hierarchical taxonomy support. Videos are organized by educational classification (e.g., `elementary/math/number system/counting`) and can be filtered by taxonomic rank.

## Security Improvements (v0.0.1+)

### Critical Fixes Applied:
1. **Path Traversal Prevention** - All filename inputs are validated and sanitized
2. **Non-root Service User** - Runs as dedicated `daayavideo` system user
3. **Input Validation** - Taxonomy rank parameters validated against allow list
4. **Systemd Hardening** - `PrivateTmp=yes`, `ProtectSystem=strict`, `AmbientCapabilities=CAP_NET_BIND_SERVICE` for privileged ports
5. **Proper Error Handling** - No silent error swallowing, graceful degradation for missing metadata

### Security Configuration:
- Service runs as `daayavideo:daayavideo` user/group
- Video directory: `/var/daaya/videos` (permissions 750)
- Log directory: `/opt/daayavideoservice/daayavideoserver.log` (permissions 640)
- Systemd restrictions prevent privilege escalation

## Configuration

### Environment Variables:
```bash
# Custom video storage path (default: /var/daaya/videos)
export DAAYA_VIDEO_PATH=/path/to/videos

# Custom port (default: :443, set via DAAYA_PORT=443 in service file)
export DAAYA_PORT=8182  # example: change back to 8182 if needed (default is 443)

# Comma-separated hostnames for Let's Encrypt (default: api.daaya.org)
export DAAYA_HOSTNAMES=api.daaya.org,api2.daaya.org
```

### Configuration Precedence:
1. Environment variables
2. Hardcoded defaults

### Video Directory Structure:
```
/var/daaya/videos/
├── video1/
│   ├── video1.mp4          # Video file (must match directory name)
│   ├── title               # Video title
│   ├── author              # Author name
│   ├── description         # Detailed description
│   └── classification      # Taxonomic hierarchy (slash-separated)
└── video2/...
```

### Taxonomic Classification Format:
```
level1/level2/level3/level4/level5
```
Example: `elementary/math/number system/counting`

## API Documentation

### Base URL: `https://host:443/api/v1` (HTTP redirects from port 80, default HTTPS port is 443)

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/videos` | GET | List all videos with metadata |
| `/stream/{filename}` | GET | Stream video file |
| `/classify?rank=class&value=elementary` | GET | Filter videos by taxonomy |
| `/help` | GET | API documentation |
| `/health` | GET | Health check (returns 200/503) |
| `/metrics` | GET | Prometheus metrics (request counts, errors, streams) |

### Examples:
```bash
# List all videos
curl -k https://localhost:443/api/v1/videos

# Stream video
curl -k https://localhost:443/api/v1/stream/video1

# Filter by taxonomy
curl -k "https://localhost:443/api/v1/classify?rank=class&value=elementary"

# Health check
curl -k https://localhost:443/health

# Metrics
curl -k https://localhost:443/metrics
```

## Deployment

### Debian Package Installation:
```bash
# Build package
./build.sh

# Install package
sudo dpkg -i daayavideoservice-0.0.1.deb
```

### Manual Installation:
```bash
# Build binary
go build

# Create directories
sudo mkdir -p /var/daaya/videos
sudo chown daayavideo:daayavideo /var/daaya/videos
sudo chmod 750 /var/daaya/videos

# Copy binary and service file
sudo cp daayavideoserver /opt/daayavideoservice/
sudo cp DEBIAN/daayavideo.service /etc/systemd/system/

# Enable and start service
sudo systemctl daemon-reload
sudo systemctl enable daayavideo.service
sudo systemctl start daayavideo.service
```

### Systemd Service Details:
- Service: `daayavideo.service`
- User: `daayavideo`
- Logs: `/opt/daayavideoservice/daayavideoserver.log`
- Auto-restart: always (with 1s delay)
- Privileged ports: Capability CAP_NET_BIND_SERVICE added to allow binding to ports 80/443 (HTTPS port set to 443 via DAAYA_PORT=443)

### Custom Configuration (Systemd Override):
```bash
sudo systemctl edit daayavideo.service
```
```ini
[Service]
Environment="DAAYA_VIDEO_PATH=/custom/video/path"
Environment="DAAYA_PORT=9000"
# For standard ports (80/443) also add:
# AmbientCapabilities=CAP_NET_BIND_SERVICE
# CapabilityBoundingSet=CAP_NET_BIND_SERVICE
# Environment="DAAYA_PORT=443"
```

## Health Monitoring

### Health Check Endpoint:
- `GET /health` - Returns 200 OK when service is healthy
- Returns 503 Service Unavailable if video directory inaccessible

### Monitoring Integration:
- Health checks can be used with load balancers and monitoring systems
- Logs include structured warnings and errors
- Systemd service manager provides process supervision

## Metrics

### Prometheus Metrics Endpoint:
- `GET /metrics` - Returns service metrics in Prometheus text format
- Metrics include: total requests, errors, video streams, classify requests
- Last error message tracked as gauge metric

### Available Metrics:
- `daaya_requests_total` - Total HTTP requests processed
- `daaya_errors_total` - Total HTTP errors (status >= 400)
- `daaya_streams_total` - Total video streams served
- `daaya_classify_requests_total` - Total classify requests
- `daaya_last_error` - Last error HTTP status code (gauge, 0 if none)

### Rate Limiting:
- 10 requests per minute per IP (configurable in code)
- Returns 429 "Rate limit exceeded" when exceeded
- Applied to all endpoints including metrics

## Logging

### Log Locations:
- Application logs: `/opt/daayavideoservice/daayavideoserver.log`
- Systemd logs: `sudo journalctl -u daayavideo.service`

### Log Format:
- Startup information (port, video path)
- Warnings for missing metadata files
- Errors for file operations
- Request errors (invalid parameters, not found)

## Testing

### Unit Tests:
```bash
# Run all tests
go test ./...

# Test with coverage
go test -cover ./...
```

### Test Coverage:
- Path traversal prevention
- Taxonomy parsing and filtering
- Error handling for missing files
- Input validation

## Building from Source

### Prerequisites:
- Go 1.19+
- Standard build tools

### Build Process:
```bash
# Run tests and build
./build.sh

# Manual build
go test ./...
go build
```

### Package Creation:
The `build.sh` script:
1. Runs all tests (fails if tests don't pass)
2. Builds Go binary
3. Creates Debian package structure
4. Generates versioned `.deb` package

## Troubleshooting

### Common Issues:

**Service fails to start:**
```bash
sudo systemctl status daayavideo.service
sudo journalctl -u daayavideo.service -f
```

**Video directory permissions:**
```bash
sudo chown -R daayavideo:daayavideo /var/daaya/videos
sudo chmod 750 /var/daaya/videos
```

**Port already in use:**
```bash
# Change port via environment variable
sudo systemctl edit daayavideo.service
# Add: Environment="DAAYA_PORT=9000"
```

**Missing metadata files:**
- Check that each video directory contains `title`, `author`, `description`, `classification` files
- Service will log warnings but continue operating

### Debug Mode:
```bash
# Run manually with debug output
DAAYA_PORT=443 ./daayavideoserver
```

## Development

### Code Structure:
- `main.go` - HTTP server and API endpoints
- `main_test.go` - Unit tests
- `go.mod` - Go dependencies
- `build.sh` - Build and packaging script
- `DEBIAN/` - Debian package configuration

### Adding Features:
1. Add endpoint handlers to `main.go`
2. Write corresponding tests in `main_test.go`
3. Update API documentation in `/help` endpoint
4. Run tests: `go test ./...`
5. Build and test package: `./build.sh`

## License

Proprietary - © Daaya.org

## Support

For issues and questions:
- Check logs: `/opt/daayavideoservice/daayavideoserver.log`
- Systemd status: `sudo systemctl status daayavideo.service`
- API documentation: `GET /help` endpoint
