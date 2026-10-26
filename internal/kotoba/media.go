package kotoba

import (
	"bytes"
	"encoding/binary"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"mime"
	"path/filepath"
	"strings"

	"github.com/hajimehoshi/go-mp3"
)

// ValidateMedia bounds reads independently of multipart headers. Image decoding
// is also bounded by pixel count to avoid compressed image decompression bombs.
func ValidateMedia(kind, name, declaredType string, source io.Reader) ([]byte, string, string, error) {
	limit := MaxCoverBytes
	if kind == "audio" {
		limit = MaxAudioBytes
	} else if kind != "cover" {
		return nil, "", "", FieldErrors{"kind": "Pilih cover atau audio."}
	}
	data, err := io.ReadAll(io.LimitReader(source, int64(limit)+1))
	if err != nil {
		return nil, "", "", FieldErrors{"file": "File tidak dapat dibaca."}
	}
	if len(data) > limit {
		return nil, "", "", ErrTooLarge
	}
	invalid := func() ([]byte, string, string, error) {
		return nil, "", "", FieldErrors{"file": "File kosong, rusak, atau formatnya tidak sesuai."}
	}
	if len(data) == 0 {
		return invalid()
	}
	ext := strings.ToLower(filepath.Ext(name))
	contentType, _, err := mime.ParseMediaType(declaredType)
	if declaredType != "" && err != nil {
		return invalid()
	}
	if kind == "cover" {
		cfg, format, err := image.DecodeConfig(bytes.NewReader(data))
		if err != nil || cfg.Width <= 0 || cfg.Height <= 0 || int64(cfg.Width)*int64(cfg.Height) > 16*1024*1024 {
			return invalid()
		}
		expected := "image/" + format
		if (format != "png" && format != "jpeg") || (format == "png" && ext != ".png") || (format == "jpeg" && ext != ".jpg" && ext != ".jpeg") {
			return invalid()
		}
		if contentType != "" && contentType != expected {
			return invalid()
		}
		if _, _, err := image.Decode(bytes.NewReader(data)); err != nil {
			return invalid()
		}
		if format == "jpeg" {
			ext = ".jpg"
		}
		return data, expected, ext, nil
	}
	if ext != ".mp3" || (contentType != "" && contentType != "audio/mpeg" && contentType != "audio/mp3") || !validMP3(data) {
		return invalid()
	}
	if !decodableMP3(data) {
		return invalid()
	}
	return data, "audio/mpeg", ".mp3", nil
}

// Reject malformed compressed bitstreams even if a codec encounters a panic.
// Keep this guard local to the decoder so application errors are not swallowed.
func decodableMP3(data []byte) (valid bool) {
	defer func() {
		if recover() != nil {
			valid = false
		}
	}()
	decoder, err := mp3.NewDecoder(bytes.NewReader(data))
	if err != nil {
		return false
	}
	decoded, err := io.Copy(io.Discard, decoder)
	return err == nil && decoded > 0
}

// Validate complete MPEG Layer III frame boundaries, rather than accepting an
// ID3 signature as proof of audio. Free-format frames are intentionally rejected.
func validMP3(data []byte) bool {
	if len(data) >= 10 && string(data[:3]) == "ID3" {
		if data[3] < 2 || data[3] > 4 {
			return false
		}
		size := 0
		for _, b := range data[6:10] {
			if b&0x80 != 0 {
				return false
			}
			size = (size << 7) | int(b)
		}
		size += 10
		if data[3] == 4 && data[5]&0x10 != 0 {
			size += 10
		}
		if size > len(data) {
			return false
		}
		data = data[size:]
	}
	if len(data) >= 128 && string(data[len(data)-128:len(data)-125]) == "TAG" {
		data = data[:len(data)-128]
	}
	frames := 0
	for len(data) > 0 {
		if len(data) < 4 {
			return false
		}
		h := binary.BigEndian.Uint32(data[:4])
		version, layer, bitrate, sample := (h>>19)&3, (h>>17)&3, (h>>12)&15, (h>>10)&3
		if h>>21 != 0x7ff || version == 1 || layer != 1 || bitrate == 0 || bitrate == 15 || sample == 3 || h&3 == 2 {
			return false
		}
		rates := [...]int{44100, 48000, 32000}
		rate := rates[sample]
		bitrates := [...]int{0, 32, 40, 48, 56, 64, 80, 96, 112, 128, 160, 192, 224, 256, 320}
		coefficient := 144000
		if version != 3 {
			bitrates = [...]int{0, 8, 16, 24, 32, 40, 48, 56, 64, 80, 96, 112, 128, 144, 160}
			coefficient = 72000
			if version == 2 {
				rate /= 2
			} else {
				rate /= 4
			}
		}
		length := coefficient*bitrates[bitrate]/rate + int((h>>9)&1)
		sideInfo := 32
		if (h>>6)&3 == 3 {
			sideInfo = 17
		}
		if version != 3 {
			sideInfo /= 2
			if sideInfo == 8 {
				sideInfo = 9
			}
		}
		minimum := 4 + sideInfo
		if h&(1<<16) == 0 {
			minimum += 2
		}
		if length < minimum || length > len(data) {
			return false
		}
		data = data[length:]
		frames++
	}
	return frames > 0
}
