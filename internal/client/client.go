package client

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Client is a thin wrapper around net/http that understands liaison-cloud's
// response envelope and injects the bearer token on every call.
type Client struct {
	Server     string
	Token      string
	HTTPClient *http.Client
	Verbose    bool
}

// New builds a Client. server is the base URL (e.g. https://liaison.cloud).
// insecure disables TLS verification — useful for self-signed testing.
func New(server, token string, insecure, verbose bool) *Client {
	tr := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: insecure},
	}
	return &Client{
		Server:     strings.TrimRight(server, "/"),
		Token:      token,
		HTTPClient: &http.Client{Transport: tr, Timeout: 30 * time.Second},
		Verbose:    verbose,
	}
}

// Do performs a request against /api/v1/<path>. The body is marshalled as JSON.
// The decoded "data" field of the response envelope is returned as raw bytes
// so callers can unmarshal it into a concrete type.
func (c *Client) Do(method, path string, query url.Values, body any) ([]byte, error) {
	if c.Server == "" {
		return nil, fmt.Errorf("server URL not configured (use --server or %s)", "LIAISON_SERVER")
	}

	u, err := url.Parse(c.Server + path)
	if err != nil {
		return nil, fmt.Errorf("build url: %w", err)
	}
	if len(query) > 0 {
		u.RawQuery = query.Encode()
	}

	var reqBody io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("marshal body: %w", err)
		}
		reqBody = bytes.NewReader(data)
	}

	req, err := http.NewRequest(method, u.String(), reqBody)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	if reqBody != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}

	if c.Verbose {
		fmt.Printf("> %s %s\n", method, u.String())
	}

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http request: %w", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}

	if c.Verbose {
		fmt.Printf("< HTTP %d\n", resp.StatusCode)
	}

	// Detect auth failures early with a clean message.
	if resp.StatusCode == http.StatusUnauthorized {
		return nil, fmt.Errorf("unauthorized (HTTP 401): token missing or invalid — run `liaison login`")
	}

	// Liaison envelope: {"code":200,"message":"success","data":{...}}.
	// Use json.RawMessage so we can return data as bytes without double-decoding.
	var envelope struct {
		Code     int             `json:"code"`
		Message  string          `json:"message"`
		Reason   string          `json:"reason,omitempty"`
		Data     json.RawMessage `json:"data,omitempty"`
		Metadata json.RawMessage `json:"metadata,omitempty"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		// Non-envelope response (e.g. proxy error). Return the raw body in the error.
		return nil, fmt.Errorf("http %d: %s", resp.StatusCode, strings.TrimSpace(string(raw)))
	}

	if envelope.Code != 0 && envelope.Code != 200 {
		msg := envelope.Message
		if envelope.Reason != "" {
			msg = fmt.Sprintf("%s (%s)", msg, envelope.Reason)
		}
		return nil, fmt.Errorf("api error %d: %s", envelope.Code, msg)
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("http %d: %s", resp.StatusCode, envelope.Message)
	}
	return envelope.Data, nil
}

// Get is a convenience wrapper around Do for GET requests.
func (c *Client) Get(path string, query url.Values) ([]byte, error) {
	return c.Do(http.MethodGet, path, query, nil)
}

// Post is a convenience wrapper around Do for POST requests.
func (c *Client) Post(path string, body any) ([]byte, error) {
	return c.Do(http.MethodPost, path, nil, body)
}

// Put is a convenience wrapper around Do for PUT requests.
func (c *Client) Put(path string, body any) ([]byte, error) {
	return c.Do(http.MethodPut, path, nil, body)
}

// Delete is a convenience wrapper around Do for DELETE requests.
func (c *Client) Delete(path string) ([]byte, error) {
	return c.Do(http.MethodDelete, path, nil, nil)
}
