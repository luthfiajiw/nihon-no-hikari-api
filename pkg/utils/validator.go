package utils

import (
	"errors"
	"regexp"

	"github.com/go-playground/validator/v10"
)

type BodyValidator struct {
	Validator *validator.Validate
}

func (v BodyValidator) Validate(data any) *ErrorRes {
	err := v.Validator.Struct(data)
	if err != nil {
		return &ErrorRes{Success: false, Error: err.Error()}
	}
	return nil
}

var emailRegex = regexp.MustCompile(`^[a-zA-Z0-9._%+\-]+@[a-zA-Z0-9.\-]+\.[a-zA-Z]{2,}$`)

func ValidateEmail(email string) bool {
	if email == "" {
		return false
	}
	return emailRegex.MatchString(email)
}

func ValidatePassword(pw string) error {
	if len(pw) < 6 {
		return errors.New("panjang password minimal 6 karakter")
	}
	return nil
}
