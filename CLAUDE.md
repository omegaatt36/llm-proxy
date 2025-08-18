# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Build Commands

- **Build**: `go build -o llm-proxy ./cmd/llm-proxy`
- **Run from source**: `go run cmd/llm-proxy/main.go`
- **Test build**: `go build -o /tmp/test-build ./cmd/llm-proxy`
- **Run tests**: `go test -v ./cmd/llm-proxy/...`
- **Docker build**: `docker build -t llm-proxy .`
- **Docker run with compose**: `docker-compose up`

## Architecture Overview

This is a lightweight LLM proxy service written in Go that provides a unified interface for various LLM services. The architecture consists of:

### Core Components

- **ProxyServer** (`cmd/llm-proxy/server.go`): Central proxy server struct that handles upstream communication
- **HTTPClient Interface** (`cmd/llm-proxy/server.go:14-16`): Abstraction for HTTP client, enables unit testing with mocks
- **Config** (`cmd/llm-proxy/config.go:10-16`): Configuration structure for proxy settings
- **Middleware** (`cmd/llm-proxy/middleware.go`): HTTP middleware chain with logging support

### Key Features

1. **Model Mapping**: Transforms incoming model names to upstream service model names via `modelMappings` configuration
2. **API Compatibility**: Supports OpenAI API endpoints (`/v1/chat/completions`, `/v1/messages`, `/v1/models`)
3. **Streaming Support**: Handles both streaming and non-streaming responses with proper flushing
4. **Request Proxying**: Forwards all other requests to upstream services while preserving headers and query parameters

### Request Flow

1. Client request → Logging middleware → Route handler
2. For specific endpoints (`/v1/chat/completions`, `/v1/messages`):
   - Parse request body
   - Apply model mapping if configured
   - Forward to upstream with modified model name
   - Transform response model name back to original
3. For `/v1/models`: Apply reverse model mapping to response
4. For all other paths: Direct proxy to upstream

### Configuration

- **Search paths**: `./config.yaml`, `/etc/llm-proxy/config.yaml`, `$HOME/.llm-proxy/config.yaml`
- **Required**: `upstreamURL`
- **Optional**: `port` (default: 4000), `upstreamAPIKey`, `modelMappings`, `logLevel`

### Error Handling

- Upstream request failures return 502 Bad Gateway
- Invalid request formats return 400 Bad Request
- All errors are logged with structured logging (slog)

## Project Structure

```
cmd/llm-proxy/
├── main.go          # Program entry point
├── config.go        # Configuration loading logic
├── server.go        # ProxyServer implementation and HTTP handlers
├── server_test.go   # Unit tests for ProxyServer
└── middleware.go    # HTTP middleware (logging)
config.yaml.example  # Configuration template
Dockerfile          # Multi-stage build using distroless base
docker-compose.yaml # Container deployment
```

The application follows a modular structure with separated concerns:
- Configuration management is isolated in `config.go`
- Server logic and HTTP handlers are in `server.go` with HTTPClient interface for testability
- Comprehensive unit tests in `server_test.go` using `net/http/httptest`
- Single external dependency on `github.com/goccy/go-yaml` for YAML configuration parsing