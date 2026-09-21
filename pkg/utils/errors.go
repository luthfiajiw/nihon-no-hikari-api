package utils

import "errors"

var (
	// Auth & User
	ErrUserNotFound       = errors.New("pengguna tidak ditemukan")
	ErrUserNotRegistered  = errors.New("email belum terdaftar")
	ErrEmailAlreadyTaken  = errors.New("email sudah terpakai, silahkan gunakan email lain")
	ErrInvalidCredentials = errors.New("email atau password tidak valid")
	ErrSessionNotFound    = errors.New("sesi tidak ditemukan")
	ErrSessionBlocked     = errors.New("sesi telah ditutup")
	ErrSessionExpired     = errors.New("sesi kadaluarsa")
	ErrTokenMismatched    = errors.New("mismatched refresh token")
)
