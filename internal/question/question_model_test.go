package question

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"nihon-no-hikari-api/pkg/utils"

	"github.com/google/uuid"
)

func TestSubmitAttemptRequestValidate(t *testing.T) {
	questionID := uuid.New()
	otherQuestionID := uuid.New()
	optionID := uuid.New()
	strokeInput := json.RawMessage(`[{"x":10,"y":20,"t":0}]`)
	invalidStrokeInput := json.RawMessage(`{"x":`)
	nullStrokeInput := json.RawMessage(`null`)
	largeStrokeInput := json.RawMessage(`"` + strings.Repeat("a", 64*1024) + `"`)

	tests := []struct {
		name    string
		request SubmitAttemptRequest
		wantErr bool
	}{
		{name: "option answer", request: SubmitAttemptRequest{Answers: []SubmitAnswerRequest{{QuestionID: questionID, SelectedOptionID: &optionID}}}},
		{name: "stroke answer", request: SubmitAttemptRequest{Answers: []SubmitAnswerRequest{{QuestionID: questionID, StrokeInput: &strokeInput}}}},
		{name: "missing answers", request: SubmitAttemptRequest{}, wantErr: true},
		{name: "missing question id", request: SubmitAttemptRequest{Answers: []SubmitAnswerRequest{{StrokeInput: &strokeInput}}}, wantErr: true},
		{name: "duplicate question", request: SubmitAttemptRequest{Answers: []SubmitAnswerRequest{{QuestionID: questionID, StrokeInput: &strokeInput}, {QuestionID: questionID, SelectedOptionID: &optionID}}}, wantErr: true},
		{name: "both answer forms", request: SubmitAttemptRequest{Answers: []SubmitAnswerRequest{{QuestionID: questionID, SelectedOptionID: &optionID, StrokeInput: &strokeInput}}}, wantErr: true},
		{name: "empty answer", request: SubmitAttemptRequest{Answers: []SubmitAnswerRequest{{QuestionID: questionID}}}, wantErr: true},
		{name: "null stroke", request: SubmitAttemptRequest{Answers: []SubmitAnswerRequest{{QuestionID: questionID, StrokeInput: &nullStrokeInput}}}, wantErr: true},
		{name: "invalid stroke JSON", request: SubmitAttemptRequest{Answers: []SubmitAnswerRequest{{QuestionID: questionID, StrokeInput: &invalidStrokeInput}}}, wantErr: true},
		{name: "stroke too large", request: SubmitAttemptRequest{Answers: []SubmitAnswerRequest{{QuestionID: otherQuestionID, StrokeInput: &largeStrokeInput}}}, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.request.Validate()
			if (err != nil) != tt.wantErr {
				t.Fatalf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestSubmitAttemptRequestAllowsNullStrokeForOptionAnswer(t *testing.T) {
	requestJSON := `{"answers":[{"question_id":"` + uuid.NewString() + `","selected_option_id":"` + uuid.NewString() + `","stroke_input":null}]}`

	var request SubmitAttemptRequest
	if err := json.Unmarshal([]byte(requestJSON), &request); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if request.Answers[0].StrokeInput != nil {
		t.Fatal("StrokeInput should be nil when JSON value is null")
	}
	if err := request.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestSkillIsSupported(t *testing.T) {
	if !SkillReading.IsSupported() || !SkillWriting.IsSupported() {
		t.Fatal("reading and writing should be supported")
	}
	if Skill("listening").IsSupported() {
		t.Fatal("listening should not be supported for lesson question sets")
	}
}

func TestGradeAnswerRejectsStrokeWithoutServerEvaluator(t *testing.T) {
	item := gradingQuestion{QuestionType: QuestionTypeStrokeWriting}
	strokeInput := json.RawMessage(`[{"x":10,"y":20}]`)
	answer := SubmitAnswerRequest{StrokeInput: &strokeInput}

	_, err := gradeAnswer(context.Background(), nil, item, answer)
	if !errors.Is(err, utils.ErrInvalidQuestionConfiguration) {
		t.Fatalf("gradeAnswer() error = %v, want ErrInvalidQuestionConfiguration", err)
	}
}
