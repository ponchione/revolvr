package embedding

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"
)

const (
	DefaultTimeout          = 60 * time.Second
	DefaultMaxBatchInputs   = 64
	DefaultMaxInputBytes    = 256 << 10
	DefaultMaxBatchBytes    = 1 << 20
	DefaultMaxResponseBytes = 64 << 20
)

type HTTPClient interface {
	Do(*http.Request) (*http.Response, error)
}

type Config struct {
	Endpoint         string
	ExpectedModel    EmbeddingModelInfo
	Timeout          time.Duration
	MaxBatchInputs   int
	MaxInputBytes    int
	MaxBatchBytes    int
	MaxResponseBytes int64
	HTTPClient       HTTPClient
}

func (c Config) normalize() (Config, error) {
	c.Endpoint = strings.TrimSpace(c.Endpoint)
	if c.Endpoint == "" {
		return Config{}, errors.New("embedding client: endpoint is required")
	}
	parsed, err := url.Parse(c.Endpoint)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" || parsed.Opaque != "" {
		return Config{}, errors.New("embedding client: endpoint must be an absolute local URL without credentials, query, or fragment")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return Config{}, errors.New("embedding client: endpoint scheme must be http or https")
	}
	if !localHost(parsed.Hostname()) {
		return Config{}, fmt.Errorf("embedding client: endpoint host %q is not local or private", parsed.Hostname())
	}
	cleanPath := path.Clean(parsed.Path)
	if cleanPath == "." {
		cleanPath = ""
	}
	if cleanPath != strings.TrimSuffix(parsed.Path, "/") {
		return Config{}, errors.New("embedding client: endpoint path must be normalized")
	}
	parsed.Path = cleanPath
	c.Endpoint = strings.TrimSuffix(parsed.String(), "/")
	if err := c.ExpectedModel.Validate(); err != nil {
		return Config{}, err
	}
	if c.Timeout == 0 {
		c.Timeout = DefaultTimeout
	}
	if c.MaxBatchInputs == 0 {
		c.MaxBatchInputs = DefaultMaxBatchInputs
	}
	if c.MaxInputBytes == 0 {
		c.MaxInputBytes = DefaultMaxInputBytes
	}
	if c.MaxBatchBytes == 0 {
		c.MaxBatchBytes = DefaultMaxBatchBytes
	}
	if c.MaxResponseBytes == 0 {
		c.MaxResponseBytes = DefaultMaxResponseBytes
	}
	if c.Timeout <= 0 || c.MaxBatchInputs <= 0 || c.MaxInputBytes <= 0 || c.MaxBatchBytes <= 0 || c.MaxResponseBytes <= 0 {
		return Config{}, errors.New("embedding client: timeout and input/response bounds must be positive")
	}
	if c.MaxInputBytes > c.MaxBatchBytes {
		return Config{}, errors.New("embedding client: max input bytes cannot exceed max batch bytes")
	}
	if c.HTTPClient == nil {
		c.HTTPClient = &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		}}
	}
	return c, nil
}

func localHost(host string) bool {
	host = strings.TrimSuffix(strings.ToLower(strings.TrimSpace(host)), ".")
	if host == "" {
		return false
	}
	if host == "localhost" || strings.HasSuffix(host, ".localhost") {
		return true
	}
	if ip := net.ParseIP(host); ip != nil {
		return ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast()
	}
	// A single DNS label is resolved only inside the caller's local resolver
	// domain (for example the Compose service name "embedding-service").
	return !strings.Contains(host, ".")
}
