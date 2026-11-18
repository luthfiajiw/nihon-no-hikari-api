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

func TestRubyElementsKeepSafeMarkupAndStyles(t *testing.T) {
	input := `<p><ruby style="ruby-align: center; ruby-position: over; background-image: url(https://evil.example/x)" onclick="alert(1)"><rb>誕生日</rb><rp>(</rp><rt style="color: #8F7365; font-size: 13px; font-weight: 600" onmouseover="alert(2)">たんじょうび</rt><rp>)</rp><rtc style="ruby-align: space-around">tanjobi</rtc><rtc>plain</rtc></ruby></p>`
	clean := HTML(input)
	for _, safe := range []string{
		`<ruby style="ruby-align: center; ruby-position: over">`,
		`<rb>誕生日</rb>`,
		`<rp>(</rp>`,
		`<rt style="color: #8F7365; font-size: 13px; font-weight: 600">たんじょうび</rt>`,
		`<rp>)</rp>`,
		`<rtc style="ruby-align: space-around">tanjobi</rtc>`,
		`<rtc>plain</rtc>`,
		`</ruby>`,
	} {
		if !strings.Contains(clean, safe) {
			t.Errorf("ruby markup lost %q: %s", safe, clean)
		}
	}
	for _, unsafe := range []string{"onclick", "onmouseover", "alert(", "evil.example", "background-image"} {
		if strings.Contains(strings.ToLower(clean), unsafe) {
			t.Errorf("unsafe ruby fragment %q survived: %s", unsafe, clean)
		}
	}
	if HTML(clean) != clean {
		t.Errorf("ruby sanitization is not idempotent: %s", clean)
	}
}

func TestGridTemplateColumnsAllowsSafeLayoutsAndRejectsUnsafeCSS(t *testing.T) {
	safeCases := []struct {
		name string
		html string
		want string
	}{
		{
			name: "div repeat auto-fit px",
			html: `<div style="display: grid; grid-template-columns: repeat(auto-fit, minmax(220px, 1fr))">a</div>`,
			want: `grid-template-columns: repeat(auto-fit, minmax(220px, 1fr))`,
		},
		{
			name: "div repeat 2 1fr",
			html: `<div style="display: grid; grid-template-columns: repeat(2, 1fr); gap: 1rem; margin-top: 1rem">a</div>`,
			want: `grid-template-columns: repeat(2, 1fr)`,
		},
		{
			name: "div repeat fixed count",
			html: `<div style="display: grid; grid-template-columns: repeat(4, 1fr)">a</div>`,
			want: `grid-template-columns: repeat(4, 1fr)`,
		},
		{
			name: "div explicit fr and px tracks",
			html: `<div style="display: grid; grid-template-columns: 200px 1fr 2fr">a</div>`,
			want: `grid-template-columns: 200px 1fr 2fr`,
		},
		{
			name: "div repeat auto-fill rem and min",
			html: `<div style="display: grid; grid-template-columns: repeat(auto-fill, minmax(min(14rem, 100%), 1fr))">a</div>`,
			want: `grid-template-columns: repeat(auto-fill, minmax(min(14rem, 100%), 1fr))`,
		},
		{
			name: "section grid-template-columns",
			html: `<section style="display: grid; grid-template-columns: repeat(2, minmax(0, 1fr))">a</section>`,
			want: `grid-template-columns: repeat(2, minmax(0, 1fr))`,
		},
		{
			name: "full ruby card grid snippet",
			html: `<section style="background-color: #FFF; border: 1px solid #0c0a09; border-radius: 12px; padding: 1.75rem; margin-bottom: 24px"><h2 style="color: #332D29; font-size: 22px; font-weight: 700; line-height: 1.3; margin: 0">Kosakata Baru</h2><div style="display: grid; grid-template-columns: repeat(2, 1fr); gap: 1rem; margin-top: 1rem"><div style="background-color: #F5F3F2; border: 1px solid #E5DFDA; border-radius: 8px; padding: 1.25rem 1rem; text-align: center"><div style="color: #332D29; font-size: 32px; font-weight: 700; line-height: 1.4; margin-bottom: 0.35rem"><ruby style="ruby-align: center">誕生日<rt style="color: #8F7365; font-size: 16px; font-weight: 600">たんじょうび</rt></ruby></div></div></div></section>`,
			want: `grid-template-columns: repeat(2, 1fr)`,
		},
	}
	for _, tc := range safeCases {
		t.Run(tc.name, func(t *testing.T) {
			clean := HTML(tc.html)
			if !strings.Contains(clean, tc.want) {
				t.Errorf("safe grid-template-columns lost %q: %s", tc.want, clean)
			}
		})
	}

	unsafeCases := []string{
		`<div style="grid-template-columns: expression(alert(1))">a</div>`,
		`<div style="grid-template-columns: url(https://evil.example/x)">a</div>`,
		`<div style="grid-template-columns: repeat(1000, 1fr)">a</div>`,
	}
	for _, input := range unsafeCases {
		clean := HTML(input)
		if strings.Contains(strings.ToLower(clean), "grid-template-columns") {
			t.Errorf("unsafe grid-template-columns survived: %s", clean)
		}
	}
}

func TestFlexLayoutAllowsSafeStylesAndRejectsUnsafeCSS(t *testing.T) {
	input := `<section style="background-color: #FFF; border: 1px solid #0c0a09; border-radius: 12px; padding: 1.75rem; margin-bottom: 24px">
  <h2 style="color: #332D29; font-size: 22px; font-weight: 700; line-height: 1.3; margin: 0">Kosakata Baru</h2>
  <div style="display: flex; flex-wrap: wrap; gap: 1rem; margin-top: 1rem; justify-content: center; align-items: stretch">
    <div style="box-sizing: border-box; flex: 1 1 45%; min-width: 200px; background-color: #F5F3F2; border: 1px solid #E5DFDA; border-radius: 8px; padding: 1.25rem 1rem; text-align: center">
      <div style="color: #332D29; font-size: 32px; font-weight: 700; line-height: 1.4; margin-bottom: 0.35rem">
        <ruby style="ruby-align: center">誕生日<rt style="color: #8F7365; font-size: 16px; font-weight: 600">たんじょうび</rt></ruby>
      </div>
      <div style="color: #615752; font-size: 14px; font-style: italic; line-height: 1.4; margin-bottom: 0.25rem">tanjoubi</div>
      <div style="color: #2B2521; font-size: 16px; font-weight: 700; line-height: 1.4">Ulang tahun</div>
    </div>
  </div>
</section>`

	clean := HTML(input)
	for _, want := range []string{
		"display: flex",
		"flex-wrap: wrap",
		"gap: 1rem",
		"justify-content: center",
		"align-items: stretch",
		"box-sizing: border-box",
		"flex: 1 1 45%",
		"min-width: 200px",
		"border: 1px solid #e5dfda",
		"ruby-align: center",
	} {
		if !strings.Contains(strings.ToLower(clean), want) {
			t.Errorf("flex layout lost %q: %s", want, clean)
		}
	}
	if HTML(clean) != clean {
		t.Errorf("flex sanitization is not idempotent: %s", clean)
	}

	unsafeCases := []struct {
		html string
		prop string
	}{
		{html: `<div style="flex: expression(alert(1))">a</div>`, prop: "flex"},
		{html: `<div style="flex: url(https://evil.example/x)">a</div>`, prop: "flex"},
		{html: `<div style="min-width: calc(100vw + 1px); max-width: url(https://evil.example/x)">a</div>`, prop: "min-width"},
		{html: `<div style="flex-direction: url(https://evil.example/x); box-sizing: inherit">a</div>`, prop: "flex-direction"},
	}
	for _, tc := range unsafeCases {
		out := HTML(tc.html)
		if strings.Contains(strings.ToLower(out), tc.prop) || strings.Contains(strings.ToLower(out), "evil.example") || strings.Contains(strings.ToLower(out), "alert(") {
			t.Errorf("unsafe %s survived: %s", tc.prop, out)
		}
	}
}

func TestFormulaCardComponentKeepsSafeStylesAndRejectsUnsafeCSS(t *testing.T) {
	input := `<div style="background-color: #FAF8F6; padding: 2rem 1.5rem; border-radius: 12px; font-family: system-ui, -apple-system, sans-serif; text-align: center; max-width: 640px; margin: 0 auto;">
  <div style="margin-bottom: 1.75rem;">
    <p style="color: #2B2521; font-size: 40px; font-weight: 700; letter-spacing: 0.04em; margin: 0 0 0.5rem 0; line-height: 1.2;">
      8月17日
    </p>
    <span style="display: inline-block; background-color: #EFEBE8; color: #8F7365; font-size: 15px; font-weight: 600; padding: 6px 20px; border-radius: 999px;">
      はちがつ じゅうしちにち
    </span>
  </div>
  <div style="background-color: #EFEBE8; border: 1px solid #D4C9C2; border-radius: 12px; padding: 1.125rem 1.25rem; display: flex; align-items: center; justify-content: space-between; gap: 10px; flex-wrap: wrap;">
    <div style="flex: 1; min-width: 125px; background-color: #FAF8F6; border: 1px solid #E3DCD7; border-radius: 10px; padding: 0.875rem 0.75rem; text-align: center;">
      <span style="display: block; color: #615752; font-size: 12px; margin-bottom: 4px;">Bulan 8 (Agustus)</span>
      <strong style="display: block; color: #2B2521; font-size: 18px; margin-bottom: 2px;">8月</strong>
      <span style="display: block; color: #8F7365; font-size: 12px;">はちがつ</span>
    </div>
    <span style="color: #615752; font-size: 20px; font-weight: 700; padding: 0 2px;">+</span>
    <div style="flex: 1; min-width: 125px; background-color: #72655E; border: 1px solid #615752; border-radius: 10px; padding: 0.875rem 0.75rem; text-align: center;">
      <span style="display: block; color: #EFEBE8; font-size: 12px; margin-bottom: 4px;">Hasil Baku</span>
      <strong style="display: block; color: #FAF8F6; font-size: 18px; margin-bottom: 2px;">8月17日</strong>
      <span style="display: block; color: #D4C9C2; font-size: 12px;">17 Agustus</span>
    </div>
  </div>
</div>`

	clean := HTML(input)
	for _, want := range []string{
		"font-family: system-ui, -apple-system, sans-serif",
		"max-width: 640px",
		"margin: 0 auto",
		"letter-spacing: 0.04em",
		"margin: 0 0 0.5rem 0",
		"display: inline-block",
		"padding: 6px 20px",
		"border-radius: 999px",
		"padding: 1.125rem 1.25rem",
		"padding: 0.875rem 0.75rem",
		"display: block",
		"margin-bottom: 4px",
		`<strong style="display: block; color: #2b2521; font-size: 18px; margin-bottom: 2px"`,
		`<strong style="display: block; color: #faf8f6; font-size: 18px; margin-bottom: 2px"`,
		"padding: 0 2px",
	} {
		if !strings.Contains(strings.ToLower(clean), want) {
			t.Errorf("formula card lost %q: %s", want, clean)
		}
	}
	if HTML(clean) != clean {
		t.Errorf("formula card sanitization is not idempotent: %s", clean)
	}

	unsafeCases := []struct {
		html string
		prop string
	}{
		{html: `<div style="font-family: url(https://evil.example/font.woff2)">a</div>`, prop: "font-family"},
		{html: `<p style="letter-spacing: expression(alert(1))">a</p>`, prop: "letter-spacing"},
		{html: `<strong style="margin: url(https://evil.example/x); position: fixed">a</strong>`, prop: "margin"},
	}
	for _, tc := range unsafeCases {
		out := HTML(tc.html)
		if strings.Contains(strings.ToLower(out), tc.prop) || strings.Contains(strings.ToLower(out), "evil.example") || strings.Contains(strings.ToLower(out), "alert(") {
			t.Errorf("unsafe %s survived: %s", tc.prop, out)
		}
	}
}

func TestCalcFunctionAllowsSafeArithmeticAndRejectsUnsafeCSS(t *testing.T) {
	safeCases := []struct {
		name string
		html string
		want string
	}{
		{
			name: "flex basis calc",
			html: `<div style="flex: 1 1 calc(50% - 1rem); min-width: calc(200px + 1rem)">a</div>`,
			want: `flex: 1 1 calc(50% - 1rem); min-width: calc(200px + 1rem)`,
		},
		{
			name: "width and max-width calc with parens and division",
			html: `<div style="width: calc((100% - 2rem) / 3); max-width: calc(100% - 32px)">a</div>`,
			want: `width: calc((100% - 2rem) / 3); max-width: calc(100% - 32px)`,
		},
		{
			name: "padding margin and gap calc",
			html: `<section style="padding: calc(1rem + 4px) 1.5rem; margin: 0 auto calc(1rem * 2); gap: calc(0.5rem + 2px)">a</section>`,
			want: `padding: calc(1rem + 4px) 1.5rem; margin: 0 auto calc(1rem * 2); gap: calc(0.5rem + 2px)`,
		},
		{
			name: "grid-template-columns calc",
			html: `<div style="display: grid; grid-template-columns: calc(100% - 200px) 200px">a</div>`,
			want: `grid-template-columns: calc(100% - 200px) 200px`,
		},
	}

	for _, tc := range safeCases {
		t.Run(tc.name, func(t *testing.T) {
			clean := HTML(tc.html)
			if !strings.Contains(clean, tc.want) {
				t.Errorf("safe calc() lost %q: %s", tc.want, clean)
			}
			if HTML(clean) != clean {
				t.Errorf("calc() sanitization is not idempotent: %s", clean)
			}
		})
	}

	unsafeCases := []string{
		`<div style="width: calc(expression(alert(1)))">a</div>`,
		`<div style="width: calc(url(https://evil.example/x))">a</div>`,
		`<div style="width: calc(var(--secret) + 10px)">a</div>`,
		`<div style="width: calc(100vw - 10px)">a</div>`,
	}
	for _, input := range unsafeCases {
		clean := HTML(input)
		if strings.Contains(strings.ToLower(clean), "calc(") || strings.Contains(strings.ToLower(clean), "evil.example") || strings.Contains(strings.ToLower(clean), "alert(") {
			t.Errorf("unsafe calc() survived: %s", clean)
		}
	}
}
