package kanastroke

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"reflect"
	"strings"
	"testing"
)

func testSVG(character, code, path string) string {
	return fmt.Sprintf(`<svg xmlns="http://www.w3.org/2000/svg" xmlns:kvg="https://kanjivg.tagaini.net/" viewBox="0 0 109 109"><g id="kvg:StrokePaths_%s"><g kvg:element="%s"><path id="kvg:%s-s1" d="%s"/></g></g><g id="kvg:StrokeNumbers_%s"><text transform="translate(1,2)">1</text></g></svg>`, code, character, code, path, code)
}

func TestConvertMatchesAllSeedGuides(t *testing.T) {
	raw, err := os.ReadFile("../db/stroke_sources/hiragana_a_guides.json")
	if err != nil {
		t.Fatal(err)
	}
	var guides []struct {
		Code, Character, Romaji string
		Data
	}
	if err := json.Unmarshal(raw, &guides); err != nil {
		t.Fatal(err)
	}
	if len(guides) != 5 {
		t.Fatalf("got %d guides", len(guides))
	}
	for _, guide := range guides {
		t.Run(guide.Character, func(t *testing.T) {
			svg, err := os.ReadFile("../db/stroke_sources/" + guide.Code + ".svg")
			if err != nil {
				t.Fatal(err)
			}
			data, err := Convert(string(svg), "hiragana", guide.Character)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(data, guide.Data) {
				t.Fatal("conversion differs from existing seed geometry")
			}
		})
	}
}

func TestCubicCommandsAndKatakana(t *testing.T) {
	paths := []string{
		"M10,10 c10,0 20,10 30,10 s20,10 30,10",
		"m10,10 C20,10 30,20 40,20 S60,30 70,30",
		"M10 10 C20 10 30 20 40 20 50 20 60 30 70 30",
		"M1e1,+10 c10 0 20 10 30 10 s20 10 30 10",
	}
	var reference []Point
	for _, path := range paths {
		data, err := Convert(testSVG("カ", "030ab", path), "katakana", "カ")
		if err != nil {
			t.Fatalf("%s: %v", path, err)
		}
		if len(data.Strokes) != 1 || len(data.Strokes[0].Points) != 32 {
			t.Fatal("wrong shape")
		}
		if reference == nil {
			reference = data.Strokes[0].Points
		}
		if !reflect.DeepEqual(data.Strokes[0].Points, reference) {
			t.Fatal("absolute/relative/smooth conversion differs")
		}
	}
	// A smooth curve without a preceding cubic reflects the start point itself.
	if _, err := Convert(testSVG("カ", "030ab", "M10 10 S30 20 40 20"), "katakana", "カ"); err != nil {
		t.Fatal(err)
	}
}

func TestRejectInvalidSVG(t *testing.T) {
	valid := testSVG("あ", "03042", "M10 10 c10 0 20 10 30 10")
	for name, svg := range map[string]string{
		"empty": "", "bad XML": "<svg>", "wrong root": "<html/>",
		"wrong viewbox":       strings.Replace(valid, "0 0 109 109", "0 0 100 100", 1),
		"wrong identity":      strings.ReplaceAll(valid, "03042", "03044"),
		"wrong order":         strings.Replace(valid, "-s1", "-s2", 1),
		"missing strokes":     strings.Replace(valid, `<path id="kvg:03042-s1" d="M10 10 c10 0 20 10 30 10"/>`, "", 1),
		"transform":           strings.Replace(valid, `<g id="kvg:StrokePaths_03042"`, `<g transform="translate(1,2)" id="kvg:StrokePaths_03042"`, 1),
		"ancestor transform":  strings.Replace(valid, "viewBox=", `transform="translate(1,2)" viewBox=`, 1),
		"unsupported element": strings.Replace(valid, "<path ", "<use ", 1),
		"external entity":     strings.Replace(valid, "M10 10", "&external;", 1),
		"second root":         valid + valid,
		"trailing text":       valid + "trailing",
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := Convert(svg, "hiragana", "あ"); err == nil {
				t.Fatal("invalid SVG accepted")
			}
		})
	}
	for _, path := range []string{"", "M0 0", "M0 0 c0 0 0 0 0 0", "M10 10 L30 30", "M10 10 C10 20 30", "M10 10 C10 10 10 10 20 20 C", "M10 10 c10 0 20 10 30 10 m1 1", "M10 10 c1e400 0 20 10 30 10", "M-1 0 c0 0 0 1 0 2"} {
		if _, err := Convert(testSVG("あ", "03042", path), "hiragana", "あ"); err == nil {
			t.Errorf("invalid path accepted: %q", path)
		}
	}
	if _, err := Convert(strings.Repeat(" ", MaxSVGBytes+1), "hiragana", "あ"); !errors.Is(err, ErrTooLarge) {
		t.Fatalf("size limit: %v", err)
	}
	for _, input := range [][2]string{{"katakana", "あ"}, {"hiragana", "カ"}, {"kanji", "日"}, {"hiragana", "あい"}, {"hiragana", "\u3099"}} {
		if _, err := Convert(valid, input[0], input[1]); err == nil {
			t.Errorf("accepted %v", input)
		}
	}
}
