package validator

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

type ValidationError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
	Line    int    `json:"line"`
}

type ValidationResponse struct {
	Status string            `json:"status"`
	Errors []ValidationError `json:"errors,omitempty"`
}

type Client struct {
	baseURL    string
	httpClient *http.Client
}

func NewClient(baseURL string) *Client {
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		httpClient: &http.Client{
			Transport: otelhttp.NewTransport(http.DefaultTransport),
			Timeout:   10 * time.Second,
		},
	}
}

func (c *Client) Validate(ctx context.Context, configRaw string) (*ValidationResponse, error) {
	url := c.baseURL + "/validate"

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewBufferString(configRaw))
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Content-Type", "text/plain")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("calling validator: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("validator returned status %d: %s", resp.StatusCode, string(body))
	}

	var result ValidationResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("parsing response: %w", err)
	}

	return &result, nil
}

// FetchDeployToml fetches deploy.toml from a GitHub repo at a specific commit.
func FetchDeployToml(ctx context.Context, repoURL, commitSHA string) (string, error) {
	rawURL, err := buildRawURL(repoURL, commitSHA)
	if err != nil {
		return "", err
	}

	slog.Info("fetching deploy.toml", "url", rawURL)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return "", fmt.Errorf("creating request: %w", err)
	}

	client := &http.Client{
		Transport: otelhttp.NewTransport(http.DefaultTransport),
		Timeout:   15 * time.Second,
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("fetching deploy.toml: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return "", fmt.Errorf("deploy.toml not found in repo at commit %s", commitSHA)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("GitHub returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("reading deploy.toml: %w", err)
	}

	return string(body), nil
}

// buildRawURL converts a GitHub repo URL to a raw content URL for deploy.toml.
// Supports: https://github.com/owner/repo[.git]
func buildRawURL(repoURL, commitSHA string) (string, error) {
	u := strings.TrimSuffix(repoURL, ".git")
	const prefix = "https://github.com/"
	if !strings.HasPrefix(u, prefix) {
		return "", fmt.Errorf("unsupported repo URL format: %s (only GitHub HTTPS URLs supported)", repoURL)
	}
	ownerRepo := strings.TrimPrefix(u, prefix)
	return fmt.Sprintf("https://raw.githubusercontent.com/%s/%s/deploy.toml", ownerRepo, commitSHA), nil
}
