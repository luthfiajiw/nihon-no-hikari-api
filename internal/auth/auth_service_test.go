package auth

import (
	"context"
	"errors"
	"testing"
	"time"

	"nihon-no-hikari-api/config"
	"nihon-no-hikari-api/internal/user"
	"nihon-no-hikari-api/pkg/utils"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type fakeTx struct{}

func (fakeTx) Exec(_ context.Context, fn func(tx pgx.Tx) error) error { return fn(nil) }

type fakeVerifier struct {
	claims GoogleClaims
	err    error
}

func (f fakeVerifier) Verify(context.Context, string) (GoogleClaims, error) { return f.claims, f.err }

type fakeAuthRepo struct {
	identities map[string]uuid.UUID
	sessions   []UserSession
}

func (r *fakeAuthRepo) UseTx(pgx.Tx) Repository { return r }
func (r *fakeAuthRepo) CreateSession(_ context.Context, s UserSession) error {
	r.sessions = append(r.sessions, s)
	return nil
}
func (r *fakeAuthRepo) GetSession(context.Context, string) (UserSession, error) {
	return UserSession{}, utils.ErrSessionNotFound
}
func (r *fakeAuthRepo) UpdateRefreshToken(context.Context, uuid.UUID, string) error { return nil }
func (r *fakeAuthRepo) BlockSessionById(context.Context, string) error             { return nil }
func (r *fakeAuthRepo) FindIdentityUserID(_ context.Context, provider, sub string) (uuid.UUID, error) {
	if id, ok := r.identities[provider+":"+sub]; ok {
		return id, nil
	}
	return uuid.Nil, utils.ErrUserNotFound
}
func (r *fakeAuthRepo) CreateIdentity(_ context.Context, userID uuid.UUID, provider, sub, _ string) error {
	r.identities[provider+":"+sub] = userID
	return nil
}

type fakeUserRepo struct {
	users []user.User
}

func (r *fakeUserRepo) UseTx(pgx.Tx) user.Repository { return r }
func (r *fakeUserRepo) FindBy(_ context.Context, p user.FindUserParam) (user.User, error) {
	for _, u := range r.users {
		if (p.ID != uuid.Nil && u.ID == p.ID) || (p.Email != "" && u.Email == p.Email) {
			return u, nil
		}
	}
	return user.User{}, utils.ErrUserNotFound
}
func (r *fakeUserRepo) Create(context.Context, user.UserParams) (user.User, error) {
	return user.User{}, errors.New("unused")
}
func (r *fakeUserRepo) CreateOAuth(_ context.Context, p user.OAuthUserParams) (user.User, error) {
	u := user.User{ID: uuid.New(), Email: p.Email, DisplayName: p.DisplayName, AvatarURL: p.AvatarURL, Role: user.RoleUser}
	r.users = append(r.users, u)
	return u, nil
}
func (r *fakeUserRepo) GetProfile(context.Context, uuid.UUID, uuid.UUID) (user.ProfileData, error) {
	return user.ProfileData{}, errors.New("unused")
}
func (r *fakeUserRepo) UpdateProfile(context.Context, uuid.UUID, string, *string) (user.User, error) {
	return user.User{}, errors.New("unused")
}
func (r *fakeUserRepo) UpdatePassword(context.Context, uuid.UUID, string) error {
	return errors.New("unused")
}
func (r *fakeUserRepo) HasIdentity(context.Context, uuid.UUID, string) (bool, error) {
	return false, errors.New("unused")
}
func (r *fakeUserRepo) FindIdentityUserID(context.Context, string, string) (uuid.UUID, error) {
	return uuid.Nil, errors.New("unused")
}
func (r *fakeUserRepo) CreateIdentity(context.Context, uuid.UUID, string, string, string) error {
	return errors.New("unused")
}
func (r *fakeUserRepo) RevokeOtherSessions(context.Context, uuid.UUID, uuid.UUID) error {
	return errors.New("unused")
}

func newGoogleTestService(v GoogleVerifier, users ...user.User) (Service, *fakeAuthRepo, *fakeUserRepo) {
	cfg := &config.Config{
		JWTSecretKey:         "access-secret-for-tests-0123456789",
		JWTRefreshSecretKey:  "refresh-secret-for-tests-0123456789",
		AccessTokenDuration:  time.Minute,
		RefreshTokenDuration: time.Hour,
	}
	authRepo := &fakeAuthRepo{identities: map[string]uuid.UUID{}}
	userRepo := &fakeUserRepo{users: users}
	return NewService(fakeTx{}, cfg, authRepo, userRepo, v), authRepo, userRepo
}

func goodClaims() GoogleClaims {
	return GoogleClaims{Sub: "sub-1", Email: "a@example.com", EmailVerified: true, Name: "Aya", Picture: "http://p/x.png"}
}

func TestGoogleSignInRejectsInvalidToken(t *testing.T) {
	svc, _, _ := newGoogleTestService(fakeVerifier{err: utils.ErrInvalidGoogleToken})
	_, err := svc.GoogleSignIn(context.Background(), GoogleSignInRequest{IDToken: "bad"}, "ua", "ip")
	if !errors.Is(err, utils.ErrInvalidGoogleToken) {
		t.Fatalf("expected ErrInvalidGoogleToken, got %v", err)
	}
}

func TestGoogleSignInRejectsUnverifiedEmail(t *testing.T) {
	c := goodClaims()
	c.EmailVerified = false
	svc, _, userRepo := newGoogleTestService(fakeVerifier{claims: c})
	_, err := svc.GoogleSignIn(context.Background(), GoogleSignInRequest{IDToken: "token"}, "ua", "ip")
	if !errors.Is(err, utils.ErrGoogleEmailNotVerified) {
		t.Fatalf("expected ErrGoogleEmailNotVerified, got %v", err)
	}
	if len(userRepo.users) != 0 {
		t.Fatal("must not create a user for an unverified email")
	}
}

func TestGoogleSignInCreatesNewUserAndIdentity(t *testing.T) {
	c := goodClaims()
	svc, authRepo, userRepo := newGoogleTestService(fakeVerifier{claims: c})
	res, err := svc.GoogleSignIn(context.Background(), GoogleSignInRequest{IDToken: "token"}, "ua", "ip")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(userRepo.users) != 1 || res.Data.Email != "a@example.com" || res.Data.Role != user.RoleUser {
		t.Fatalf("unexpected created user: %+v", userRepo.users)
	}
	if userRepo.users[0].DisplayName != "Aya" || userRepo.users[0].AvatarURL == nil || *userRepo.users[0].AvatarURL != "http://p/x.png" {
		t.Fatalf("unexpected profile values: %+v", userRepo.users[0])
	}
	if authRepo.identities["google:sub-1"] != res.Data.ID {
		t.Fatalf("identity not recorded: %+v", authRepo.identities)
	}
	if len(authRepo.sessions) != 1 {
		t.Fatalf("expected 1 session, got %d", len(authRepo.sessions))
	}
}

func TestGoogleSignInLinksExistingEmail(t *testing.T) {
	existing := user.User{ID: uuid.New(), Email: "a@example.com", DisplayName: "Old", Password: "hash", Role: user.RoleAdmin}
	c := goodClaims()
	svc, authRepo, userRepo := newGoogleTestService(fakeVerifier{claims: c}, existing)
	res, err := svc.GoogleSignIn(context.Background(), GoogleSignInRequest{IDToken: "token"}, "ua", "ip")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(userRepo.users) != 1 {
		t.Fatalf("should not create duplicate user, users=%+v", userRepo.users)
	}
	if res.Data.ID != existing.ID || res.Data.Role != user.RoleAdmin {
		t.Fatalf("expected existing user returned, got %+v", res.Data)
	}
	if authRepo.identities["google:sub-1"] != existing.ID {
		t.Fatalf("expected identity linked to existing user, got %+v", authRepo.identities)
	}
}

func TestGoogleSignInSecondLoginMatchesBySub(t *testing.T) {
	existing := user.User{ID: uuid.New(), Email: "old@example.com", DisplayName: "Existing", Role: user.RoleUser}
	c := goodClaims()
	c.Email = "new-email@example.com"
	svc, authRepo, _ := newGoogleTestService(fakeVerifier{claims: c}, existing)
	authRepo.identities["google:sub-1"] = existing.ID

	res, err := svc.GoogleSignIn(context.Background(), GoogleSignInRequest{IDToken: "token"}, "ua", "ip")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Data.ID != existing.ID {
		t.Fatalf("expected sub lookup to find existing user, got %+v", res.Data)
	}
}

func TestSignInRejectsOAuthOnlyUser(t *testing.T) {
	oauthOnly := user.User{ID: uuid.New(), Email: "a@example.com", Password: "", Role: user.RoleUser}
	svc, _, _ := newGoogleTestService(fakeVerifier{}, oauthOnly)
	_, err := svc.SignIn(context.Background(), SignInRequest{Email: "a@example.com", Password: ""}, "ua", "ip")
	if !errors.Is(err, utils.ErrInvalidCredentials) {
		t.Fatalf("expected ErrInvalidCredentials, got %v", err)
	}
}
