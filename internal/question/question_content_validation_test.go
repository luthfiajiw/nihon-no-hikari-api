package question

import (
	"encoding/json"
	"testing"

	"github.com/google/uuid"
)

func TestParseSetChecksReadingAnswerAndPublication(t *testing.T) {
	lessonID := uuid.New()
	input := SetInput{Kind: "practice", LessonID: &lessonID, Title: "Membaca", Skill: "reading", PassingScore: 80, Questions: []QuestionInput{{QuestionType: "multiple_choice", PromptText: "Baca huruf ini", StimulusText: stringPtr("あ"), Points: 1, Options: []OptionInput{{Label: "a", IsCorrect: true}, {Label: "i", IsCorrect: false}}}}}
	raw, _ := json.Marshal(input)
	if _, err := parseSet(raw, true); err != nil {
		t.Fatalf("valid reading set rejected: %v", err)
	}
	input.Questions[0].Options[1].IsCorrect = true
	raw, _ = json.Marshal(input)
	if _, err := parseSet(raw, true); err == nil {
		t.Fatal("two correct options must be rejected")
	}
	input.Questions = nil
	raw, _ = json.Marshal(input)
	if _, err := parseSet(raw, false); err != nil {
		t.Fatalf("empty draft rejected: %v", err)
	}
	if _, err := parseSet(raw, true); err == nil {
		t.Fatal("empty published set must be rejected")
	}
	input.Questions = []QuestionInput{{QuestionType: "multiple_choice", Points: 1}}
	raw, _ = json.Marshal(input)
	if _, err := parseSet(raw, false); err != nil {
		t.Fatalf("incomplete question draft rejected: %v", err)
	}
	if _, err := parseSet(raw, true); err == nil {
		t.Fatal("incomplete question must be rejected at publication")
	}
}

func TestParseSetRequiresWritingTemplate(t *testing.T) {
	moduleID := uuid.New()
	input := SetInput{Kind: "final_exam", ModuleID: &moduleID, Title: "Menulis", Skill: "writing", PassingScore: 80, Questions: []QuestionInput{{QuestionType: "stroke_writing", PromptText: "Tulis あ", Points: 1}}}
	raw, _ := json.Marshal(input)
	if _, err := parseSet(raw, true); err == nil {
		t.Fatal("writing question without template must be rejected")
	}
	templateID := uuid.New()
	input.Questions[0].StrokeTemplateID = &templateID
	raw, _ = json.Marshal(input)
	if _, err := parseSet(raw, true); err != nil {
		t.Fatalf("valid writing set rejected: %v", err)
	}
}

func stringPtr(value string) *string { return &value }
