package kotoba

import (
	"bytes"
	"context"
	"encoding/json"
	"image"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"nihon-no-hikari-api/pkg/supabasestorage"

	"github.com/google/uuid"
	"github.com/hajimehoshi/go-mp3"
)

// This smoke test exercises actual HTTP object upload, private signed download, image
// decoding, and audio playback decoding without using production credentials.
func TestPrivateMediaHTTPRoundTrip(t *testing.T) {
	objects := map[string][]byte{}
	type grant struct {
		key     string
		expires time.Time
	}
	grants := map[string]grant{}
	now := time.Now()
	var mu sync.Mutex
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		switch {
		case r.Method == "POST" && strings.HasPrefix(r.URL.Path, "/storage/v1/object/sign/"):
			if r.Header.Get("Authorization") != "Bearer server-only" {
				w.WriteHeader(401)
				return
			}
			bucket := strings.TrimPrefix(r.URL.Path, "/storage/v1/object/sign/")
			var body struct {
				Paths     []string `json:"paths"`
				ExpiresIn int      `json:"expiresIn"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.ExpiresIn != 900 {
				t.Error("invalid signing payload")
				w.WriteHeader(400)
				return
			}
			entries := []map[string]any{}
			for _, path := range body.Paths {
				token := uuid.NewString()
				key := bucket + "/" + path
				grants[token] = grant{key: key, expires: now.Add(time.Duration(body.ExpiresIn) * time.Second)}
				entries = append(entries, map[string]any{"path": path, "signedURL": "/object/sign/" + key + "?token=" + token, "error": nil})
			}
			_ = json.NewEncoder(w).Encode(entries)
		case r.Method == "POST" && strings.HasPrefix(r.URL.Path, "/storage/v1/object/"):
			if r.Header.Get("Authorization") != "Bearer server-only" {
				w.WriteHeader(401)
				return
			}
			data, err := io.ReadAll(r.Body)
			if err != nil {
				t.Error(err)
				w.WriteHeader(500)
				return
			}
			objects[strings.TrimPrefix(r.URL.Path, "/storage/v1/object/")] = data
			w.WriteHeader(201)
		case r.Method == "GET" && strings.HasPrefix(r.URL.Path, "/storage/v1/object/sign/"):
			if r.Header.Get("Authorization") != "" {
				t.Error("signed GET should not carry server credentials")
			}
			key := strings.TrimPrefix(r.URL.Path, "/storage/v1/object/sign/")
			grant, ok := grants[r.URL.Query().Get("token")]
			if !ok || grant.key != key || !now.Before(grant.expires) {
				w.WriteHeader(403)
				return
			}
			data, ok := objects[key]
			if !ok {
				w.WriteHeader(404)
				return
			}
			if strings.HasSuffix(key, ".png") {
				w.Header().Set("Content-Type", "image/png")
			} else {
				w.Header().Set("Content-Type", "audio/mpeg")
			}
			_, _ = w.Write(data)
		default:
			w.WriteHeader(404)
		}
	}))
	defer server.Close()
	storage := supabasestorage.New(server.URL, "server-only", nil)
	svc := NewService(transactionStub{}, &repositoryStub{}, storage, "covers", "audio")
	for _, kind := range []string{"cover", "audio"} {
		filename, mime, data := "cover.png", "image/png", pngFixture(t)
		if kind == "audio" {
			filename, mime, data = "word.mp3", "audio/mpeg", audioFixture(t)
		}
		media, err := svc.Upload(context.Background(), uuid.New(), kind, filename, mime, bytes.NewReader(data))
		if err != nil {
			t.Fatal(err)
		}
		res, err := server.Client().Get(media.URL)
		if err != nil {
			t.Fatal(err)
		}
		actual, err := io.ReadAll(res.Body)
		res.Body.Close()
		if err != nil || res.StatusCode != 200 || !bytes.Equal(data, actual) {
			t.Fatalf("private signed media download failed: %v", err)
		}
		if kind == "cover" {
			if _, _, err := image.Decode(bytes.NewReader(actual)); err != nil {
				t.Fatal("private cover does not display", err)
			}
		} else {
			decoder, err := mp3.NewDecoder(bytes.NewReader(actual))
			if err != nil {
				t.Fatal(err)
			}
			n, err := io.Copy(io.Discard, decoder)
			if err != nil || n == 0 {
				t.Fatal("private MP3 does not decode for playback", err)
			}
		}
		if media.URLExpiresAt == nil {
			t.Fatal("missing preview expiration")
		}
		for _, deniedURL := range []string{strings.Split(media.URL, "?")[0], strings.Replace(strings.Split(media.URL, "?")[0], "/object/sign/", "/object/public/", 1)} {
			res, err := server.Client().Get(deniedURL)
			if err != nil {
				t.Fatal(err)
			}
			res.Body.Close()
			if res.StatusCode < 400 {
				t.Fatal("private object accessible without a signed token")
			}
		}
		mu.Lock()
		now = now.Add(MediaURLTTL + time.Second)
		mu.Unlock()
		res, err = server.Client().Get(media.URL)
		if err != nil {
			t.Fatal(err)
		}
		res.Body.Close()
		if res.StatusCode != 403 {
			t.Fatal("expired signed URL still accepted")
		}
	}
}

func FuzzMediaValidation(f *testing.F) {
	f.Add([]byte("ID3"))
	f.Add([]byte{0xff, 0xfb, 0x90, 0x64})
	data, err := os.ReadFile("testdata/tone.mp3")
	if err != nil {
		f.Fatal(err)
	}
	f.Add(data)
	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) > MaxAudioBytes {
			return
		}
		_, _, _, _ = ValidateMedia("audio", "word.mp3", "audio/mpeg", bytes.NewReader(data))
	})
}
