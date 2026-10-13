package contenthtml

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestJSONSanitizesExecutableMarkupAndCSS(t *testing.T) {
	input := []byte(`{"content":"<p style=\"color:#123456;position:fixed;background-image:url(https://evil.example/x)\" onclick=\"alert(1)\">Halo <a href=\"javascript:alert(1)\">tautan</a><script>alert(2)</script></p>","tips":"Aman"}`)
	clean, err := JSON(input)
	if err != nil {
		t.Fatal(err)
	}
	var result struct {
		Content string `json:"content"`
		Tips    string `json:"tips"`
	}
	if err := json.Unmarshal(clean, &result); err != nil {
		t.Fatal(err)
	}
	for _, unsafe := range []string{"onclick", "javascript:", "<script", "position:", "background-image", "alert("} {
		if strings.Contains(strings.ToLower(result.Content), unsafe) {
			t.Fatalf("unsafe fragment %q survived: %s", unsafe, result.Content)
		}
	}
	if !strings.Contains(result.Content, "color") || !strings.Contains(result.Content, "Halo") || result.Tips != "Aman" {
		t.Fatalf("safe content lost: %s", clean)
	}
}

func TestJSONKeepsLegacyAudioAndEscapesMaliciousURL(t *testing.T) {
	input := []byte(`{"content":"<div data-audio=\"hiragana/a\"><button data-play-audio>Putar</button></div><img src=\"data:text/html,evil\">"}`)
	clean, err := JSON(input)
	if err != nil {
		t.Fatal(err)
	}
	var result map[string]string
	if err := json.Unmarshal(clean, &result); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result["content"], "data-audio") || !strings.Contains(result["content"], "data-play-audio") {
		t.Fatalf("audio control lost: %s", result["content"])
	}
	if strings.Contains(result["content"], "data:text/html") {
		t.Fatalf("unsafe image URL survived: %s", result["content"])
	}
}
