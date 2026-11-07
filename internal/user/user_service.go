package user

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"time"

	"nihon-no-hikari-api/pkg/db"
	"nihon-no-hikari-api/pkg/mailer"
	"nihon-no-hikari-api/pkg/utils"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type GoogleTokenVerifier interface {
	Verify(ctx context.Context, idToken string) (sub string, email string, emailVerified bool, err error)
}

type GoogleVerifierFunc func(ctx context.Context, idToken string) (string, string, bool, error)

func (f GoogleVerifierFunc) Verify(ctx context.Context, idToken string) (string, string, bool, error) {
	return f(ctx, idToken)
}

type Service interface {
	GetProfile(ctx context.Context, userID uuid.UUID, currentSessionID uuid.UUID) (ProfileData, error)
	UpdateProfile(ctx context.Context, userID uuid.UUID, req UpdateProfileRequest) (User, error)
	ChangePassword(ctx context.Context, userID uuid.UUID, req ChangePasswordRequest) error
	RequestLinkGoogleCode(ctx context.Context, userID uuid.UUID) error
	VerifyLinkGoogleCode(ctx context.Context, userID uuid.UUID, req VerifyLinkGoogleCodeRequest) (string, error)
	LinkGoogle(ctx context.Context, userID uuid.UUID, req LinkGoogleRequest) error
	UnlinkGoogle(ctx context.Context, userID uuid.UUID, req UnlinkGoogleRequest) error
	RevokeOtherSessions(ctx context.Context, userID uuid.UUID, currentSessionID uuid.UUID) error
}

type service struct {
	tx             db.WithTx
	repo           Repository
	googleVerifier GoogleTokenVerifier
	mailer         mailer.Mailer
}

func NewService(tx db.WithTx, repo Repository, googleVerifier GoogleTokenVerifier, mailer mailer.Mailer) Service {
	return &service{
		tx:             tx,
		repo:           repo,
		googleVerifier: googleVerifier,
		mailer:         mailer,
	}
}

func (s *service) GetProfile(ctx context.Context, userID uuid.UUID, currentSessionID uuid.UUID) (ProfileData, error) {
	return s.repo.GetProfile(ctx, userID, currentSessionID)
}

func (s *service) UpdateProfile(ctx context.Context, userID uuid.UUID, req UpdateProfileRequest) (User, error) {
	trimmedName := strings.TrimSpace(req.DisplayName)
	if trimmedName == "" {
		return User{}, errors.New("nama tampilan tidak boleh kosong")
	}
	if len(trimmedName) > 120 {
		return User{}, errors.New("nama tampilan maksimal 120 karakter")
	}

	return s.repo.UpdateProfile(ctx, userID, trimmedName, req.AvatarURL)
}

func (s *service) ChangePassword(ctx context.Context, userID uuid.UUID, req ChangePasswordRequest) error {
	if len(req.NewPassword) < 8 {
		return errors.New("kata sandi baru minimal 8 karakter")
	}
	if req.NewPassword != req.ConfirmPassword {
		return utils.ErrPasswordNotMatch
	}

	user, err := s.repo.FindBy(ctx, FindUserParam{ID: userID})
	if err != nil {
		return err
	}

	// If the user already has a password, verify current password first
	if user.Password != "" {
		if req.CurrentPassword == "" {
			return utils.ErrCurrentPasswordRequired
		}
		if !utils.CheckPasswordMatch(req.CurrentPassword, user.Password) {
			return utils.ErrCurrentPasswordIncorrect
		}
	}

	hashed, err := utils.HashPassword(req.NewPassword)
	if err != nil {
		return err
	}

	return s.repo.UpdatePassword(ctx, userID, hashed)
}

func (s *service) RequestLinkGoogleCode(ctx context.Context, userID uuid.UUID) error {
	if s.mailer == nil {
		return utils.ErrMailerDisabled
	}

	alreadyLinked, err := s.repo.HasIdentity(ctx, userID, "google")
	if err != nil {
		return err
	}
	if alreadyLinked {
		return utils.ErrGoogleAlreadyLinked
	}

	user, err := s.repo.FindBy(ctx, FindUserParam{ID: userID})
	if err != nil {
		return err
	}

	// Check 60s cooldown from latest request
	latest, err := s.repo.LatestEmailVerification(ctx, userID, "link_google")
	if err != nil {
		return err
	}
	if latest != nil && time.Since(latest.CreatedAt) < 60*time.Second {
		return utils.ErrVerificationRateLimited
	}

	// Check hourly rate limit: max 5 requests per hour
	hourlyCount, err := s.repo.CountRecentEmailVerifications(ctx, userID, "link_google", time.Now().Add(-1*time.Hour))
	if err != nil {
		return err
	}
	if hourlyCount >= 5 {
		return utils.ErrVerificationRateLimited
	}

	code, err := generateOTP()
	if err != nil {
		return fmt.Errorf("gagal membuat kode verifikasi: %w", err)
	}

	codeHash := hashSecret(code)
	expiresAt := time.Now().Add(10 * time.Minute)

	_, err = s.repo.CreateEmailVerification(ctx, userID, "link_google", codeHash, expiresAt)
	if err != nil {
		return err
	}

	subject := "Kode Verifikasi Tautan Akun Google - Gakkobit"
	textBody := fmt.Sprintf("Halo,\n\nKode verifikasi Anda untuk menautkan akun Google adalah: %s\n\nKode ini berlaku selama 10 menit. Jangan berikan kode ini kepada siapapun.\n\nSalam,\nTim Gakkobit", code)
	htmlBody := fmt.Sprintf(`<div style="font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, Helvetica, Arial, sans-serif; max-width: 500px; margin: 0 auto; padding: 32px 24px; border: 1px solid #e5e7eb; border-radius: 12px; background-color: #ffffff;">
		<div style="text-align: center; margin-bottom: 24px;">
			<img src="cid:gakkobit-logo" alt="Gakkobit" style="height: 48px; max-width: 200px; object-fit: contain; margin: 0 auto; display: inline-block;" />
		</div>
		<h2 style="color: #111827; font-size: 20px; font-weight: 700; margin: 0 0 12px; text-align: center;">Verifikasi Tautan Akun Google</h2>
		<p style="color: #4b5563; font-size: 14px; line-height: 1.6; margin: 0 0 24px; text-align: center;">Gunakan kode di bawah ini untuk melanjutkan proses penautan akun Google Anda ke Gakkobit:</p>
		<div style="background-color: #f3f4f6; border-radius: 10px; padding: 20px; text-align: center; margin: 0 0 24px;">
			<span style="font-family: 'SF Mono', Monaco, Consolas, monospace; font-size: 36px; font-weight: 800; letter-spacing: 8px; color: #111827; display: block; margin-left: 8px;">%s</span>
		</div>
		<p style="color: #6b7280; font-size: 13px; line-height: 1.5; margin: 0 0 16px; text-align: center;">Kode ini berlaku selama <strong>10 menit</strong> dan memiliki batas percobaan.</p>
		<hr style="border: none; border-top: 1px solid #f3f4f6; margin: 24px 0;" />
		<p style="color: #9ca3af; font-size: 12px; text-align: center; margin: 0;">Jika Anda tidak merasa meminta penautan akun Google ini, abaikan pesan ini.</p>
	</div>`, code)

	var attachments []mailer.Attachment
	if len(gakkobitLogoBytes) > 0 {
		attachments = append(attachments, mailer.Attachment{
			Filename:    "gakkobit-full-logo.png",
			ContentType: "image/png",
			ContentID:   "gakkobit-logo",
			Data:        gakkobitLogoBytes,
			Inline:      true,
		})
	}

	return s.mailer.SendWithAttachments(ctx, user.Email, subject, textBody, htmlBody, attachments)
}

func (s *service) VerifyLinkGoogleCode(ctx context.Context, userID uuid.UUID, req VerifyLinkGoogleCodeRequest) (string, error) {
	code := strings.TrimSpace(req.Code)
	if len(code) != 6 {
		return "", utils.ErrInvalidVerificationCode
	}

	latest, err := s.repo.LatestEmailVerification(ctx, userID, "link_google")
	if err != nil {
		return "", err
	}
	if latest == nil || time.Now().After(latest.ExpiresAt) {
		return "", utils.ErrInvalidVerificationCode
	}

	if latest.VerifiedAt != nil {
		return "", utils.ErrInvalidVerificationCode
	}

	if latest.Attempts >= 5 {
		return "", utils.ErrVerificationAttemptsExceeded
	}

	attempts, err := s.repo.IncrementVerificationAttempts(ctx, latest.ID)
	if err != nil {
		return "", err
	}

	expectedHash := hashSecret(code)
	if subtle.ConstantTimeCompare([]byte(expectedHash), []byte(latest.CodeHash)) != 1 {
		if attempts >= 5 {
			return "", utils.ErrVerificationAttemptsExceeded
		}
		return "", utils.ErrInvalidVerificationCode
	}

	tokenBytes := make([]byte, 32)
	if _, err := rand.Read(tokenBytes); err != nil {
		return "", fmt.Errorf("gagal membuat token verifikasi: %w", err)
	}
	grantToken := hex.EncodeToString(tokenBytes)
	grantHash := hashSecret(grantToken)
	grantExpiresAt := time.Now().Add(10 * time.Minute)

	if err := s.repo.SetVerificationGrant(ctx, latest.ID, grantHash, grantExpiresAt); err != nil {
		return "", err
	}

	return grantToken, nil
}

func (s *service) LinkGoogle(ctx context.Context, userID uuid.UUID, req LinkGoogleRequest) error {
	if s.googleVerifier == nil {
		return utils.ErrGoogleSignInDisabled
	}

	verificationToken := strings.TrimSpace(req.VerificationToken)
	if verificationToken == "" {
		return utils.ErrVerificationRequired
	}

	grantHash := hashSecret(verificationToken)
	grant, err := s.repo.FindValidVerificationGrant(ctx, userID, "link_google", grantHash)
	if err != nil {
		return err
	}
	if grant == nil {
		return utils.ErrVerificationTokenInvalid
	}

	sub, email, verified, err := s.googleVerifier.Verify(ctx, req.IDToken)
	if err != nil {
		return err
	}

	if !verified {
		return utils.ErrGoogleEmailNotVerified
	}

	// Check if this user already linked Google
	alreadyLinked, err := s.repo.HasIdentity(ctx, userID, "google")
	if err != nil {
		return err
	}
	if alreadyLinked {
		return utils.ErrGoogleAlreadyLinked
	}

	// Check if this Google sub is already linked to another user
	existingUserID, err := s.repo.FindIdentityUserID(ctx, "google", sub)
	if err == nil && existingUserID != uuid.Nil && existingUserID != userID {
		return utils.ErrIdentityAlreadyLinked
	}

	linkOp := func(r Repository) error {
		if err := r.ConsumeVerificationGrant(ctx, grant.ID); err != nil {
			return err
		}
		return r.CreateIdentity(ctx, userID, "google", sub, email)
	}

	if s.tx != nil {
		return s.tx.Exec(ctx, func(tx pgx.Tx) error {
			return linkOp(s.repo.UseTx(tx))
		})
	}
	return linkOp(s.repo)
}

func (s *service) UnlinkGoogle(ctx context.Context, userID uuid.UUID, req UnlinkGoogleRequest) error {
	user, err := s.repo.FindBy(ctx, FindUserParam{ID: userID})
	if err != nil {
		return err
	}

	if user.Password == "" {
		return utils.ErrCannotUnlinkWithoutPassword
	}

	if req.CurrentPassword == "" {
		return utils.ErrCurrentPasswordRequired
	}

	if !utils.CheckPasswordMatch(req.CurrentPassword, user.Password) {
		return utils.ErrCurrentPasswordIncorrect
	}

	alreadyLinked, err := s.repo.HasIdentity(ctx, userID, "google")
	if err != nil {
		return err
	}
	if !alreadyLinked {
		return utils.ErrGoogleNotLinked
	}

	unlinkOp := func(r Repository) error {
		return r.DeleteIdentity(ctx, userID, "google")
	}

	if s.tx != nil {
		return s.tx.Exec(ctx, func(tx pgx.Tx) error {
			return unlinkOp(s.repo.UseTx(tx))
		})
	}
	return unlinkOp(s.repo)
}

func (s *service) RevokeOtherSessions(ctx context.Context, userID uuid.UUID, currentSessionID uuid.UUID) error {
	return s.repo.RevokeOtherSessions(ctx, userID, currentSessionID)
}

func generateOTP() (string, error) {
	n, err := rand.Int(rand.Reader, big.NewInt(1000000))
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%06d", n.Int64()), nil
}

func hashSecret(secret string) string {
	h := sha256.Sum256([]byte(secret))
	return hex.EncodeToString(h[:])
}
