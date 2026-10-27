package question

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"nihon-no-hikari-api/pkg/utils"
)

func TestWritingSnapshotKeepsOriginalGuide(t *testing.T) {
	guide := loadTestGuides(t)[0]
	before, _ := json.Marshal(guide)
	raw, err := encodeWritingSnapshot(Question{QuestionType: QuestionTypeStrokeWriting, WritingGuide: &guide})
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != string(before) {
		t.Fatal("snapshot differs from returned guide")
	}
	guide.Character, guide.Romaji = "カ", "ka"
	guide.Strokes = loadTestGuides(t)[1].Strokes
	snapshot, err := decodeWritingSnapshot(raw)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Character != "あ" || snapshot.Romaji != "a" {
		t.Fatal("snapshot changed with template")
	}
	input, _ := json.Marshal(referenceInput(*snapshot))
	correct, _, err := gradeStrokeWriting(*snapshot, true, input)
	if err != nil || !correct {
		t.Fatalf("snapshot grade: %v %v", correct, err)
	}
	if raw, err := encodeWritingSnapshot(Question{QuestionType: QuestionTypeMultipleChoice}); err != nil || raw != nil {
		t.Fatal("multiple choice must not have snapshot")
	}
}

func TestRejectMissingOrCorruptWritingSnapshot(t *testing.T) {
	for _, raw := range []string{"", "null", "{}", "{", `{"character":"あ","romaji":"a","view_box":109,"strokes":[]}`} {
		if _, err := decodeWritingSnapshot([]byte(raw)); !errors.Is(err, utils.ErrInvalidQuestionConfiguration) {
			t.Errorf("accepted %q: %v", raw, err)
		}
	}
	guide := loadTestGuides(t)[0]
	valid, _ := json.Marshal(guide)
	corrupt := strings.Replace(string(valid), `"x":31.01`, `"x":null`, 1)
	if corrupt == string(valid) {
		t.Fatal("test must corrupt an actual coordinate")
	}
	if _, err := decodeWritingSnapshot([]byte(corrupt)); !errors.Is(err, utils.ErrInvalidQuestionConfiguration) {
		t.Fatal("accepted null coordinate")
	}
	guide.Strokes[0].Points = nil
	raw, _ := json.Marshal(guide)
	if _, err := decodeWritingSnapshot(raw); !errors.Is(err, utils.ErrInvalidQuestionConfiguration) {
		t.Fatal("accepted missing reference points")
	}
}
