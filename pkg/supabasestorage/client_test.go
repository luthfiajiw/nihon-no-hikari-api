package supabasestorage

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestStorageREST(t *testing.T) {
	methods := []string{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer server-secret" || r.Header.Get("apikey") != "server-secret" {
			t.Error("missing server credentials")
		}
		methods = append(methods, r.Method+" "+r.URL.Path)
		switch r.Method {
		case "POST":
			b, _ := io.ReadAll(r.Body)
			if string(b) != "media bytes" || r.Header.Get("Content-Type") != "image/png" || r.Header.Get("x-upsert") != "false" {
				t.Error("bad upload request")
			}
			w.WriteHeader(201)
		case "DELETE":
			var body struct {
				Prefixes []string `json:"prefixes"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil || len(body.Prefixes) != 1 || body.Prefixes[0] != "a b/cover.png" {
				t.Error("bad delete body")
			}
			w.WriteHeader(200)
		}
	}))
	defer server.Close()
	client := New(server.URL, "server-secret", nil)
	if err := client.Upload(context.Background(), "covers", "a b/cover.png", "image/png", strings.NewReader("media bytes")); err != nil {
		t.Fatal(err)
	}
	if err := client.Delete(context.Background(), "covers", "a b/cover.png"); err != nil {
		t.Fatal(err)
	}
	if len(methods) != 2 || methods[0] != "POST /storage/v1/object/covers/a b/cover.png" || methods[1] != "DELETE /storage/v1/object/covers" {
		t.Fatal(methods)
	}
}

func TestSignedURLs(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Method != "POST" || r.URL.Path != "/storage/v1/object/sign/covers" || r.Header.Get("Authorization") != "Bearer secret" || r.Header.Get("x-upsert") != "" {
			t.Error("invalid signing request")
		}
		var body struct {
			Paths     []string `json:"paths"`
			ExpiresIn int      `json:"expiresIn"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.ExpiresIn != 900 || len(body.Paths) > 100 {
			t.Error("invalid signing body")
		}
		entries := []map[string]any{}
		for _, path := range body.Paths {
			entries = append(entries, map[string]any{"path": path, "signedURL": "/object/sign/covers/" + escapedPath(path) + "?token=temporary", "error": nil})
		}
		_ = json.NewEncoder(w).Encode(entries)
	}))
	defer server.Close()
	paths := []string{"a b/cover.png", "a b/cover.png"}
	for i := range 100 {
		paths = append(paths, fmt.Sprintf("%d.png", i))
	}
	urls, err := New(server.URL, "secret", nil).SignedURLs(context.Background(), "covers", paths, 15*time.Minute)
	if err != nil || calls != 2 || len(urls) != 101 || urls["a b/cover.png"] != server.URL+"/storage/v1/object/sign/covers/a%20b/cover.png?token=temporary" {
		t.Fatalf("bad signed URLs: calls=%d size=%d error=%v", calls, len(urls), err)
	}
}

func TestSignedURLsRejectIncompleteOrUnsafeResponses(t *testing.T) {
	for _, response := range []string{
		`[]`, `not json`,
		`[{"path":"id.png","error":"not found"}]`,
		`[{"path":"other.png","signedURL":"/object/sign/covers/other.png?token=x"}]`,
		`[{"path":"id.png","signedURL":"https://evil.test/object/sign/covers/id.png?token=x"}]`,
		`[{"path":"id.png","signedURL":"/object/sign/covers/id.png"}]`,
		`[{"path":"id.png","signedURL":"/object/public/covers/id.png?token=x"}]`,
	} {
		t.Run(response, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = io.WriteString(w, response) }))
			defer server.Close()
			if _, err := New(server.URL, "server-secret", nil).SignedURLs(context.Background(), "covers", []string{"id.png"}, time.Minute); err == nil || strings.Contains(err.Error(), "server-secret") {
				t.Fatalf("unsafe signing response accepted: %v", err)
			}
		})
	}
}
func TestStorageFailuresAndRedirects(t *testing.T) {
	for _, status := range []int{301, 400, 401, 403, 500} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			followed := false
			target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { followed = true }))
			defer target.Close()
			origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Location", target.URL)
				w.WriteHeader(status)
				_, _ = w.Write([]byte("server-secret"))
			}))
			defer origin.Close()
			client := New(origin.URL, "server-secret", nil)
			if err := client.Upload(context.Background(), "covers", "id.png", "image/png", bytes.NewReader(nil)); err == nil || strings.Contains(err.Error(), "server-secret") {
				t.Fatalf("unsafe error: %v", err)
			}
			if followed {
				t.Fatal("followed credential-bearing redirect")
			}
		})
	}
	client := New("https://example.supabase.co", "", nil)
	if client.Enabled() || client.Upload(context.Background(), "x", "y", "z", nil) != ErrDisabled {
		t.Fatal("missing key must disable writes")
	}
	if _, err := client.SignedURLs(context.Background(), "covers", []string{"id.png"}, time.Minute); err != ErrDisabled {
		t.Fatal("private media signing must require credentials")
	}
	for _, url := range []string{"invalid", "file:///etc/passwd", "https://user:pass@example.com", "https://example.com?key=secret"} {
		if New(url, "key", nil).Enabled() {
			t.Fatalf("accepted invalid project URL: %s", url)
		}
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-time.After(200 * time.Millisecond):
		}
	}))
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if err := New(server.URL, "key", nil).Delete(ctx, "covers", "id.png"); err == nil {
		t.Fatal("cancellation ignored")
	}
}
func TestEnsureBuckets(t *testing.T) {
	calls := []string{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, r.Method+" "+r.URL.Path)
		if r.Method == "GET" && strings.HasSuffix(r.URL.Path, "covers") {
			w.WriteHeader(404)
			return
		}
		if r.Method == "GET" {
			return
		}
		var b Bucket
		data, _ := io.ReadAll(r.Body)
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(data, &fields); err != nil || string(fields["public"]) != "false" {
			t.Error("private updates must explicitly send public=false")
		}
		if err := json.Unmarshal(data, &b); err != nil || b.Public || b.FileSizeLimit == 0 || len(b.AllowedMIMETypes) == 0 {
			t.Error("missing bucket restrictions")
		}
	}))
	defer server.Close()
	buckets := []Bucket{{ID: "covers", Name: "covers", Public: false, FileSizeLimit: 2097152, AllowedMIMETypes: []string{"image/png"}}, {ID: "audio", Name: "audio", Public: false, FileSizeLimit: 1048576, AllowedMIMETypes: []string{"audio/mpeg"}}}
	if err := New(server.URL, "key", nil).EnsureBuckets(context.Background(), buckets); err != nil {
		t.Fatal(err)
	}
	if strings.Join(calls, "|") != "GET /storage/v1/bucket/covers|POST /storage/v1/bucket|GET /storage/v1/bucket/audio|PUT /storage/v1/bucket/audio" {
		t.Fatal(calls)
	}
}
