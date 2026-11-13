// Package supabasestorage implements the Storage REST API without exposing service keys.
package supabasestorage

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

var ErrDisabled = errors.New("supabase storage is not configured")

type Client struct {
	base, key string
	http      *http.Client
}

func New(projectURL, serviceKey string, client *http.Client) *Client {
	u, err := url.Parse(strings.TrimRight(strings.TrimSpace(projectURL), "/"))
	base := ""
	if err == nil && (u.Scheme == "https" || u.Scheme == "http") && u.Host != "" && u.User == nil && u.RawQuery == "" && u.Fragment == "" {
		base = strings.TrimRight(u.String(), "/")
	}
	c := http.Client{Timeout: 30 * time.Second}
	if client != nil {
		c = *client
		if c.Timeout == 0 {
			c.Timeout = 30 * time.Second
		}
	}
	// Redirects must never forward the service key to another origin.
	c.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	return &Client{base: base, key: strings.TrimSpace(serviceKey), http: &c}
}
func (c *Client) Enabled() bool { return c.base != "" && c.key != "" }
func escapedPath(parts ...string) string {
	encoded := []string{}
	for _, part := range parts {
		for _, segment := range strings.Split(part, "/") {
			encoded = append(encoded, url.PathEscape(segment))
		}
	}
	return strings.Join(encoded, "/")
}
func (c *Client) requestJSON(ctx context.Context, method, path, mime string, body io.Reader) (int, []byte, error) {
	if !c.Enabled() {
		return 0, nil, ErrDisabled
	}
	req, err := http.NewRequestWithContext(ctx, method, c.base+"/storage/v1/"+path, body)
	if err != nil {
		return 0, nil, errors.New("cannot create storage request")
	}
	req.Header.Set("Authorization", "Bearer "+c.key)
	req.Header.Set("apikey", c.key)
	req.Header.Set("Content-Type", mime)
	if method == http.MethodPost && strings.HasPrefix(path, "object/") && !strings.HasPrefix(path, "object/sign/") {
		req.Header.Set("x-upsert", "false")
		req.Header.Set("Cache-Control", "max-age=3600")
	}
	res, err := c.http.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return 0, nil, ctx.Err()
		}
		return 0, nil, errors.New("storage request failed")
	}
	defer res.Body.Close()
	data, err := io.ReadAll(io.LimitReader(res.Body, 1024*1024+1))
	if err != nil || len(data) > 1024*1024 {
		return 0, nil, errors.New("invalid storage response")
	}
	return res.StatusCode, data, nil
}
func (c *Client) request(ctx context.Context, method, path, mime string, body io.Reader) (int, error) {
	status, _, err := c.requestJSON(ctx, method, path, mime, body)
	return status, err
}

// PublicURL builds the permanent public object URL for a public bucket.
func (c *Client) PublicURL(bucket, path string) string {
	if c.base == "" || bucket == "" || path == "" {
		return ""
	}
	return c.base + "/storage/v1/object/public/" + escapedPath(bucket, path)
}
func checkStatus(status int, err error) error {
	if err != nil {
		return err
	}
	if status < 200 || status >= 300 {
		return fmt.Errorf("storage returned HTTP %d", status)
	}
	return nil
}
func (c *Client) Upload(ctx context.Context, bucket, path, mime string, body io.Reader) error {
	status, err := c.request(ctx, http.MethodPost, "object/"+escapedPath(bucket, path), mime, body)
	return checkStatus(status, err)
}
func (c *Client) Delete(ctx context.Context, bucket, path string) error {
	body, _ := json.Marshal(map[string]any{"prefixes": []string{path}})
	status, err := c.request(ctx, http.MethodDelete, "object/"+escapedPath(bucket), "application/json", bytes.NewReader(body))
	if err == nil && status == http.StatusNotFound {
		return nil
	}
	return checkStatus(status, err)
}

type Bucket struct {
	ID               string   `json:"id"`
	Name             string   `json:"name"`
	Public           bool     `json:"public"`
	FileSizeLimit    int64    `json:"file_size_limit"`
	AllowedMIMETypes []string `json:"allowed_mime_types"`
}

// EnsureBuckets is called by the explicit setup command, never by API startup.
func (c *Client) EnsureBuckets(ctx context.Context, buckets []Bucket) error {
	for _, bucket := range buckets {
		body, err := json.Marshal(bucket)
		if err != nil {
			return err
		}
		status, err := c.request(ctx, http.MethodGet, "bucket/"+escapedPath(bucket.ID), "application/json", nil)
		if err != nil {
			return err
		}
		method, path := http.MethodPut, "bucket/"+escapedPath(bucket.ID)
		if status == http.StatusNotFound {
			method, path = http.MethodPost, "bucket"
		} else if err := checkStatus(status, nil); err != nil {
			return err
		}
		status, err = c.request(ctx, method, path, "application/json", bytes.NewReader(body))
		if err := checkStatus(status, err); err != nil {
			return err
		}
	}
	return nil
}
