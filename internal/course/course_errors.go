package course

import (
	"encoding/json"
	"errors"
	"strings"
)

var (
	ErrInvalidInput         = errors.New("input tidak valid")
	ErrNotFound             = errors.New("konten tidak ditemukan")
	ErrConflict             = errors.New("konflik konten")
	ErrInvalidRelation      = errors.New("relasi atau konfigurasi konten tidak valid")
	ErrStaleOrder           = errors.New("daftar konten telah berubah. Muat ulang dan coba lagi")
	ErrPayloadTooLarge      = errors.New("payload terlalu besar")
	errInvalidPrerequisites = errors.New("prasyarat tidak valid")
)

// applicationError keeps the public message separate from its classification.
type applicationError struct {
	kind    error
	message string
}

func (e *applicationError) Error() string { return e.message }
func (e *applicationError) Unwrap() error { return e.kind }
func invalidInput(err error) error {
	return &applicationError{kind: ErrInvalidInput, message: err.Error()}
}

func inputMessage(message string) error {
	return &applicationError{kind: ErrInvalidInput, message: message}
}

// Moving request types into model must not change existing JSON error messages.
func decodeError(err error) error {
	var mismatch *json.UnmarshalTypeError
	if !errors.As(err, &mismatch) {
		return err
	}
	copy := *mismatch
	switch copy.Struct {
	case "CoreInput":
		copy.Struct = "coreInput"
	case "SetInput":
		copy.Struct = "setInput"
	case "QuestionInput":
		copy.Struct = "questionInput"
	case "OptionInput":
		copy.Struct = "optionInput"
	}
	message := strings.NewReplacer(
		"model.CoreInput", "admin.coreInput",
		"model.SetInput", "admin.setInput",
		"question.SetInput", "admin.setInput",
		"model.QuestionInput", "admin.questionInput",
		"question.QuestionInput", "admin.questionInput",
		"model.OptionInput", "admin.optionInput",
		"question.OptionInput", "admin.optionInput",
	).Replace(copy.Error())
	return errors.New(message)
}
