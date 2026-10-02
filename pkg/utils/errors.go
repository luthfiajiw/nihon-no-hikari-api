package utils

import "errors"

var (
	// Auth & User
	ErrUserNotFound          = errors.New("pengguna tidak ditemukan")
	ErrUserNotRegistered     = errors.New("email belum terdaftar")
	ErrInvalidEmail          = errors.New("email tidak valid")
	ErrInvalidPassword       = errors.New("password tidak sesuai")
	ErrPasswordNotMatch      = errors.New("password tidak sesuai")
	ErrEmailAlreadyTaken     = errors.New("email sudah terpakai, silahkan gunakan email lain")
	ErrInvalidCredentials    = errors.New("email atau password tidak valid")
	ErrSessionNotFound       = errors.New("sesi tidak ditemukan")
	ErrSessionBlocked        = errors.New("sesi telah ditutup")
	ErrSessionExpired        = errors.New("sesi kadaluarsa")
	ErrTokenMismatched       = errors.New("mismatched refresh token")
	ErrInvalidRefreshToken   = errors.New("refresh token tidak valid")
	ErrUnsupportedGrantType  = errors.New("grant_type tidak didukung")
	ErrCourseNotFound        = errors.New("kursus tidak ditemukan")
	ErrLessonNotFound        = errors.New("pelajaran tidak ditemukan")
	ErrModuleNotFound        = errors.New("modul tidak ditemukan pada kursus")
	ErrInvalidModuleProgress = errors.New("data progress modul tidak valid")
)
