package supabasestorage

import (
	"bytes"
	"context"
	"encoding/json"
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

func TestPublicURL(t *testing.T) {
	client := New("https://example.supabase.co/", "secret", nil)
	if got, want := client.PublicURL("covers", "a b/cover.png"), "https://example.supabase.co/storage/v1/object/public/covers/a%20b/cover.png"; got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
	if client.PublicURL("", "cover.png") != "" || client.PublicURL("covers", "") != "" {
		t.Fatal("empty bucket or path must yield empty URL")
	}
	if New("invalid", "secret", nil).PublicURL("covers", "cover.png") != "" {
		t.Fatal("invalid base URL must yield empty URL")
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
		if err := json.Unmarshal(data, &fields); err != nil || string(fields["public"]) != "true" {
			t.Error("public updates must explicitly send public=true")
		}
		if err := json.Unmarshal(data, &b); err != nil || !b.Public || b.FileSizeLimit == 0 || len(b.AllowedMIMETypes) == 0 {
			t.Error("missing bucket restrictions")
		}
	}))
	defer server.Close()
	buckets := []Bucket{{ID: "covers", Name: "covers", Public: true, FileSizeLimit: 2097152, AllowedMIMETypes: []string{"image/png"}}, {ID: "audio", Name: "audio", Public: true, FileSizeLimit: 1048576, AllowedMIMETypes: []string{"audio/mpeg"}}}
	if err := New(server.URL, "key", nil).EnsureBuckets(context.Background(), buckets); err != nil {
		t.Fatal(err)
	}
	if strings.Join(calls, "|") != "GET /storage/v1/bucket/covers|POST /storage/v1/bucket|GET /storage/v1/bucket/audio|PUT /storage/v1/bucket/audio" {
		t.Fatal(calls)
	}
}
