package question

import (
	"encoding/json"
	"os"
	"testing"
)

func loadTestGuides(t *testing.T) []WritingGuide {
	t.Helper()
	data, err := os.ReadFile("../../pkg/db/stroke_sources/hiragana_a_guides.json")
	if err != nil {
		t.Fatal(err)
	}
	var guides []WritingGuide
	if err := json.Unmarshal(data, &guides); err != nil {
		t.Fatal(err)
	}
	if len(guides) != 5 {
		t.Fatalf("got %d guides", len(guides))
	}
	return guides
}

func referenceInput(guide WritingGuide) [][]StrokePoint {
	strokes := make([][]StrokePoint, len(guide.Strokes))
	for index, stroke := range guide.Strokes {
		strokes[index] = append([]StrokePoint(nil), stroke.Points...)
	}
	return strokes
}

func testEvaluate(t *testing.T, guide WritingGuide, strokes [][]StrokePoint) (bool, float64, error) {
	t.Helper()
	data, err := json.Marshal(strokes)
	if err != nil {
		t.Fatal(err)
	}
	return EvaluateStrokeWriting(guide, data)
}

func TestEvaluateStrokeWritingForAllFiveKana(t *testing.T) {
	for _, guide := range loadTestGuides(t) {
		t.Run(guide.Character, func(t *testing.T) {
			valid, accuracy, err := testEvaluate(t, guide, referenceInput(guide))
			if err != nil || !valid || accuracy != 100 {
				t.Fatalf("reference: valid=%v accuracy=%v err=%v", valid, accuracy, err)
			}

			reversed := referenceInput(guide)
			for left, right := 0, len(reversed)-1; left < right; left, right = left+1, right-1 {
				reversed[left], reversed[right] = reversed[right], reversed[left]
			}
			if len(reversed) > 1 {
				valid, _, err = testEvaluate(t, guide, reversed)
				if err != nil || valid {
					t.Fatalf("wrong order: valid=%v err=%v", valid, err)
				}
			}

			wrongDirection := referenceInput(guide)
			for left, right := 0, len(wrongDirection[0])-1; left < right; left, right = left+1, right-1 {
				wrongDirection[0][left], wrongDirection[0][right] = wrongDirection[0][right], wrongDirection[0][left]
			}
			valid, _, err = testEvaluate(t, guide, wrongDirection)
			if err != nil || valid {
				t.Fatalf("wrong direction: valid=%v err=%v", valid, err)
			}

			offset := referenceInput(guide)
			for index := range offset[0] {
				offset[0][index].X += 12
			}
			valid, _, err = testEvaluate(t, guide, offset)
			if err != nil || valid {
				t.Fatalf("offset path: valid=%v err=%v", valid, err)
			}

			missing := referenceInput(guide)[:len(guide.Strokes)-1]
			valid, _, err = testEvaluate(t, guide, missing)
			if err != nil || valid {
				t.Fatalf("missing stroke: valid=%v err=%v", valid, err)
			}

			extra := append(referenceInput(guide), guide.Strokes[0].Points)
			valid, _, err = testEvaluate(t, guide, extra)
			if err != nil || valid {
				t.Fatalf("extra stroke: valid=%v err=%v", valid, err)
			}
		})
	}
}

func TestEvaluateStrokeWritingRejectsMalformedInput(t *testing.T) {
	guide := loadTestGuides(t)[0]
	for _, raw := range []string{`{"x":1}`, `[[{"x":"bad","y":2}]]`, `[[{"x":1,"y":2,"unexpected":true}]]`} {
		_, _, err := EvaluateStrokeWriting(guide, json.RawMessage(raw))
		if err == nil {
			t.Fatalf("expected validation error for %s", raw)
		}
	}
	short := referenceInput(guide)
	short[0] = short[0][:1]
	if _, _, err := testEvaluate(t, guide, short); err == nil {
		t.Fatal("expected too few points error")
	}
	outOfBounds := referenceInput(guide)
	outOfBounds[0][0].X = 999
	if _, _, err := testEvaluate(t, guide, outOfBounds); err == nil {
		t.Fatal("expected bounds error")
	}
}
