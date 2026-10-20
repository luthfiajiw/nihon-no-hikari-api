package question

import (
	"encoding/json"
	"strings"
	"unicode/utf8"

	"nihon-no-hikari-api/pkg/kanastroke"
	"nihon-no-hikari-api/pkg/utils"
)

func validateWritingGuide(guide *WritingGuide) error {
	if guide == nil || (!kanastroke.ValidCharacter("hiragana", guide.Character) && !kanastroke.ValidCharacter("katakana", guide.Character)) ||
		strings.TrimSpace(guide.Romaji) == "" || utf8.RuneCountInString(guide.Romaji) > 8 {
		return utils.ErrInvalidQuestionConfiguration
	}
	if err := kanastroke.Validate(kanastroke.Data{ViewBox: guide.ViewBox, Strokes: guide.Strokes}); err != nil {
		return utils.ErrInvalidQuestionConfiguration
	}
	return nil
}

func encodeWritingSnapshot(item Question) (json.RawMessage, error) {
	if item.QuestionType != QuestionTypeStrokeWriting {
		return nil, nil
	}
	if err := validateWritingGuide(item.WritingGuide); err != nil {
		return nil, err
	}
	return json.Marshal(item.WritingGuide)
}

func decodeWritingSnapshot(raw []byte) (*WritingGuide, error) {
	// Pointer coordinates distinguish missing/null fields from a valid zero.
	var payload *struct {
		Character string  `json:"character"`
		Romaji    string  `json:"romaji"`
		ViewBox   float64 `json:"view_box"`
		Strokes   []struct {
			Path   string `json:"path"`
			Points []struct {
				X *float64 `json:"x"`
				Y *float64 `json:"y"`
			} `json:"points"`
		} `json:"strokes"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil || payload == nil {
		return nil, utils.ErrInvalidQuestionConfiguration
	}
	guide := &WritingGuide{Character: payload.Character, Romaji: payload.Romaji, ViewBox: payload.ViewBox, Strokes: make([]GuideStroke, len(payload.Strokes))}
	for i, stroke := range payload.Strokes {
		guide.Strokes[i] = GuideStroke{Path: stroke.Path, Points: make([]StrokePoint, len(stroke.Points))}
		for j, point := range stroke.Points {
			if point.X == nil || point.Y == nil {
				return nil, utils.ErrInvalidQuestionConfiguration
			}
			guide.Strokes[i].Points[j] = StrokePoint{X: *point.X, Y: *point.Y}
		}
	}
	if err := validateWritingGuide(guide); err != nil {
		return nil, err
	}
	return guide, nil
}
