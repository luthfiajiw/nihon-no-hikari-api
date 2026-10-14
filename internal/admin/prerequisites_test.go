package admin

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/google/uuid"
)

func TestPrerequisitesSurviveDraftRoundTrip(t *testing.T) {
	parent, required := uuid.New(), uuid.New()
	for _, kind := range []string{"module", "lesson"} {
		for _, requiredIDs := range [][]uuid.UUID{nil, {}, {required}} {
			input := coreInput{CourseID: parent, ModuleID: parent, Slug: "hiragana", Title: "Hiragana", IsEntry: true, Content: json.RawMessage(`{"content":"<p>あ</p>","tips":""}`)}
			if kind == "module" {
				input.ModulePrerequisites = requiredIDs
			} else {
				input.LessonPrerequisites = requiredIDs
			}
			raw, err := json.Marshal(input)
			if err != nil {
				t.Fatal(err)
			}
			parsed, err := parseCore(kind, raw)
			if err != nil {
				t.Fatal(err)
			}
			// Published updates are serialized as drafts and parsed again on publish.
			draft, err := json.Marshal(parsed)
			if err != nil {
				t.Fatal(err)
			}
			published, err := parseCore(kind, draft)
			if err != nil {
				t.Fatal(err)
			}
			actual := published.prerequisites(kind)
			if !published.IsEntry || len(actual) != len(requiredIDs) || (actual == nil) != (requiredIDs == nil) {
				t.Fatalf("%s draft changed entry/prerequisites: %#v", kind, published)
			}
			if len(actual) > 0 && actual[0] != required {
				t.Fatal("draft changed required ID")
			}
		}
	}
}

func TestPrerequisitesRejectSelfDuplicatesAndZeroIDs(t *testing.T) {
	id, other := uuid.New(), uuid.New()
	for _, kind := range []string{"module", "lesson"} {
		for _, ids := range [][]uuid.UUID{{id}, {other, other}, {uuid.Nil}} {
			err := validatePrerequisites(context.Background(), nil, kind, id, ids)
			if !errors.Is(err, errInvalidPrerequisites) {
				t.Fatalf("%s accepted invalid prerequisites %v: %v", kind, ids, err)
			}
		}
	}
}
