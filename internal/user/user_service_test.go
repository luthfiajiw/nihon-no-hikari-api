package user

import (
	"context"
	"errors"
	"testing"
	"time"

	"nihon-no-hikari-api/pkg/mailer"
	"nihon-no-hikari-api/pkg/utils"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type sentEmail struct {
	to, subject, text, html string
}

type mockMailer struct {
	sentEmails []sentEmail
	err        error
}

func (m *mockMailer) Send(ctx context.Context, to, subject, text, html string) error {
	return m.SendWithAttachments(ctx, to, subject, text, html, nil)
}

func (m *mockMailer) SendWithAttachments(ctx context.Context, to, subject, text, html string, attachments []mailer.Attachment) error {
	if m.err != nil {
		return m.err
	}
	m.sentEmails = append(m.sentEmails, sentEmail{to: to, subject: subject, text: text, html: html})
	return nil
}

type mockUserRepo struct {
	users         map[uuid.UUID]User
	identities    map[string]uuid.UUID // provider:sub -> userID
	userIdent     map[uuid.UUID]map[string]string
	sessions      map[uuid.UUID][]ProfileSessionSummary
	passwordHash  map[uuid.UUID]string
	verifications []EmailVerification
}

func newMockUserRepo() *mockUserRepo {
	return &mockUserRepo{
		users:         make(map[uuid.UUID]User),
		identities:    make(map[string]uuid.UUID),
		userIdent:     make(map[uuid.UUID]map[string]string),
		sessions:      make(map[uuid.UUID][]ProfileSessionSummary),
		passwordHash:  make(map[uuid.UUID]string),
		verifications: make([]EmailVerification, 0),
	}
}

func (m *mockUserRepo) UseTx(tx pgx.Tx) Repository { return m }

func (m *mockUserRepo) FindBy(ctx context.Context, params FindUserParam) (User, error) {
	if params.ID != uuid.Nil {
		u, ok := m.users[params.ID]
		if !ok {
			return User{}, utils.ErrUserNotFound
		}
		u.Password = m.passwordHash[u.ID]
		return u, nil
	}
	for _, u := range m.users {
		if u.Email == params.Email {
			u.Password = m.passwordHash[u.ID]
			return u, nil
		}
	}
	return User{}, utils.ErrUserNotFound
}

func (m *mockUserRepo) Create(ctx context.Context, params UserParams) (User, error) {
	id := uuid.New()
	u := User{ID: id, Email: params.Email, DisplayName: params.DisplayName, Role: RoleUser}
	m.users[id] = u
	m.passwordHash[id] = params.Password
	return u, nil
}

func (m *mockUserRepo) CreateOAuth(ctx context.Context, params OAuthUserParams) (User, error) {
	id := uuid.New()
	u := User{ID: id, Email: params.Email, DisplayName: params.DisplayName, AvatarURL: params.AvatarURL, Role: RoleUser}
	m.users[id] = u
	return u, nil
}

func (m *mockUserRepo) GetProfile(ctx context.Context, userID uuid.UUID, currentSessionID uuid.UUID) (ProfileData, error) {
	u, ok := m.users[userID]
	if !ok {
		return ProfileData{}, utils.ErrUserNotFound
	}
	pwd := m.passwordHash[userID]
	hasPwd := pwd != ""

	_, googleLinked := m.userIdent[userID]["google"]

	sessions := make([]ProfileSessionSummary, 0)
	for _, s := range m.sessions[userID] {
		s.IsCurrent = (s.ID == currentSessionID)
		sessions = append(sessions, s)
	}

	return ProfileData{
		ID:           u.ID,
		Email:        u.Email,
		DisplayName:  u.DisplayName,
		AvatarURL:    u.AvatarURL,
		Role:         u.Role,
		HasPassword:  hasPwd,
		GoogleLinked: googleLinked,
		Sessions:     sessions,
	}, nil
}

func (m *mockUserRepo) UpdateProfile(ctx context.Context, userID uuid.UUID, displayName string, avatarURL *string) (User, error) {
	u, ok := m.users[userID]
	if !ok {
		return User{}, utils.ErrUserNotFound
	}
	u.DisplayName = displayName
	u.AvatarURL = avatarURL
	m.users[userID] = u
	return u, nil
}

func (m *mockUserRepo) UpdatePassword(ctx context.Context, userID uuid.UUID, passwordHash string) error {
	if _, ok := m.users[userID]; !ok {
		return utils.ErrUserNotFound
	}
	m.passwordHash[userID] = passwordHash
	return nil
}

func (m *mockUserRepo) HasIdentity(ctx context.Context, userID uuid.UUID, provider string) (bool, error) {
	if m.userIdent[userID] == nil {
		return false, nil
	}
	_, ok := m.userIdent[userID][provider]
	return ok, nil
}

func (m *mockUserRepo) FindIdentityUserID(ctx context.Context, provider string, sub string) (uuid.UUID, error) {
	id, ok := m.identities[provider+":"+sub]
	if !ok {
		return uuid.Nil, utils.ErrUserNotFound
	}
	return id, nil
}

func (m *mockUserRepo) CreateIdentity(ctx context.Context, userID uuid.UUID, provider string, sub string, email string) error {
	m.identities[provider+":"+sub] = userID
	if m.userIdent[userID] == nil {
		m.userIdent[userID] = make(map[string]string)
	}
	m.userIdent[userID][provider] = sub
	return nil
}

func (m *mockUserRepo) DeleteIdentity(ctx context.Context, userID uuid.UUID, provider string) error {
	if m.userIdent[userID] == nil {
		return utils.ErrGoogleNotLinked
	}
	sub, ok := m.userIdent[userID][provider]
	if !ok {
		return utils.ErrGoogleNotLinked
	}
	delete(m.userIdent[userID], provider)
	delete(m.identities, provider+":"+sub)
	return nil
}

func (m *mockUserRepo) RevokeOtherSessions(ctx context.Context, userID uuid.UUID, currentSessionID uuid.UUID) error {
	kept := make([]ProfileSessionSummary, 0)
	for _, s := range m.sessions[userID] {
		if s.ID == currentSessionID {
			kept = append(kept, s)
		}
	}
	m.sessions[userID] = kept
	return nil
}

func (m *mockUserRepo) CreateEmailVerification(ctx context.Context, userID uuid.UUID, purpose string, codeHash string, expiresAt time.Time) (uuid.UUID, error) {
	id := uuid.New()
	ev := EmailVerification{
		ID:        id,
		UserID:    userID,
		Purpose:   purpose,
		CodeHash:  codeHash,
		ExpiresAt: expiresAt,
		CreatedAt: time.Now(),
	}
	m.verifications = append(m.verifications, ev)
	return id, nil
}

func (m *mockUserRepo) LatestEmailVerification(ctx context.Context, userID uuid.UUID, purpose string) (*EmailVerification, error) {
	for i := len(m.verifications) - 1; i >= 0; i-- {
		ev := m.verifications[i]
		if ev.UserID == userID && ev.Purpose == purpose {
			copy := ev
			return &copy, nil
		}
	}
	return nil, nil
}

func (m *mockUserRepo) CountRecentEmailVerifications(ctx context.Context, userID uuid.UUID, purpose string, since time.Time) (int, error) {
	count := 0
	for _, ev := range m.verifications {
		if ev.UserID == userID && ev.Purpose == purpose && !ev.CreatedAt.Before(since) {
			count++
		}
	}
	return count, nil
}

func (m *mockUserRepo) IncrementVerificationAttempts(ctx context.Context, id uuid.UUID) (int, error) {
	for i := range m.verifications {
		if m.verifications[i].ID == id {
			m.verifications[i].Attempts++
			return m.verifications[i].Attempts, nil
		}
	}
	return 0, utils.ErrInvalidVerificationCode
}

func (m *mockUserRepo) SetVerificationGrant(ctx context.Context, id uuid.UUID, grantHash string, grantExpiresAt time.Time) error {
	for i := range m.verifications {
		if m.verifications[i].ID == id {
			now := time.Now()
			m.verifications[i].VerifiedAt = &now
			m.verifications[i].GrantHash = &grantHash
			m.verifications[i].GrantExpiresAt = &grantExpiresAt
			return nil
		}
	}
	return utils.ErrInvalidVerificationCode
}

func (m *mockUserRepo) FindValidVerificationGrant(ctx context.Context, userID uuid.UUID, purpose string, grantHash string) (*EmailVerification, error) {
	now := time.Now()
	for i := len(m.verifications) - 1; i >= 0; i-- {
		ev := m.verifications[i]
		if ev.UserID == userID && ev.Purpose == purpose && ev.GrantHash != nil && *ev.GrantHash == grantHash && ev.ConsumedAt == nil && ev.GrantExpiresAt != nil && ev.GrantExpiresAt.After(now) {
			copy := ev
			return &copy, nil
		}
	}
	return nil, nil
}

func (m *mockUserRepo) ConsumeVerificationGrant(ctx context.Context, id uuid.UUID) error {
	for i := range m.verifications {
		if m.verifications[i].ID == id {
			if m.verifications[i].ConsumedAt != nil {
				return utils.ErrVerificationTokenInvalid
			}
			now := time.Now()
			m.verifications[i].ConsumedAt = &now
			return nil
		}
	}
	return utils.ErrVerificationTokenInvalid
}

func TestUserService_GetProfile(t *testing.T) {
	repo := newMockUserRepo()
	svc := NewService(nil, repo, nil, nil)

	userID := uuid.New()
	sessionID := uuid.New()
	avatar := "https://example.com/avatar.png"
	repo.users[userID] = User{ID: userID, Email: "test@example.com", DisplayName: "Kenji", AvatarURL: &avatar, Role: RoleUser}
	repo.sessions[userID] = []ProfileSessionSummary{
		{ID: sessionID, ClientIP: "127.0.0.1", UserAgent: "Chrome", CreatedAt: time.Now()},
		{ID: uuid.New(), ClientIP: "10.0.0.1", UserAgent: "Firefox", CreatedAt: time.Now()},
	}

	p, err := svc.GetProfile(context.Background(), userID, sessionID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.DisplayName != "Kenji" || p.Email != "test@example.com" {
		t.Errorf("wrong user data: %+v", p)
	}
	if p.HasPassword {
		t.Errorf("expected HasPassword false for oauth user")
	}
	if len(p.Sessions) != 2 {
		t.Fatalf("expected 2 sessions, got %d", len(p.Sessions))
	}
	if !p.Sessions[0].IsCurrent {
		t.Errorf("expected first session to be current")
	}
}

func TestUserService_UpdateProfile(t *testing.T) {
	repo := newMockUserRepo()
	svc := NewService(nil, repo, nil, nil)

	userID := uuid.New()
	repo.users[userID] = User{ID: userID, Email: "test@example.com", DisplayName: "Kenji", Role: RoleUser}

	// Reject empty name
	_, err := svc.UpdateProfile(context.Background(), userID, UpdateProfileRequest{DisplayName: "   "})
	if err == nil {
		t.Error("expected error for empty display name")
	}

	// Successful update
	newAva := "preset:sakura"
	updated, err := svc.UpdateProfile(context.Background(), userID, UpdateProfileRequest{DisplayName: "Ren Tanaka", AvatarURL: &newAva})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if updated.DisplayName != "Ren Tanaka" || *updated.AvatarURL != "preset:sakura" {
		t.Errorf("update failed: %+v", updated)
	}
}

func TestUserService_ChangePassword(t *testing.T) {
	repo := newMockUserRepo()
	svc := NewService(nil, repo, nil, nil)

	userID := uuid.New()
	initialHash, _ := utils.HashPassword("secret123")
	repo.users[userID] = User{ID: userID, Email: "user@test.com", DisplayName: "User", Role: RoleUser}
	repo.passwordHash[userID] = initialHash

	// 1. Password mismatch with confirm
	err := svc.ChangePassword(context.Background(), userID, ChangePasswordRequest{
		CurrentPassword: "secret123",
		NewPassword:     "newpassword123",
		ConfirmPassword: "differentpassword",
	})
	if !errors.Is(err, utils.ErrPasswordNotMatch) {
		t.Errorf("expected ErrPasswordNotMatch, got %v", err)
	}

	// 2. Wrong current password
	err = svc.ChangePassword(context.Background(), userID, ChangePasswordRequest{
		CurrentPassword: "wrongpassword",
		NewPassword:     "newpassword123",
		ConfirmPassword: "newpassword123",
	})
	if !errors.Is(err, utils.ErrCurrentPasswordIncorrect) {
		t.Errorf("expected ErrCurrentPasswordIncorrect, got %v", err)
	}

	// 3. Successful change with correct current password
	err = svc.ChangePassword(context.Background(), userID, ChangePasswordRequest{
		CurrentPassword: "secret123",
		NewPassword:     "brandnewpass123",
		ConfirmPassword: "brandnewpass123",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !utils.CheckPasswordMatch("brandnewpass123", repo.passwordHash[userID]) {
		t.Errorf("password was not updated properly in repository")
	}

	// 4. OAuth user without existing password can set password without current password
	oauthUserID := uuid.New()
	repo.users[oauthUserID] = User{ID: oauthUserID, Email: "oauth@test.com", DisplayName: "OAuth User", Role: RoleUser}
	err = svc.ChangePassword(context.Background(), oauthUserID, ChangePasswordRequest{
		NewPassword:     "firstpassword123",
		ConfirmPassword: "firstpassword123",
	})
	if err != nil {
		t.Fatalf("expected oauth user to set password without current password: %v", err)
	}
	if !utils.CheckPasswordMatch("firstpassword123", repo.passwordHash[oauthUserID]) {
		t.Errorf("password was not set for oauth user")
	}
}

func TestUserService_RequestLinkGoogleCode(t *testing.T) {
	repo := newMockUserRepo()
	mailer := &mockMailer{}
	svc := NewService(nil, repo, nil, mailer)

	userID := uuid.New()
	repo.users[userID] = User{ID: userID, Email: "kenji@example.com", DisplayName: "Kenji", Role: RoleUser}

	// 1. Mailer disabled
	svcNoMailer := NewService(nil, repo, nil, nil)
	err := svcNoMailer.RequestLinkGoogleCode(context.Background(), userID)
	if !errors.Is(err, utils.ErrMailerDisabled) {
		t.Errorf("expected ErrMailerDisabled, got %v", err)
	}

	// 2. Success request
	err = svc.RequestLinkGoogleCode(context.Background(), userID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(mailer.sentEmails) != 1 {
		t.Fatalf("expected 1 sent email, got %d", len(mailer.sentEmails))
	}
	if mailer.sentEmails[0].to != "kenji@example.com" {
		t.Errorf("expected email to kenji@example.com, got %s", mailer.sentEmails[0].to)
	}
	if len(repo.verifications) != 1 {
		t.Fatalf("expected 1 verification record, got %d", len(repo.verifications))
	}

	// 3. Cooldown rejection within 60s
	err = svc.RequestLinkGoogleCode(context.Background(), userID)
	if !errors.Is(err, utils.ErrVerificationRateLimited) {
		t.Errorf("expected ErrVerificationRateLimited within 60s, got %v", err)
	}

	// 4. Already linked Google
	linkedUserID := uuid.New()
	repo.users[linkedUserID] = User{ID: linkedUserID, Email: "linked@example.com", DisplayName: "Linked", Role: RoleUser}
	_ = repo.CreateIdentity(context.Background(), linkedUserID, "google", "sub-123", "linked@example.com")
	err = svc.RequestLinkGoogleCode(context.Background(), linkedUserID)
	if !errors.Is(err, utils.ErrGoogleAlreadyLinked) {
		t.Errorf("expected ErrGoogleAlreadyLinked, got %v", err)
	}
}

func TestUserService_VerifyLinkGoogleCode(t *testing.T) {
	repo := newMockUserRepo()
	mailer := &mockMailer{}
	svc := NewService(nil, repo, nil, mailer)

	userID := uuid.New()
	repo.users[userID] = User{ID: userID, Email: "verify@example.com", DisplayName: "Verify", Role: RoleUser}

	// 1. Invalid code length
	_, err := svc.VerifyLinkGoogleCode(context.Background(), userID, VerifyLinkGoogleCodeRequest{Code: "123"})
	if !errors.Is(err, utils.ErrInvalidVerificationCode) {
		t.Errorf("expected ErrInvalidVerificationCode, got %v", err)
	}

	// 2. No request sent yet
	_, err = svc.VerifyLinkGoogleCode(context.Background(), userID, VerifyLinkGoogleCodeRequest{Code: "123456"})
	if !errors.Is(err, utils.ErrInvalidVerificationCode) {
		t.Errorf("expected ErrInvalidVerificationCode, got %v", err)
	}

	// Seed code
	code := "654321"
	_, _ = repo.CreateEmailVerification(context.Background(), userID, "link_google", hashSecret(code), time.Now().Add(10*time.Minute))

	// 3. Wrong code
	_, err = svc.VerifyLinkGoogleCode(context.Background(), userID, VerifyLinkGoogleCodeRequest{Code: "000000"})
	if !errors.Is(err, utils.ErrInvalidVerificationCode) {
		t.Errorf("expected ErrInvalidVerificationCode, got %v", err)
	}

	// 4. Max attempts exhaustion
	for i := 0; i < 4; i++ {
		_, _ = svc.VerifyLinkGoogleCode(context.Background(), userID, VerifyLinkGoogleCodeRequest{Code: "000000"})
	}
	_, err = svc.VerifyLinkGoogleCode(context.Background(), userID, VerifyLinkGoogleCodeRequest{Code: code})
	if !errors.Is(err, utils.ErrVerificationAttemptsExceeded) {
		t.Errorf("expected ErrVerificationAttemptsExceeded after 5 attempts, got %v", err)
	}

	// 5. Success with fresh request
	freshCode := "112233"
	_, _ = repo.CreateEmailVerification(context.Background(), userID, "link_google", hashSecret(freshCode), time.Now().Add(10*time.Minute))
	token, err := svc.VerifyLinkGoogleCode(context.Background(), userID, VerifyLinkGoogleCodeRequest{Code: freshCode})
	if err != nil {
		t.Fatalf("unexpected verify error: %v", err)
	}
	if len(token) != 64 { // hex of 32 bytes
		t.Errorf("expected 64 char hex token, got %s", token)
	}

	// 6. Cannot verify again after verified
	_, err = svc.VerifyLinkGoogleCode(context.Background(), userID, VerifyLinkGoogleCodeRequest{Code: freshCode})
	if !errors.Is(err, utils.ErrInvalidVerificationCode) {
		t.Errorf("expected ErrInvalidVerificationCode on re-verification, got %v", err)
	}
}

func TestUserService_LinkGoogle(t *testing.T) {
	repo := newMockUserRepo()
	mockVerifier := GoogleVerifierFunc(func(ctx context.Context, idToken string) (string, string, bool, error) {
		if idToken == "valid-token" {
			return "google-sub-1", "test@gmail.com", true, nil
		}
		if idToken == "unverified-token" {
			return "google-sub-2", "unverified@gmail.com", false, nil
		}
		return "", "", false, utils.ErrInvalidGoogleToken
	})
	svc := NewService(nil, repo, mockVerifier, nil)

	userID := uuid.New()
	repo.users[userID] = User{ID: userID, Email: "test@example.com", DisplayName: "Kenji", Role: RoleUser}

	// 1. Missing verification token
	err := svc.LinkGoogle(context.Background(), userID, LinkGoogleRequest{IDToken: "valid-token", VerificationToken: ""})
	if !errors.Is(err, utils.ErrVerificationRequired) {
		t.Errorf("expected ErrVerificationRequired, got %v", err)
	}

	// 2. Invalid verification token
	err = svc.LinkGoogle(context.Background(), userID, LinkGoogleRequest{IDToken: "valid-token", VerificationToken: "invalid-token"})
	if !errors.Is(err, utils.ErrVerificationTokenInvalid) {
		t.Errorf("expected ErrVerificationTokenInvalid, got %v", err)
	}

	// Setup valid verification grant
	grantToken := "a1b2c3d4e5f60718293a4b5c6d7e8f90a1b2c3d4e5f60718293a4b5c6d7e8f90"
	evID, _ := repo.CreateEmailVerification(context.Background(), userID, "link_google", "dummyhash", time.Now().Add(10*time.Minute))
	_ = repo.SetVerificationGrant(context.Background(), evID, hashSecret(grantToken), time.Now().Add(10*time.Minute))

	// 3. Invalid Google ID token
	err = svc.LinkGoogle(context.Background(), userID, LinkGoogleRequest{IDToken: "bad", VerificationToken: grantToken})
	if !errors.Is(err, utils.ErrInvalidGoogleToken) {
		t.Errorf("expected ErrInvalidGoogleToken, got %v", err)
	}

	// 4. Unverified Google email
	err = svc.LinkGoogle(context.Background(), userID, LinkGoogleRequest{IDToken: "unverified-token", VerificationToken: grantToken})
	if !errors.Is(err, utils.ErrGoogleEmailNotVerified) {
		t.Errorf("expected ErrGoogleEmailNotVerified, got %v", err)
	}

	// 5. Successful link
	err = svc.LinkGoogle(context.Background(), userID, LinkGoogleRequest{IDToken: "valid-token", VerificationToken: grantToken})
	if err != nil {
		t.Fatalf("unexpected link error: %v", err)
	}

	// 6. Grant token consumed (cannot be reused)
	err = svc.LinkGoogle(context.Background(), userID, LinkGoogleRequest{IDToken: "valid-token", VerificationToken: grantToken})
	if !errors.Is(err, utils.ErrVerificationTokenInvalid) {
		t.Errorf("expected ErrVerificationTokenInvalid on reused grant, got %v", err)
	}
}

func TestUserService_RevokeOtherSessions(t *testing.T) {
	repo := newMockUserRepo()
	svc := NewService(nil, repo, nil, nil)

	userID := uuid.New()
	currentSessionID := uuid.New()
	repo.users[userID] = User{ID: userID, Email: "test@example.com"}
	repo.sessions[userID] = []ProfileSessionSummary{
		{ID: currentSessionID, ClientIP: "1.1.1.1"},
		{ID: uuid.New(), ClientIP: "2.2.2.2"},
		{ID: uuid.New(), ClientIP: "3.3.3.3"},
	}

	err := svc.RevokeOtherSessions(context.Background(), userID, currentSessionID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(repo.sessions[userID]) != 1 || repo.sessions[userID][0].ID != currentSessionID {
		t.Errorf("expected only current session to remain, got %v", repo.sessions[userID])
	}
}

func TestUserService_UnlinkGoogle(t *testing.T) {
	repo := newMockUserRepo()
	svc := NewService(nil, repo, nil, nil)

	// 1. OAuth-only user (no password set)
	oauthUserID := uuid.New()
	repo.users[oauthUserID] = User{ID: oauthUserID, Email: "oauth@example.com", DisplayName: "OAuth User", Role: RoleUser}
	_ = repo.CreateIdentity(context.Background(), oauthUserID, "google", "sub-oauth", "oauth@example.com")

	err := svc.UnlinkGoogle(context.Background(), oauthUserID, UnlinkGoogleRequest{CurrentPassword: "anypassword"})
	if !errors.Is(err, utils.ErrCannotUnlinkWithoutPassword) {
		t.Fatalf("expected ErrCannotUnlinkWithoutPassword, got %v", err)
	}

	// 2. User with password
	userID := uuid.New()
	pwdHash, _ := utils.HashPassword("correctpassword123")
	repo.users[userID] = User{ID: userID, Email: "user@example.com", DisplayName: "Regular User", Role: RoleUser}
	repo.passwordHash[userID] = pwdHash

	// 2a. User has password, but Google is not linked yet
	err = svc.UnlinkGoogle(context.Background(), userID, UnlinkGoogleRequest{CurrentPassword: "correctpassword123"})
	if !errors.Is(err, utils.ErrGoogleNotLinked) {
		t.Fatalf("expected ErrGoogleNotLinked, got %v", err)
	}

	// Link Google to userID
	_ = repo.CreateIdentity(context.Background(), userID, "google", "sub-12345", "user@example.com")

	// 2b. Empty current password
	err = svc.UnlinkGoogle(context.Background(), userID, UnlinkGoogleRequest{CurrentPassword: ""})
	if !errors.Is(err, utils.ErrCurrentPasswordRequired) {
		t.Fatalf("expected ErrCurrentPasswordRequired, got %v", err)
	}

	// 2c. Incorrect current password
	err = svc.UnlinkGoogle(context.Background(), userID, UnlinkGoogleRequest{CurrentPassword: "wrongpassword"})
	if !errors.Is(err, utils.ErrCurrentPasswordIncorrect) {
		t.Fatalf("expected ErrCurrentPasswordIncorrect, got %v", err)
	}

	// 2d. Successful unlinking
	err = svc.UnlinkGoogle(context.Background(), userID, UnlinkGoogleRequest{CurrentPassword: "correctpassword123"})
	if err != nil {
		t.Fatalf("unexpected unlinking error: %v", err)
	}

	// Verify profile reflects GoogleLinked == false
	profile, err := svc.GetProfile(context.Background(), userID, uuid.Nil)
	if err != nil {
		t.Fatalf("unexpected GetProfile error: %v", err)
	}
	if profile.GoogleLinked {
		t.Fatalf("expected GoogleLinked to be false after unlinking")
	}

	// Verify identity lookup returns ErrUserNotFound
	foundID, err := repo.FindIdentityUserID(context.Background(), "google", "sub-12345")
	if !errors.Is(err, utils.ErrUserNotFound) || foundID != uuid.Nil {
		t.Fatalf("expected ErrUserNotFound for unlinked sub, got id=%v err=%v", foundID, err)
	}

	// 2e. Subsequent duplicate unlink attempt returns ErrGoogleNotLinked
	err = svc.UnlinkGoogle(context.Background(), userID, UnlinkGoogleRequest{CurrentPassword: "correctpassword123"})
	if !errors.Is(err, utils.ErrGoogleNotLinked) {
		t.Fatalf("expected ErrGoogleNotLinked on duplicate unlink, got %v", err)
	}
}
