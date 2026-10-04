package question

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"
)

func TestSubmitAttemptRequestValidate(t *testing.T) {
	questionID := uuid.New()
	otherQuestionID := uuid.New()
	optionID := uuid.New()
	answerText := "あ"
	emptyText := "   "
	longText := strings.Repeat("a", 256)

	tests := []struct {
		name    string
		request SubmitAttemptRequest
		wantErr bool
	}{
		{name: "option answer", request: SubmitAttemptRequest{Answers: []SubmitAnswerRequest{{QuestionID: questionID, SelectedOptionID: &optionID}}}},
		{name: "text answer", request: SubmitAttemptRequest{Answers: []SubmitAnswerRequest{{QuestionID: questionID, AnswerText: &answerText}}}},
		{name: "missing answers", request: SubmitAttemptRequest{}, wantErr: true},
		{name: "missing question id", request: SubmitAttemptRequest{Answers: []SubmitAnswerRequest{{AnswerText: &answerText}}}, wantErr: true},
		{name: "duplicate question", request: SubmitAttemptRequest{Answers: []SubmitAnswerRequest{{QuestionID: questionID, AnswerText: &answerText}, {QuestionID: questionID, SelectedOptionID: &optionID}}}, wantErr: true},
		{name: "both answer forms", request: SubmitAttemptRequest{Answers: []SubmitAnswerRequest{{QuestionID: questionID, SelectedOptionID: &optionID, AnswerText: &answerText}}}, wantErr: true},
		{name: "empty answer", request: SubmitAttemptRequest{Answers: []SubmitAnswerRequest{{QuestionID: questionID, AnswerText: &emptyText}}}, wantErr: true},
		{name: "too long", request: SubmitAttemptRequest{Answers: []SubmitAnswerRequest{{QuestionID: otherQuestionID, AnswerText: &longText}}}, wantErr: true},
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

func TestSkillIsSupported(t *testing.T) {
	if !SkillReading.IsSupported() || !SkillWriting.IsSupported() {
		t.Fatal("reading and writing should be supported")
	}
	if Skill("listening").IsSupported() {
		t.Fatal("listening should not be supported for lesson question sets")
	}
}

func TestGradeAnswerNormalizesJapaneseText(t *testing.T) {
	correctAnswer := "が"
	decomposedAnswer := "か\u3099"
	item := gradingQuestion{CorrectAnswer: &correctAnswer}
	answer := SubmitAnswerRequest{AnswerText: &decomposedAnswer}

	isCorrect, err := gradeAnswer(context.Background(), nil, item, answer)
	if err != nil {
		t.Fatalf("gradeAnswer() error = %v", err)
	}
	if !isCorrect {
		t.Fatal("canonically equivalent Japanese answer should be correct")
	}
}
