// Explicit provisioning for the two Kotoba buckets; does not run on API startup.
package main

import (
	"context"
	"log"
	"os"
	"time"

	"nihon-no-hikari-api/internal/kotoba"
	"nihon-no-hikari-api/pkg/supabasestorage"

	"github.com/joho/godotenv"
)

func main() {
	_ = godotenv.Load()
	client := supabasestorage.New(os.Getenv("SUPABASE_URL"), os.Getenv("SUPABASE_SERVICE_ROLE_KEY"), nil)
	if !client.Enabled() {
		log.Fatal("Set SUPABASE_URL and SUPABASE_SERVICE_ROLE_KEY before provisioning buckets")
	}
	cover, audio := os.Getenv("SUPABASE_KOTOBA_COVER_BUCKET"), os.Getenv("SUPABASE_KOTOBA_AUDIO_BUCKET")
	if cover == "" {
		cover = "kotoba-covers"
	}
	if audio == "" {
		audio = "kotoba-audio"
	}
	if cover == audio {
		log.Fatal("Cover and audio must use different buckets")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	err := client.EnsureBuckets(ctx, []supabasestorage.Bucket{
		{ID: cover, Name: cover, Public: false, FileSizeLimit: kotoba.MaxCoverBytes, AllowedMIMETypes: []string{"image/png", "image/jpeg"}},
		{ID: audio, Name: audio, Public: false, FileSizeLimit: kotoba.MaxAudioBytes, AllowedMIMETypes: []string{"audio/mpeg"}},
	})
	if err != nil {
		log.Fatalf("Bucket setup failed: %v", err)
	}
	log.Print("Kotoba private buckets and upload limits are configured")
}
