package kotoba

import (
	"bytes"
	"context"
	"image"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"

	"nihon-no-hikari-api/pkg/supabasestorage"

	"github.com/google/uuid"
	"github.com/hajimehoshi/go-mp3"
)

// This smoke test exercises actual HTTP object upload, public object download, image
// decoding, and audio playback decoding without using production credentials.
func TestPublicMediaHTTPRoundTrip(t *testing.T) {
	objects := map[string][]byte{}
	var mu sync.Mutex
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		switch {
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
		case r.Method == "GET" && strings.HasPrefix(r.URL.Path, "/storage/v1/object/public/"):
			if r.Header.Get("Authorization") != "" {
				t.Error("public GET should not carry server credentials")
			}
			key := strings.TrimPrefix(r.URL.Path, "/storage/v1/object/public/")
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
		if !strings.HasPrefix(media.URL, server.URL+"/storage/v1/object/public/") || media.URLExpiresAt != nil {
			t.Fatalf("unexpected public upload metadata: %+v", media)
		}
		res, err := server.Client().Get(media.URL)
		if err != nil {
			t.Fatal(err)
		}
		actual, err := io.ReadAll(res.Body)
		res.Body.Close()
		if err != nil || res.StatusCode != 200 || !bytes.Equal(data, actual) {
			t.Fatalf("public media download failed: %v", err)
		}
		if kind == "cover" {
			if _, _, err := image.Decode(bytes.NewReader(actual)); err != nil {
				t.Fatal("public cover does not display", err)
			}
		} else {
			decoder, err := mp3.NewDecoder(bytes.NewReader(actual))
			if err != nil {
				t.Fatal(err)
			}
			n, err := io.Copy(io.Discard, decoder)
			if err != nil || n == 0 {
				t.Fatal("public MP3 does not decode for playback", err)
			}
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
