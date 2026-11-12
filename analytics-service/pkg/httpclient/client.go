// Package httpclient is a tiny wrapper over net/http with timeouts, JSON
// helpers, and capped retry on 5xx. App-agnostic.
package httpclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

type Client struct {
	httpClient *http.Client
	maxRetries int
}

func New(timeout time.Duration) *Client {
	return &Client{
		httpClient: &http.Client{Timeout: timeout},
		maxRetries: 3,
	}
}

// GetJSON does a GET and decodes the JSON body into out. Headers are added
// before the request fires. 5xx → retry with capped exponential backoff.
func (c *Client) GetJSON(ctx context.Context, url string, headers map[string]string, out any) error {
	return c.doJSON(ctx, http.MethodGet, url, headers, nil, out)
}

// PostJSON sends body marshalled as JSON and decodes the response.
func (c *Client) PostJSON(ctx context.Context, url string, headers map[string]string, body, out any) error {
	return c.doJSON(ctx, http.MethodPost, url, headers, body, out)
}

func (c *Client) doJSON(ctx context.Context, method, url string, headers map[string]string, body, out any) error {
	var bodyBytes []byte
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("marshal body: %w", err)
		}
		bodyBytes = b
	}

	var lastErr error
	for attempt := 0; attempt <= c.maxRetries; attempt++ {
		req, err := http.NewRequestWithContext(ctx, method, url, bytes.NewReader(bodyBytes))
		if err != nil {
			return fmt.Errorf("new request: %w", err)
		}
		if bodyBytes != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		req.Header.Set("Accept", "application/json")
		for k, v := range headers {
			req.Header.Set(k, v)
		}

		resp, err := c.httpClient.Do(req)
		if err != nil {
			lastErr = err
		} else if resp.StatusCode >= 500 {
			body, _ := io.ReadAll(resp.Body)
			_ = resp.Body.Close()
			lastErr = fmt.Errorf("upstream %d: %s", resp.StatusCode, string(body))
		} else {
			defer resp.Body.Close()
			if resp.StatusCode >= 400 {
				body, _ := io.ReadAll(resp.Body)
				return fmt.Errorf("upstream %d: %s", resp.StatusCode, string(body))
			}
			if out != nil {
				if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
					return fmt.Errorf("decode response: %w", err)
				}
			}
			return nil
		}

		if attempt < c.maxRetries {
			backoff := time.Duration(1<<attempt) * 200 * time.Millisecond
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(backoff):
			}
		}
	}
	return fmt.Errorf("after %d retries: %w", c.maxRetries, lastErr)
}
