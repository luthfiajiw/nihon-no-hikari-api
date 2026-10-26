package kotoba

import (
	"bytes"
	"errors"
	"image"
	"image/jpeg"
	"image/png"
	"os"
	"testing"
)

func pngFixture(t *testing.T) []byte {
	t.Helper()
	var b bytes.Buffer
	if err := png.Encode(&b, image.NewRGBA(image.Rect(0, 0, 8, 8))); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}
func audioFixture(t *testing.T) []byte {
	t.Helper()
	data, err := os.ReadFile("testdata/tone.mp3")
	if err != nil {
		t.Fatal(err)
	}
	return data
}
func padMP3(data []byte, size int) []byte {
	// Add a well-formed ID3v2 padding block to exercise the exact byte limit.
	if len(data) >= 10 && string(data[:3]) == "ID3" {
		tagSize := 0
		for _, b := range data[6:10] {
			tagSize = (tagSize << 7) | int(b)
		}
		data = data[10+tagSize:]
	}
	padding := size - len(data) - 10
	result := make([]byte, 10+padding)
	copy(result, []byte{'I', 'D', '3', 3, 0, 0})
	n := padding
	for i := 9; i >= 6; i-- {
		result[i] = byte(n & 127)
		n >>= 7
	}
	return append(result, data...)
}
func TestMediaValidation(t *testing.T) {
	pngData := pngFixture(t)
	mp3 := audioFixture(t)
	var jpg bytes.Buffer
	if err := jpeg.Encode(&jpg, image.NewRGBA(image.Rect(0, 0, 4, 4)), nil); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, kind, filename, mime string
		data                       []byte
		valid, large               bool
	}{
		{name: "png", kind: "cover", filename: "cover.png", mime: "image/png", data: pngData, valid: true},
		{name: "jpeg", kind: "cover", filename: "cover.jpeg", mime: "image/jpeg", data: jpg.Bytes(), valid: true},
		{name: "mp3", kind: "audio", filename: "word.mp3", mime: "audio/mpeg", data: mp3, valid: true},
		{name: "mp3 alias", kind: "audio", filename: "word.MP3", mime: "audio/mp3", data: mp3, valid: true},
		{name: "exact image size", kind: "cover", filename: "cover.png", mime: "image/png", data: append(append([]byte{}, pngData...), make([]byte, MaxCoverBytes-len(pngData))...), valid: true},
		{name: "exact audio size", kind: "audio", filename: "word.mp3", mime: "audio/mpeg", data: padMP3(mp3, MaxAudioBytes), valid: true},
		{name: "large cover", kind: "cover", filename: "cover.png", data: make([]byte, MaxCoverBytes+1), large: true},
		{name: "large audio", kind: "audio", filename: "word.mp3", data: make([]byte, MaxAudioBytes+1), large: true},
		{name: "empty", kind: "cover", filename: "cover.png"},
		{name: "mime spoof", kind: "cover", filename: "cover.png", mime: "image/jpeg", data: pngData},
		{name: "extension spoof", kind: "cover", filename: "cover.jpg", mime: "image/png", data: pngData},
		{name: "image truncation", kind: "cover", filename: "cover.png", mime: "image/png", data: pngData[:len(pngData)-20]},
		{name: "mp3 truncation", kind: "audio", filename: "word.mp3", mime: "audio/mpeg", data: mp3[:len(mp3)-20]},
		{name: "id3 is not audio", kind: "audio", filename: "word.mp3", mime: "audio/mpeg", data: []byte{'I', 'D', '3', 3, 0, 0, 0, 0, 0, 0}},
		{name: "audio spoof", kind: "audio", filename: "word.mp3", mime: "audio/mpeg", data: pngData},
		{name: "bad kind", kind: "video", filename: "cover.png", data: pngData},
	} {
		t.Run(tc.name, func(t *testing.T) {
			data, mime, _, err := ValidateMedia(tc.kind, tc.filename, tc.mime, bytes.NewReader(tc.data))
			if tc.valid {
				if err != nil || len(data) != len(tc.data) || mime == "" {
					t.Fatalf("valid file rejected: %v", err)
				}
			} else if err == nil {
				t.Fatal("invalid file accepted")
			}
			if tc.large && !errors.Is(err, ErrTooLarge) {
				t.Fatalf("want payload too large: %v", err)
			}
		})
	}
}
