// Package embeddingeval provides a bounded local-only compatibility proxy for
// Architecture 021 model evaluation. It preserves the Architecture 020 wire
// contract without granting the stock model backend fallback or metadata
// authority.
package embeddingeval

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"revolvr/internal/embedding"
	codeindex "revolvr/internal/index"
)

const (
	MaximumInputs       = 32
	MaximumInputBytes   = 256 << 10
	MaximumRequestBytes = 1 << 20
	MaximumResponse     = 64 << 20
)

type Config struct {
	BackendEndpoint string
	BackendModel    string
	Model           embedding.EmbeddingModelInfo
	DocumentPrefix  string
	QueryPrefix     string
	Timeout         time.Duration
	HTTPClient      *http.Client
}

type Server struct {
	config Config
}

func New(config Config) (*Server, error) {
	if err := codeindex.ValidateSelectedEmbeddingModel(config.Model); err != nil {
		return nil, err
	}
	backend, err := url.Parse(strings.TrimSuffix(strings.TrimSpace(config.BackendEndpoint), "/"))
	if err != nil || backend.Scheme != "http" || backend.Host == "" || backend.User != nil || backend.RawQuery != "" || backend.Fragment != "" || backend.Path != "" || !localHost(backend.Hostname()) {
		return nil, errors.New("embedding evaluation proxy: backend must be a normalized local HTTP origin")
	}
	config.BackendEndpoint = backend.String()
	if strings.TrimSpace(config.BackendModel) == "" || config.BackendModel != strings.TrimSpace(config.BackendModel) {
		return nil, errors.New("embedding evaluation proxy: backend model is required")
	}
	if config.DocumentPrefix != "" || config.QueryPrefix != codeindex.SelectedQueryInstruction {
		return nil, errors.New("embedding evaluation proxy: selected Qwen input policy is required")
	}
	if config.Timeout == 0 {
		config.Timeout = 60 * time.Second
	}
	if config.Timeout <= 0 || len(config.DocumentPrefix) > 4096 || len(config.QueryPrefix) > 4096 {
		return nil, errors.New("embedding evaluation proxy: invalid bounds")
	}
	if config.HTTPClient == nil {
		config.HTTPClient = &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	}
	return &Server{config: config}, nil
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/health", s.health)
	mux.HandleFunc("GET /v1/metadata", s.metadata)
	mux.HandleFunc("POST /v1/embeddings", s.embeddings)
	return http.MaxBytesHandler(mux, MaximumRequestBytes)
}

func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	request, err := http.NewRequestWithContext(r.Context(), http.MethodGet, s.config.BackendEndpoint+"/health", nil)
	if err != nil {
		http.Error(w, "backend unavailable", http.StatusServiceUnavailable)
		return
	}
	response, err := s.config.HTTPClient.Do(request)
	if err != nil {
		http.Error(w, "backend unavailable", http.StatusServiceUnavailable)
		return
	}
	defer response.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(response.Body, 4097))
	var status struct {
		Status string `json:"status"`
	}
	validStatus := len(bytes.TrimSpace(raw)) == 0 || (json.Unmarshal(raw, &status) == nil && status.Status == "ok")
	if err != nil || len(raw) > 4096 || response.StatusCode != http.StatusOK || !validStatus {
		http.Error(w, "backend unhealthy", http.StatusServiceUnavailable)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) metadata(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, s.config.Model)
}

type embeddingRequest struct {
	Model          string   `json:"model"`
	Input          []string `json:"input"`
	InputType      string   `json:"input_type"`
	EncodingFormat string   `json:"encoding_format"`
}

type backendRequest struct {
	Model          string   `json:"model"`
	Input          []string `json:"input"`
	EncodingFormat string   `json:"encoding_format"`
}

type backendResponse struct {
	Object string `json:"object"`
	Model  string `json:"model"`
	Data   []struct {
		Object    string    `json:"object"`
		Index     int       `json:"index"`
		Embedding []float64 `json:"embedding"`
	} `json:"data"`
}

func (s *Server) embeddings(w http.ResponseWriter, r *http.Request) {
	var input embeddingRequest
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if decoder.Decode(&input) != nil || decoder.Decode(&struct{}{}) != io.EOF || input.Model != s.config.Model.ModelName || input.EncodingFormat != "float" || (input.InputType != "documents" && input.InputType != "query") || len(input.Input) == 0 || len(input.Input) > MaximumInputs || (input.InputType == "query" && len(input.Input) != 1) {
		http.Error(w, "invalid bounded embedding request", http.StatusBadRequest)
		return
	}
	prefix := s.config.DocumentPrefix
	if input.InputType == "query" {
		prefix = s.config.QueryPrefix
	}
	values := make([]string, len(input.Input))
	total := 0
	for index, value := range input.Input {
		if value == "" || len(value) > MaximumInputBytes || len(prefix)+len(value) > MaximumInputBytes {
			http.Error(w, "invalid bounded embedding input", http.StatusBadRequest)
			return
		}
		values[index] = prefix + value
		total += len(values[index])
	}
	if total > MaximumRequestBytes {
		http.Error(w, "embedding batch exceeds bound", http.StatusBadRequest)
		return
	}
	payload, _ := json.Marshal(backendRequest{Model: s.config.BackendModel, Input: values, EncodingFormat: "float"})
	ctx, cancel := context.WithTimeout(r.Context(), s.config.Timeout)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, s.config.BackendEndpoint+"/v1/embeddings", bytes.NewReader(payload))
	if err != nil {
		http.Error(w, "backend unavailable", http.StatusServiceUnavailable)
		return
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json")
	response, err := s.config.HTTPClient.Do(request)
	if err != nil {
		http.Error(w, "backend unavailable", http.StatusServiceUnavailable)
		return
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		http.Error(w, "backend rejected request", http.StatusServiceUnavailable)
		return
	}
	var output backendResponse
	backendDecoder := json.NewDecoder(io.LimitReader(response.Body, MaximumResponse+1))
	if backendDecoder.Decode(&output) != nil || backendDecoder.Decode(&struct{}{}) != io.EOF || output.Object != "list" || output.Model != s.config.BackendModel || len(output.Data) != len(input.Input) {
		http.Error(w, "backend response drift", http.StatusServiceUnavailable)
		return
	}
	seen := make([]bool, len(input.Input))
	data := make([]map[string]any, len(input.Input))
	for _, item := range output.Data {
		if item.Object != "embedding" || item.Index < 0 || item.Index >= len(input.Input) || seen[item.Index] || len(item.Embedding) != s.config.Model.Dimensions || !unitFinite(item.Embedding) {
			http.Error(w, "backend vector drift", http.StatusServiceUnavailable)
			return
		}
		seen[item.Index] = true
		data[item.Index] = map[string]any{"object": "embedding", "index": item.Index, "embedding": item.Embedding}
	}
	writeJSON(w, http.StatusOK, map[string]any{"object": "list", "data": data, "model": s.config.Model.ModelName, "model_info": s.config.Model})
}

func unitFinite(values []float64) bool {
	norm := 0.0
	for _, value := range values {
		if math.IsNaN(value) || math.IsInf(value, 0) {
			return false
		}
		norm += value * value
	}
	return norm >= 0.98*0.98 && norm <= 1.02*1.02
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func localHost(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && (ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast())
}

func (s *Server) String() string {
	return fmt.Sprintf("embedding evaluation proxy for %s at %s", s.config.Model.ModelName, s.config.BackendEndpoint)
}
