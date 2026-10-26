package contenthtml

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
)

func TestAllHeadingLevelsKeepSafeMarkupAndStyles(t *testing.T) {
	for level := 1; level <= 6; level++ {
		t.Run(fmt.Sprintf("h%d", level), func(t *testing.T) {
			input := fmt.Sprintf(`<h%d style="font-size:20px;color:#123456;margin:0;background-image:url(https://evil.example/x)" onclick="alert(1)">Judul <strong>あい</strong><br>lanjutan</h%d>`, level, level)
			clean := HTML(input)
			for _, safe := range []string{fmt.Sprintf("<h%d ", level), fmt.Sprintf("</h%d>", level), "font-size: 20px", "color: #123456", "margin: 0", "<strong>あい</strong>", "<br>"} {
				if !strings.Contains(clean, safe) {
					t.Errorf("heading lost %q: %s", safe, clean)
				}
			}
			for _, unsafe := range []string{"onclick", "alert(", "evil.example", "background-image"} {
				if strings.Contains(clean, unsafe) {
					t.Errorf("unsafe heading fragment %q survived: %s", unsafe, clean)
				}
			}
			if HTML(clean) != clean {
				t.Errorf("heading sanitization is not idempotent: %s", clean)
			}
		})
	}
}

func TestJSONVocabularyLesson(t *testing.T) {
	html, err := os.ReadFile("testdata/lesson-vocabulary.html")
	if err != nil {
		t.Fatal(err)
	}
	input, err := json.Marshal(map[string]string{"content": string(html), "tips": "Latih あか setiap hari."})
	if err != nil {
		t.Fatal(err)
	}
	clean, err := JSON(input)
	if err != nil {
		t.Fatal(err)
	}
	var result map[string]string
	if err := json.Unmarshal(clean, &result); err != nil {
		t.Fatal(err)
	}
	for _, fragment := range []string{"<section", "display: grid", "grid-template-columns:", "border-radius:", "box-shadow:", "position: relative", "position: absolute", "translateY(-50%)", `data-audio="/audio/vocabulary/aka.mp3"`, `aria-label="Dengarkan aka"`, "あか", "Cumi-cumi"} {
		if !strings.Contains(result["content"], fragment) {
			t.Errorf("vocabulary lesson lost %q", fragment)
		}
	}
	if result["tips"] != "Latih あか setiap hari." {
		t.Fatalf("tips changed: %s", result["tips"])
	}
	// The same policy is used for preview, saving, publishing and reading.
	again, err := JSON(clean)
	if err != nil || string(again) != string(clean) {
		t.Fatalf("repeated sanitization changed the saved lesson: %v", err)
	}
}

func TestCardLayoutsRejectExecutableCSSAndMarkup(t *testing.T) {
	clean := HTML(`<section style="background:url(https://evil.example/image);position:fixed;z-index:999"><div style="grid-template-columns:expression(alert(1));box-shadow:url(https://evil.example/image)"><button type="submit" data-play-audio data-audio="javascript:alert(1)" onclick="alert(1)" style="position:fixed;transform:scale(100);background-image:url(https://evil.example/image)">Putar</button><script>alert(1)</script></div></section>`)
	for _, unsafe := range []string{"evil.example", "fixed", "z-index", "expression", "javascript:", "onclick", "scale(", "background-image", `type="submit"`, "<script", "alert("} {
		if strings.Contains(clean, unsafe) {
			t.Errorf("unsafe card fragment %q survived: %s", unsafe, clean)
		}
	}
	if !strings.Contains(clean, "data-play-audio") || !strings.Contains(clean, "Putar") {
		t.Fatalf("safe audio button lost: %s", clean)
	}
}

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
