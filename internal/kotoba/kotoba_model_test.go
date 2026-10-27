package kotoba

import (
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"
)

func validInput() TopicInput {
	return TopicInput{Title: "Sekolah", Level: "N5", Category: "Dasar", Description: "Kosakata sekolah", OrderIndex: 1, IsActive: true, IsVisible: true, CoverMediaID: uuid.New(), Vocabulary: []VocabularyInput{
		{WordFields: WordFields{Kanji: "学校", Hiragana: "がっこう", Romaji: "gakkou", Translation: "sekolah", Type: "Kata Benda", Example: Example{Japanese: "学校へ行きます。", Indonesian: "Saya pergi ke sekolah."}}},
	}}
}
func TestTopicValidation(t *testing.T) {
	for _, tc := range []struct {
		name, field string
		update      bool
		change      func(*TopicInput)
	}{
		{name: "empty title", field: "title", change: func(v *TopicInput) { v.Title = " " }},
		{name: "long title", field: "title", change: func(v *TopicInput) { v.Title = strings.Repeat("学", 161) }},
		{name: "long description", field: "description", change: func(v *TopicInput) { v.Description = strings.Repeat("a", 201) }},
		{name: "level", field: "level", change: func(v *TopicInput) { v.Level = "N6" }},
		{name: "category", field: "category", change: func(v *TopicInput) { v.Category = "other" }},
		{name: "order", field: "order_index", change: func(v *TopicInput) { v.OrderIndex = 0 }},
		{name: "order overflow", field: "order_index", change: func(v *TopicInput) { v.OrderIndex = 1 << 40 }},
		{name: "cover", field: "cover_media_id", change: func(v *TopicInput) { v.CoverMediaID = uuid.Nil }},
		{name: "empty words", field: "vocabulary", change: func(v *TopicInput) { v.Vocabulary = nil }},
		{name: "word required", field: "vocabulary.0.example.japanese", change: func(v *TopicInput) { v.Vocabulary[0].Example.Japanese = " " }},
		{name: "type", field: "vocabulary.0.type", change: func(v *TopicInput) { v.Vocabulary[0].Type = "other" }},
		{name: "furigana mismatch", field: "vocabulary.0.furigana", change: func(v *TopicInput) { v.Vocabulary[0].Furigana = []Furigana{{Text: "学"}} }},
		{name: "empty segment", field: "vocabulary.0.furigana", change: func(v *TopicInput) { v.Vocabulary[0].Furigana = []Furigana{{Text: "学校"}, {Text: ""}} }},
		{name: "zero audio", field: "vocabulary.0.audio_media_id", change: func(v *TopicInput) { id := uuid.Nil; v.Vocabulary[0].AudioMediaID = &id }},
		{name: "id on create", field: "vocabulary.0.id", change: func(v *TopicInput) { id := uuid.New(); v.Vocabulary[0].ID = &id }},
		{name: "missing version", field: "version", update: true, change: func(v *TopicInput) {}},
		{name: "duplicate ids", field: "vocabulary.1.id", update: true, change: func(v *TopicInput) {
			v.Version = 1
			id := uuid.New()
			v.Vocabulary[0].ID = &id
			v.Vocabulary = append(v.Vocabulary, v.Vocabulary[0])
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			in := validInput()
			tc.change(&in)
			var fields FieldErrors
			if !errors.As(in.Validate(tc.update), &fields) || fields[tc.field] == "" {
				t.Fatalf("expected %s: %v", tc.field, fields)
			}
		})
	}
	in := validInput()
	in.Title = strings.Repeat("学", 160)
	in.Description = strings.Repeat("学", 200)
	if err := in.Validate(false); err != nil {
		t.Fatal(err)
	}
	if len(in.Vocabulary[0].Furigana) != 1 || in.Vocabulary[0].Furigana[0].Reading != "がっこう" {
		t.Fatal("missing whole-word furigana")
	}
	in.Vocabulary[0].Kanji = in.Vocabulary[0].Hiragana
	in.Vocabulary[0].Furigana = nil
	if err := in.Validate(false); err != nil || in.Vocabulary[0].Furigana[0].Reading != "" {
		t.Fatal("kana should not get a redundant ruby reading")
	}
}
func TestFilterValidation(t *testing.T) {
	for _, admin := range []bool{false, true} {
		f := Filter{}
		if err := f.Validate(admin); err != nil {
			t.Fatal(err)
		}
		if (admin && f.Sort != "order") || (!admin && f.Sort != "newest") {
			t.Fatal(f)
		}
	}
	for _, f := range []Filter{{Sort: "injection"}, {Level: "N6"}, {Status: "unknown"}, {Sort: "vocabulary"}} {
		if err := f.Validate(true); err == nil {
			t.Fatalf("accepted admin filter: %+v", f)
		}
	}
	f := Filter{Status: "active"}
	if err := f.Validate(false); err == nil {
		t.Fatal("student status filtering must not expose inactive data")
	}
}
