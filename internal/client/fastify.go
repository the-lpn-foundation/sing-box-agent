package client

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/oglenyaboss/sing-box-agent/internal/models"
)

const defaultTimeout = 10 * time.Second

type Options struct {
	BaseURL    string
	Token      string
	Secret     string
	CACertPath string
	ServerName string
	Timeout    time.Duration
	Retry      RetryConfig
	HTTPClient *http.Client
}

type Client struct {
	baseURL    string
	token      string
	secret     string
	httpClient *http.Client
	retry      RetryConfig
}

type Config struct {
	Version   int              `json:"version"`
	Inbounds  []models.Inbound `json:"inbounds,omitempty"`
	Users     []models.User    `json:"users,omitempty"`
	Timestamp time.Time        `json:"timestamp,omitempty"`
}

type Status struct {
	State     string                 `json:"state"`
	Version   int                    `json:"version,omitempty"`
	Message   string                 `json:"message,omitempty"`
	UpdatedAt time.Time              `json:"updatedAt,omitempty"`
	Details   map[string]interface{} `json:"details,omitempty"`
}

type apiError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type envelope[T any] struct {
	Success bool      `json:"success"`
	Data    T         `json:"data"`
	Error   *apiError `json:"error,omitempty"`
}

func NewFastifyClient(opts Options) (*Client, error) {
	if strings.TrimSpace(opts.BaseURL) == "" {
		return nil, fmt.Errorf("base URL is required")
	}
	if strings.TrimSpace(opts.Token) == "" {
		return nil, fmt.Errorf("token is required")
	}
	if strings.TrimSpace(opts.Secret) == "" {
		return nil, fmt.Errorf("secret is required")
	}

	httpClient, err := buildHTTPClient(opts)
	if err != nil {
		return nil, err
	}

	return &Client{
		baseURL:    strings.TrimRight(opts.BaseURL, "/"),
		token:      opts.Token,
		secret:     opts.Secret,
		httpClient: httpClient,
		retry:      opts.Retry.withDefaults(),
	}, nil
}

func (c *Client) Heartbeat(ctx context.Context) error {
	return c.doJSON(ctx, http.MethodPost, "/api/agents/heartbeat", map[string]time.Time{"timestamp": time.Now().UTC()}, nil)
}

func (c *Client) FetchConfig(ctx context.Context) (*Config, error) {
	var cfg Config
	if err := c.doJSON(ctx, http.MethodGet, "/api/agents/config", nil, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func (c *Client) ReportStatus(ctx context.Context, status Status) error {
	if status.UpdatedAt.IsZero() {
		status.UpdatedAt = time.Now().UTC()
	}
	return c.doJSON(ctx, http.MethodPost, "/api/agents/status", status, nil)
}

func (c *Client) doJSON(ctx context.Context, method, path string, payload, out interface{}) error {
	var response *http.Response

	err := Retry(ctx, c.retry, func(opCtx context.Context) (bool, error) {
		var body io.Reader
		if payload != nil {
			raw, err := json.Marshal(payload)
			if err != nil {
				return false, fmt.Errorf("marshal request payload: %w", err)
			}
			body = bytes.NewReader(raw)
		}

		req, err := http.NewRequestWithContext(opCtx, method, c.baseURL+path, body)
		if err != nil {
			return false, fmt.Errorf("create request: %w", err)
		}

		req.Header.Set("Authorization", "Bearer "+c.token)
		req.Header.Set("X-Agent-Secret", c.secret)
		req.Header.Set("Accept", "application/json")
		if payload != nil {
			req.Header.Set("Content-Type", "application/json")
		}

		resp, err := c.httpClient.Do(req)
		if err != nil {
			return true, fmt.Errorf("perform request: %w", err)
		}

		if resp.StatusCode >= http.StatusInternalServerError {
			_, _ = io.Copy(io.Discard, resp.Body)
			_ = resp.Body.Close()
			return true, fmt.Errorf("retryable server status %d", resp.StatusCode)
		}

		response = resp
		return false, nil
	})
	if err != nil {
		return err
	}
	defer func() { _ = response.Body.Close() }()

	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		body, _ := io.ReadAll(response.Body)
		return fmt.Errorf("request failed with status %d: %s", response.StatusCode, strings.TrimSpace(string(body)))
	}

	if out == nil {
		return nil
	}

	var wrapped envelope[json.RawMessage]
	if err := json.NewDecoder(response.Body).Decode(&wrapped); err != nil {
		return fmt.Errorf("decode envelope: %w", err)
	}
	if !wrapped.Success {
		if wrapped.Error == nil {
			return fmt.Errorf("API returned unsuccessful response")
		}
		return fmt.Errorf("API error %s: %s", wrapped.Error.Code, wrapped.Error.Message)
	}
	if len(wrapped.Data) == 0 {
		return nil
	}
	if err := json.Unmarshal(wrapped.Data, out); err != nil {
		return fmt.Errorf("decode envelope data: %w", err)
	}

	return nil
}

func buildHTTPClient(opts Options) (*http.Client, error) {
	if opts.HTTPClient != nil {
		return opts.HTTPClient, nil
	}

	timeout := opts.Timeout
	if timeout <= 0 {
		timeout = defaultTimeout
	}

	transport, ok := http.DefaultTransport.(*http.Transport)
	if !ok {
		return nil, fmt.Errorf("default transport is not *http.Transport")
	}
	transport = transport.Clone()
	tlsConfig := &tls.Config{MinVersion: tls.VersionTLS12}
	if strings.TrimSpace(opts.ServerName) != "" {
		tlsConfig.ServerName = strings.TrimSpace(opts.ServerName)
	}
	if strings.TrimSpace(opts.CACertPath) != "" {
		pem, err := os.ReadFile(opts.CACertPath)
		if err != nil {
			return nil, fmt.Errorf("read custom CA certificate: %w", err)
		}
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(pem) {
			return nil, fmt.Errorf("parse custom CA certificate")
		}
		tlsConfig.RootCAs = pool
	}
	transport.TLSClientConfig = tlsConfig

	return &http.Client{Timeout: timeout, Transport: transport}, nil
}
