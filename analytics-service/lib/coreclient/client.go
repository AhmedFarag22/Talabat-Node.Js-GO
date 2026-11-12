// Package coreclient is the typed HTTP client to core-service. Mirrors
// lib/core-client/* in order-service.
package coreclient

import (
	"strings"
	"time"

	"github.com/quickbite/analytics-service/pkg/httpclient"
)

type Client struct {
	baseURL string
	apiKey  string
	http    *httpclient.Client
}

func New(baseURL, apiKey string) *Client {
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		apiKey:  apiKey,
		http:    httpclient.New(5 * time.Second),
	}
}

func (c *Client) headers() map[string]string {
	return map[string]string{"api-key": c.apiKey}
}
