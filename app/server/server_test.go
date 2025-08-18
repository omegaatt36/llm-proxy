package server

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/omegaatt36/llm-proxy/config"
)

type MockHTTPClient struct {
	DoFunc func(req *http.Request) (*http.Response, error)
}

func (m *MockHTTPClient) Do(req *http.Request) (*http.Response, error) {
	return m.DoFunc(req)
}

func TestProxyServer_HandleChatCompletions(t *testing.T) {
	tests := []struct {
		name               string
		requestBody        map[string]any
		mockResponse       map[string]any
		mockStatusCode     int
		expectedStatusCode int
		modelMappings      map[string]string
		checkResponse      func(t *testing.T, response map[string]any)
	}{
		{
			name: "successful request with model mapping",
			requestBody: map[string]any{
				"model":  "gpt-4-my-alias",
				"stream": false,
				"messages": []map[string]string{
					{"role": "user", "content": "Hello"},
				},
			},
			mockResponse: map[string]any{
				"model": "gpt-4",
				"choices": []map[string]any{
					{"message": map[string]string{"content": "Hi there!"}},
				},
			},
			mockStatusCode:     http.StatusOK,
			expectedStatusCode: http.StatusOK,
			modelMappings: map[string]string{
				"gpt-4-my-alias": "gpt-4",
			},
			checkResponse: func(t *testing.T, response map[string]any) {
				if response["model"] != "gpt-4-my-alias" {
					t.Errorf("Expected model to be 'gpt-4-my-alias', got %v", response["model"])
				}
			},
		},
		{
			name: "request without model mapping",
			requestBody: map[string]any{
				"model":  "gpt-3.5-turbo",
				"stream": false,
				"messages": []map[string]string{
					{"role": "user", "content": "Hello"},
				},
			},
			mockResponse: map[string]any{
				"model": "gpt-3.5-turbo",
				"choices": []map[string]any{
					{"message": map[string]string{"content": "Hi!"}},
				},
			},
			mockStatusCode:     http.StatusOK,
			expectedStatusCode: http.StatusOK,
			modelMappings:      map[string]string{},
			checkResponse: func(t *testing.T, response map[string]any) {
				if response["model"] != "gpt-3.5-turbo" {
					t.Errorf("Expected model to be 'gpt-3.5-turbo', got %v", response["model"])
				}
			},
		},
		{
			name: "upstream error",
			requestBody: map[string]any{
				"model":  "gpt-4",
				"stream": false,
				"messages": []map[string]string{
					{"role": "user", "content": "Hello"},
				},
			},
			mockResponse:       map[string]any{"error": "Internal Server Error"},
			mockStatusCode:     http.StatusInternalServerError,
			expectedStatusCode: http.StatusInternalServerError,
			modelMappings:      map[string]string{},
			checkResponse:      nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockClient := &MockHTTPClient{
				DoFunc: func(_ *http.Request) (*http.Response, error) {
					responseBody, _ := json.Marshal(tt.mockResponse)
					return &http.Response{
						StatusCode: tt.mockStatusCode,
						Body:       io.NopCloser(bytes.NewReader(responseBody)),
						Header:     make(http.Header),
					}, nil
				},
			}

			config := &config.Config{
				UpstreamURL:   "https://api.example.com",
				ModelMappings: tt.modelMappings,
			}

			proxy, err := NewProxyServer(config, mockClient)
			if err != nil {
				t.Fatalf("Failed to create proxy server: %v", err)
			}

			requestBody, _ := json.Marshal(tt.requestBody)
			req := httptest.NewRequest("POST", "/v1/chat/completions", bytes.NewReader(requestBody))
			req.Header.Set("Content-Type", "application/json")

			recorder := httptest.NewRecorder()
			proxy.HandleChatCompletions(recorder, req)

			resp := recorder.Result()
			if resp.StatusCode != tt.expectedStatusCode {
				t.Errorf("Expected status code %d, got %d", tt.expectedStatusCode, resp.StatusCode)
			}

			if tt.checkResponse != nil && resp.StatusCode == http.StatusOK {
				var responseData map[string]any
				if err := json.NewDecoder(resp.Body).Decode(&responseData); err != nil {
					t.Fatalf("Failed to decode response: %v", err)
				}
				tt.checkResponse(t, responseData)
			}
		})
	}
}

func TestProxyServer_HandleModels(t *testing.T) {
	mockClient := &MockHTTPClient{
		DoFunc: func(_ *http.Request) (*http.Response, error) {
			response := map[string]any{
				"data": []map[string]string{
					{"id": "gpt-4", "object": "model"},
					{"id": "gpt-3.5-turbo", "object": "model"},
				},
			}
			responseBody, _ := json.Marshal(response)
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(bytes.NewReader(responseBody)),
				Header:     make(http.Header),
			}, nil
		},
	}

	config := &config.Config{
		UpstreamURL: "https://api.example.com",
		ModelMappings: map[string]string{
			"gpt-4-my-alias": "gpt-4",
		},
	}

	proxy, err := NewProxyServer(config, mockClient)
	if err != nil {
		t.Fatalf("Failed to create proxy server: %v", err)
	}

	req := httptest.NewRequest("GET", "/v1/models", nil)
	recorder := httptest.NewRecorder()

	proxy.HandleModels(recorder, req)

	resp := recorder.Result()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status code %d, got %d", http.StatusOK, resp.StatusCode)
	}

	var responseData map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&responseData); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	data, ok := responseData["data"].([]any)
	if !ok {
		t.Fatal("Response data field is not an array")
	}

	foundMapping := false
	for _, model := range data {
		if modelMap, ok := model.(map[string]any); ok {
			if modelMap["id"] == "gpt-4-my-alias" {
				foundMapping = true
				break
			}
		}
	}

	if !foundMapping {
		t.Error("Expected to find 'gpt-4-my-alias' in models response")
	}
}

func TestProxyServer_HandleMessages(t *testing.T) {
	tests := []struct {
		name               string
		requestBody        map[string]any
		mockResponse       map[string]any
		mockStatusCode     int
		expectedStatusCode int
		modelMappings      map[string]string
	}{
		{
			name: "successful messages request",
			requestBody: map[string]any{
				"model":  "claude-3-my-alias",
				"stream": false,
				"messages": []map[string]string{
					{"role": "user", "content": "Hello"},
				},
			},
			mockResponse: map[string]any{
				"model": "claude-3-opus",
				"content": []map[string]any{
					{"type": "text", "text": "Hello! How can I help you?"},
				},
			},
			mockStatusCode:     http.StatusOK,
			expectedStatusCode: http.StatusOK,
			modelMappings: map[string]string{
				"claude-3-my-alias": "claude-3-opus",
			},
		},
		{
			name: "upstream error response",
			requestBody: map[string]any{
				"model":  "claude-3",
				"stream": false,
				"messages": []map[string]string{
					{"role": "user", "content": "Hello"},
				},
			},
			mockResponse: map[string]any{
				"error": map[string]string{
					"message": "Invalid API key",
				},
			},
			mockStatusCode:     http.StatusUnauthorized,
			expectedStatusCode: http.StatusUnauthorized,
			modelMappings:      map[string]string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockClient := &MockHTTPClient{
				DoFunc: func(_ *http.Request) (*http.Response, error) {
					responseBody, _ := json.Marshal(tt.mockResponse)
					return &http.Response{
						StatusCode: tt.mockStatusCode,
						Body:       io.NopCloser(bytes.NewReader(responseBody)),
						Header:     make(http.Header),
					}, nil
				},
			}

			config := &config.Config{
				UpstreamURL:   "https://api.example.com",
				ModelMappings: tt.modelMappings,
			}

			proxy, err := NewProxyServer(config, mockClient)
			if err != nil {
				t.Fatalf("Failed to create proxy server: %v", err)
			}

			requestBody, _ := json.Marshal(tt.requestBody)
			req := httptest.NewRequest("POST", "/v1/messages", bytes.NewReader(requestBody))
			req.Header.Set("Content-Type", "application/json")

			recorder := httptest.NewRecorder()
			proxy.HandleMessages(recorder, req)

			resp := recorder.Result()
			if resp.StatusCode != tt.expectedStatusCode {
				t.Errorf("Expected status code %d, got %d", tt.expectedStatusCode, resp.StatusCode)
			}
		})
	}
}

func TestProxyServer_HandleDefault(t *testing.T) {
	mockClient := &MockHTTPClient{
		DoFunc: func(req *http.Request) (*http.Response, error) {
			if !strings.HasSuffix(req.URL.Path, "/custom/endpoint") {
				t.Errorf("Expected path to end with '/custom/endpoint', got %s", req.URL.Path)
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(`{"result": "success"}`)),
				Header:     make(http.Header),
			}, nil
		},
	}

	config := &config.Config{
		UpstreamURL: "https://api.example.com",
	}

	proxy, err := NewProxyServer(config, mockClient)
	if err != nil {
		t.Fatalf("Failed to create proxy server: %v", err)
	}

	req := httptest.NewRequest("GET", "/custom/endpoint?param=value", nil)
	recorder := httptest.NewRecorder()

	proxy.HandleDefault(recorder, req)

	resp := recorder.Result()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status code %d, got %d", http.StatusOK, resp.StatusCode)
	}

	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "success") {
		t.Errorf("Expected response to contain 'success', got %s", string(body))
	}
}

func TestProxyServer_InvalidRequestFormat(t *testing.T) {
	config := &config.Config{
		UpstreamURL: "https://api.example.com",
	}

	proxy, err := NewProxyServer(config, nil)
	if err != nil {
		t.Fatalf("Failed to create proxy server: %v", err)
	}

	tests := []struct {
		name           string
		handler        http.HandlerFunc
		requestBody    string
		expectedStatus int
	}{
		{
			name:           "invalid JSON in chat completions",
			handler:        proxy.HandleChatCompletions,
			requestBody:    `{invalid json}`,
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "missing model field in chat completions",
			handler:        proxy.HandleChatCompletions,
			requestBody:    `{"stream": false}`,
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "invalid JSON in messages",
			handler:        proxy.HandleMessages,
			requestBody:    `{invalid json}`,
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "missing model field in messages",
			handler:        proxy.HandleMessages,
			requestBody:    `{"stream": false}`,
			expectedStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("POST", "/", strings.NewReader(tt.requestBody))
			req.Header.Set("Content-Type", "application/json")

			recorder := httptest.NewRecorder()
			tt.handler(recorder, req)

			resp := recorder.Result()
			if resp.StatusCode != tt.expectedStatus {
				t.Errorf("Expected status code %d, got %d", tt.expectedStatus, resp.StatusCode)
			}
		})
	}
}

func TestProxyServer_WithAPIKey(t *testing.T) {
	mockClient := &MockHTTPClient{
		DoFunc: func(req *http.Request) (*http.Response, error) {
			authHeader := req.Header.Get("Authorization")
			if authHeader != "Bearer test-api-key" {
				t.Errorf("Expected Authorization header 'Bearer test-api-key', got '%s'", authHeader)
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(`{"model": "gpt-4", "choices": []}`)),
				Header:     make(http.Header),
			}, nil
		},
	}

	config := &config.Config{
		UpstreamURL:    "https://api.example.com",
		UpstreamAPIKey: "test-api-key",
	}

	proxy, err := NewProxyServer(config, mockClient)
	if err != nil {
		t.Fatalf("Failed to create proxy server: %v", err)
	}

	requestBody := `{"model": "gpt-4", "stream": false, "messages": []}`
	req := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(requestBody))
	req.Header.Set("Content-Type", "application/json")

	recorder := httptest.NewRecorder()
	proxy.HandleChatCompletions(recorder, req)

	resp := recorder.Result()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status code %d, got %d", http.StatusOK, resp.StatusCode)
	}
}
