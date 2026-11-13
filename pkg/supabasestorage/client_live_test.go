package supabasestorage

import (
	"bytes"
	"context"
	"image"
	"image/png"
	"io"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/joho/godotenv"
)

// Opt in explicitly: this test creates and removes only UUID-scoped test objects.
// It never provisions buckets, changes bucket settings, or edits existing objects.
func TestLivePublicStorage(t *testing.T) {
	if os.Getenv("SUPABASE_STORAGE_LIVE_TEST") != "1" {
		t.Skip("set SUPABASE_STORAGE_LIVE_TEST=1 to verify configured Supabase buckets")
	}
	if err := godotenv.Load("../../.env"); err != nil && !os.IsNotExist(err) {
		t.Fatal("cannot read backend environment configuration")
	}
	client := New(os.Getenv("SUPABASE_URL"), os.Getenv("SUPABASE_SERVICE_ROLE_KEY"), &http.Client{Timeout: 15 * time.Second})
	if !client.Enabled() {
		t.Fatal("Supabase server configuration is missing or invalid")
	}
	cover, audio := os.Getenv("SUPABASE_KOTOBA_COVER_BUCKET"), os.Getenv("SUPABASE_KOTOBA_AUDIO_BUCKET")
	if cover == "" {
		cover = "kotoba-covers"
	}
	if audio == "" {
		audio = "kotoba-audio"
	}
	var picture bytes.Buffer
	if err := png.Encode(&picture, image.NewRGBA(image.Rect(0, 0, 2, 2))); err != nil {
		t.Fatal("cannot generate test image")
	}
	mp3, err := os.ReadFile("../../internal/kotoba/testdata/tone.mp3")
	if err != nil {
		t.Fatal("cannot read generated MP3 test tone")
	}
	reader := &http.Client{Timeout: 15 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	for _, sample := range []struct {
		name, bucket, mime, extension string
		data                          []byte
	}{
		{"cover", cover, "image/png", ".png", picture.Bytes()},
		{"audio", audio, "audio/mpeg", ".mp3", mp3},
	} {
		t.Run(sample.name, func(t *testing.T) {
			path := "_kotoba-integration-checks/" + uuid.NewString() + sample.extension
			// Register before upload: even an ambiguous network failure is cleaned up.
			t.Cleanup(func() {
				cleanupCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
				defer cancel()
				if err := client.Delete(cleanupCtx, sample.bucket, path); err != nil {
					t.Errorf("temporary %s cleanup failed: %v", sample.name, err)
				}
			})
			ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
			defer cancel()
			if err := client.Upload(ctx, sample.bucket, path, sample.mime, bytes.NewReader(sample.data)); err != nil {
				t.Fatalf("%s upload failed: %v", sample.name, err)
			}
			publicURL := client.PublicURL(sample.bucket, path)
			request, err := http.NewRequestWithContext(ctx, http.MethodGet, publicURL, nil)
			if err != nil {
				t.Fatal("cannot create public download request")
			}
			response, err := reader.Do(request)
			if err != nil {
				t.Fatal("public download could not connect")
			}
			content, readErr := io.ReadAll(io.LimitReader(response.Body, int64(len(sample.data))+1))
			response.Body.Close()
			if readErr != nil || response.StatusCode != http.StatusOK || !bytes.Equal(content, sample.data) {
				t.Fatalf("%s public download did not return the original bytes (HTTP %d)", sample.name, response.StatusCode)
			}
			t.Log("upload, public download, and byte comparison verified")
		})
	}
}
