package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"time"

	"github.com/omegaatt36/llm-proxy/config"
)

type HTTPClient interface {
	Do(req *http.Request) (*http.Response, error)
}

type ProxyServer struct {
	port           string
	upstreamURL    *url.URL
	upstreamAPIKey string
	modelMappings  map[string]string
	httpClient     HTTPClient
}

func (p *ProxyServer) Start(ctx context.Context) error {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /v1/chat/completions", p.HandleChatCompletions)
	mux.HandleFunc("POST /v1/messages", p.HandleMessages)
	mux.HandleFunc("GET /v1/models", p.HandleModels)
	mux.HandleFunc("GET /health", p.HandleHealth)
	mux.HandleFunc("/", p.HandleDefault)

	server := &http.Server{
		Addr:         ":" + p.port,
		Handler:      chainMiddleware(logging)(mux),
		ReadTimeout:  120 * time.Second,
		WriteTimeout: 120 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	slog.Info("LLM Proxy server starting", "port", p.port)
	slog.Info("Proxying to", "url", p.upstreamURL)

	go func() {
		if err := server.ListenAndServe(); err != nil {
			slog.Error("Server failed to start", "error", err)
		}
	}()

	go func() {
		<-ctx.Done()
		slog.Info("Shutting down server...")
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := server.Shutdown(ctx); err != nil {
			slog.Error("Server failed to shutdown", "error", err)
		}
	}()

	return nil
}

func NewProxyServer(config *config.Config, httpClient HTTPClient) (*ProxyServer, error) {
	upstreamURL, err := url.Parse(config.UpstreamURL)
	if err != nil {
		return nil, fmt.Errorf("invalid upstream URL: %w", err)
	}

	if httpClient == nil {
		httpClient = &http.Client{
			Timeout: 120 * time.Second,
		}
	}

	proxy := &ProxyServer{
		port:           config.Port,
		upstreamURL:    upstreamURL,
		upstreamAPIKey: config.UpstreamAPIKey,
		modelMappings:  config.ModelMappings,
		httpClient:     httpClient,
	}

	return proxy, nil
}

func (p *ProxyServer) upstreamPath(path string) string {
	u := *p.upstreamURL
	return u.JoinPath(path).String()
}

func (p *ProxyServer) HandleChatCompletions(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Failed to read request body", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	slog.Debug("Chat completions request body", "body", string(body))

	var req map[string]any
	if err := json.Unmarshal(body, &req); err != nil {
		http.Error(w, "Invalid request format", http.StatusBadRequest)
		return
	}

	originalModel, ok := req["model"].(string)
	if !ok {
		http.Error(w, "Invalid request format", http.StatusBadRequest)
		return
	}

	originalStream, ok := req["stream"].(bool)
	if !ok {
		originalStream = false
	}

	if mappedModel, exists := p.modelMappings[originalModel]; exists {
		req["model"] = mappedModel
		slog.Debug("Mapped model", "from", originalModel, "to", mappedModel)
	}

	modifiedBody, err := json.Marshal(req)
	if err != nil {
		http.Error(w, "Failed to marshal request", http.StatusInternalServerError)
		return
	}

	proxyReq, err := http.NewRequest(r.Method, p.upstreamPath("/v1/chat/completions"), bytes.NewReader(modifiedBody))
	if err != nil {
		http.Error(w, "Failed to create proxy request", http.StatusInternalServerError)
		return
	}

	for key, values := range r.Header {
		if key != "Authorization" && key != "Content-Length" {
			for _, value := range values {
				proxyReq.Header.Add(key, value)
			}
		}
	}

	if p.upstreamAPIKey != "" {
		proxyReq.Header.Set("Authorization", "Bearer "+p.upstreamAPIKey)
	}

	proxyReq.Header.Set("Content-Type", "application/json")
	proxyReq.Header.Set("Content-Length", fmt.Sprintf("%d", len(modifiedBody)))

	resp, err := p.httpClient.Do(proxyReq)
	if err != nil {
		slog.Error("Upstream request failed", "error", err)
		http.Error(w, "Upstream request failed", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	for key, values := range resp.Header {
		for _, value := range values {
			w.Header().Add(key, value)
		}
	}

	w.WriteHeader(resp.StatusCode)

	if originalStream {
		flusher, ok := w.(http.Flusher)
		if ok {
			buffer := make([]byte, 1024)
			for {
				n, err := resp.Body.Read(buffer)
				if n > 0 {
					if _, writeErr := w.Write(buffer[:n]); writeErr != nil {
						return
					}
					flusher.Flush()
				}
				if err != nil {
					break
				}
			}
		} else {
			io.Copy(w, resp.Body)
		}
	} else {
		responseBody, err := io.ReadAll(resp.Body)
		if err != nil {
			slog.Error("Failed to read response body", "error", err)
			return
		}

		var responseData map[string]any
		if err := json.Unmarshal(responseBody, &responseData); err == nil {
			if responseData["model"] == req["model"] {
				responseData["model"] = originalModel
				modifiedResponse, _ := json.Marshal(responseData)
				w.Write(modifiedResponse)
				return
			}
		}

		w.Write(responseBody)
	}
}

func (p *ProxyServer) HandleMessages(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Failed to read request body", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	slog.Debug("Messages request body", "body", string(body))

	var req map[string]any
	if err := json.Unmarshal(body, &req); err != nil {
		http.Error(w, "Invalid request format", http.StatusBadRequest)
		return
	}

	originalModel, ok := req["model"].(string)
	if !ok {
		http.Error(w, "Invalid request format", http.StatusBadRequest)
		return
	}

	slog.Debug("Received request for model", "model", originalModel)

	originalStream, ok := req["stream"].(bool)
	if !ok {
		originalStream = false
	}

	if mappedModel, exists := p.modelMappings[originalModel]; exists {
		req["model"] = mappedModel
		slog.Debug("Mapped model", "from", originalModel, "to", mappedModel)
	}

	modifiedBody, err := json.Marshal(req)
	if err != nil {
		http.Error(w, "Failed to marshal request", http.StatusInternalServerError)
		return
	}

	targetURL := p.upstreamPath("/v1/messages")
	if r.URL.RawQuery != "" {
		targetURL += "?" + r.URL.RawQuery
	}

	proxyReq, err := http.NewRequest(r.Method, targetURL, bytes.NewReader(modifiedBody))
	if err != nil {
		http.Error(w, "Failed to create proxy request", http.StatusInternalServerError)
		return
	}

	for key, values := range r.Header {
		if key != "Authorization" && key != "Content-Length" {
			for _, value := range values {
				proxyReq.Header.Add(key, value)
			}
		}
	}

	if p.upstreamAPIKey != "" {
		proxyReq.Header.Set("Authorization", "Bearer "+p.upstreamAPIKey)
	}

	proxyReq.Header.Set("Content-Type", "application/json")
	proxyReq.Header.Set("Content-Length", fmt.Sprintf("%d", len(modifiedBody)))

	resp, err := p.httpClient.Do(proxyReq)
	if err != nil {
		slog.Error("Upstream request failed", "error", err)
		http.Error(w, "Upstream request failed", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		responseBody, _ := io.ReadAll(resp.Body)
		slog.Error("Upstream returned error", "status", resp.StatusCode, "body", string(responseBody))
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(resp.StatusCode)
		w.Write(responseBody)
		return
	}

	for key, values := range resp.Header {
		for _, value := range values {
			w.Header().Add(key, value)
		}
	}

	w.WriteHeader(resp.StatusCode)

	if originalStream {
		flusher, ok := w.(http.Flusher)
		if ok {
			buffer := make([]byte, 1024)
			for {
				n, err := resp.Body.Read(buffer)
				if n > 0 {
					if _, writeErr := w.Write(buffer[:n]); writeErr != nil {
						return
					}
					flusher.Flush()
				}
				if err != nil {
					break
				}
			}
		} else {
			io.Copy(w, resp.Body)
		}
	} else {
		responseBody, err := io.ReadAll(resp.Body)
		if err != nil {
			slog.Error("Failed to read response body", "error", err)
			return
		}

		var responseData map[string]any
		if err := json.Unmarshal(responseBody, &responseData); err == nil {
			if responseData["model"] == req["model"] {
				responseData["model"] = originalModel
				modifiedResponse, _ := json.Marshal(responseData)
				w.Write(modifiedResponse)
				return
			}
		}

		w.Write(responseBody)
	}
}

func (p *ProxyServer) HandleModels(w http.ResponseWriter, r *http.Request) {
	proxyReq, err := http.NewRequest(r.Method, p.upstreamPath("/v1/models"), nil)
	if err != nil {
		http.Error(w, "Failed to create proxy request", http.StatusInternalServerError)
		return
	}

	for key, values := range r.Header {
		if key != "Authorization" {
			for _, value := range values {
				proxyReq.Header.Add(key, value)
			}
		}
	}

	if p.upstreamAPIKey != "" {
		proxyReq.Header.Set("Authorization", "Bearer "+p.upstreamAPIKey)
	}

	resp, err := p.httpClient.Do(proxyReq)
	if err != nil {
		http.Error(w, "Upstream request failed", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		http.Error(w, "Failed to read response", http.StatusInternalServerError)
		return
	}

	var modelsResponse map[string]any
	if err := json.Unmarshal(body, &modelsResponse); err == nil {
		if data, ok := modelsResponse["data"].([]any); ok {
			reverseMappings := make(map[string]string)
			for localModel, remoteModel := range p.modelMappings {
				reverseMappings[remoteModel] = localModel
			}

			for _, model := range data {
				if modelMap, ok := model.(map[string]any); ok {
					if modelID, ok := modelMap["id"].(string); ok {
						if localModel, exists := reverseMappings[modelID]; exists {
							modelMap["id"] = localModel
						}
					}
				}
			}

			modifiedBody, _ := json.Marshal(modelsResponse)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(resp.StatusCode)
			w.Write(modifiedBody)
			return
		}
	}

	for key, values := range resp.Header {
		for _, value := range values {
			w.Header().Add(key, value)
		}
	}
	w.WriteHeader(resp.StatusCode)
	w.Write(body)
}

func (p *ProxyServer) HandleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("OK"))
}

func (p *ProxyServer) HandleDefault(w http.ResponseWriter, r *http.Request) {
	targetURL := p.upstreamPath(r.URL.Path)
	if r.URL.RawQuery != "" {
		targetURL += "?" + r.URL.RawQuery
	}

	slog.Debug("Default handler - proxying", "uri", r.URL.RequestURI())

	proxyReq, err := http.NewRequest(r.Method, targetURL, r.Body)
	if err != nil {
		http.Error(w, "Failed to create proxy request", http.StatusInternalServerError)
		return
	}
	defer r.Body.Close()

	for key, values := range r.Header {
		if key != "Authorization" {
			for _, value := range values {
				proxyReq.Header.Add(key, value)
			}
		}
	}

	if p.upstreamAPIKey != "" {
		proxyReq.Header.Set("Authorization", "Bearer "+p.upstreamAPIKey)
	}

	resp, err := p.httpClient.Do(proxyReq)
	if err != nil {
		http.Error(w, "Upstream request failed", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	for key, values := range resp.Header {
		for _, value := range values {
			w.Header().Add(key, value)
		}
	}

	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body)
}
