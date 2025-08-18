# LLM Proxy

This is a lightweight Large Language Model (LLM) proxy service written in Go. It provides a unified interface for various LLM services, supporting model name mapping, request proxying, and logging.

## Features

*   OpenAI API Compatibility: Proxies `/v1/chat/completions`, `/v1/messages`, and `/v1/models` endpoints.
*   Model Mapping: Allows mapping incoming model names to upstream service model names.
*   Request Proxying: Forwards requests to configured upstream LLM services.
*   Stream and Non-Stream Support: Handles both streaming and non-streaming responses from LLM services.
*   Logging: Records incoming requests and proxy activity for monitoring and debugging.

## Configuration

The proxy service is configured via a `config.yaml` file. An example is provided in `config.yaml.example`.

Configuration file search paths (in order):

1.  `./config.yaml`
2.  `/etc/llm-proxy/config.yaml`
3.  `$HOME/.llm-proxy/config.yaml`

Example `config.yaml`:

```llm-proxy/config.yaml.example#L1-9
port: "4000"
upstreamURL: "https://api.openai.com"
upstreamAPIKey: "sk-your-openai-api-key"
modelMappings:
  gpt-4-my-alias: gpt-4
  claude-3-opus-my-alias: claude-3-opus-20240229
logLevel: debug # debug, info, warn, error
```

*   `port`: (Optional) The port the proxy service listens on. Default is `4000`.
*   `upstreamURL`: (Required) The URL of the upstream LLM service.
*   `upstreamAPIKey`: (Optional) The API key for the upstream LLM service.
*   `modelMappings`: (Optional) A dictionary for mapping local model names to upstream model names.
*   `logLevel`: (Optional) Logging level. Options: `debug`, `info`, `warn`, `error`. Default is `info`.

## How to Run

### Using Docker

1.  Build the Docker image:
    ```bash
    docker build -t llm-proxy .
    ```
2.  Run the Docker container:
    ```bash
    docker run -d -p 4000:4000 --name llm-proxy -v /path/to/your/config.yaml:/etc/llm-proxy/config.yaml llm-proxy
    ```
    Replace `/path/to/your/config.yaml` with the actual path to your `config.yaml` file.

### From Source

1.  Ensure Go 1.21 or higher is installed.
2.  Clone the repository:
    ```bash
    git clone https://github.com/omegaatt36/llm-proxy.git
    cd llm-proxy
    ```
3.  Build and run the service:
    ```bash
    go run cmd/llm-proxy/main.go
    ```
    Ensure your `config.yaml` is in one of the expected paths before running.

## API Endpoints

The proxy service supports the following main endpoints, forwarding them to the upstream LLM service:

*   `POST /v1/chat/completions`
*   `POST /v1/messages`
*   `GET /v1/models`

All other requests are directly proxied to the `upstreamURL` retaining the original path and query parameters.