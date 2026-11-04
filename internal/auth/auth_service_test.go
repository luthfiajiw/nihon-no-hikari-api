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
	_, err := svc.GoogleSignIn(context.Background(), GoogleSignInRequest{IDToken: "x"}, "ua", "1.1.1.1")
	if !errors.Is(err, utils.ErrInvalidGoogleToken) {
		t.Fatalf("expected ErrInvalidGoogleToken, got %v", err)
	}
}

func TestGoogleSignInRejectsUnverifiedEmail(t *testing.T) {
	c := goodClaims()
	c.EmailVerified = false
	svc, _, userRepo := newGoogleTestService(fakeVerifier{claims: c})
	_, err := svc.GoogleSignIn(context.Background(), GoogleSignInRequest{IDToken: "x"}, "ua", "ip")
	if !errors.Is(err, utils.ErrGoogleEmailNotVerified) {
		t.Fatalf("expected ErrGoogleEmailNotVerified, got %v", err)
	}
	if len(userRepo.users) != 0 {
		t.Fatal("must not create a user for an unverified email")
	}
}

func TestGoogleSignInCreatesNewUserAndIdentity(t *testing.T) {
	svc, authRepo, userRepo := newGoogleTestService(fakeVerifier{claims: goodClaims()})
	res, err := svc.GoogleSignIn(context.Background(), GoogleSignInRequest{IDToken: "x"}, "ua", "ip")
	if err != nil {
		t.Fatal(err)
	}
	if len(userRepo.users) != 1 || res.Data.Email != "a@example.com" || res.Data.Role != user.RoleUser {
		t.Fatalf("unexpected result: %+v users=%d", res.Data, len(userRepo.users))
	}
	if res.Data.AvatarURL == nil || *res.Data.AvatarURL != "http://p/x.png" {
		t.Fatal("avatar should come from Google")
	}
	if _, ok := authRepo.identities["google:sub-1"]; !ok || len(authRepo.sessions) != 1 {
		t.Fatal("identity and session should be created")
	}
	if res.Data.Session.AccessToken == "" || res.Data.Session.RefreshToken == "" {
		t.Fatal("tokens should be issued")
	}
}

func TestGoogleSignInLinksExistingEmail(t *testing.T) {
	existing := user.User{ID: uuid.New(), Email: "a@example.com", DisplayName: "Old", Password: "hash", Role: user.RoleAdmin}
	svc, authRepo, userRepo := newGoogleTestService(fakeVerifier{claims: goodClaims()}, existing)
	res, err := svc.GoogleSignIn(context.Background(), GoogleSignInRequest{IDToken: "x"}, "ua", "ip")
	if err != nil {
		t.Fatal(err)
	}
	if len(userRepo.users) != 1 || res.Data.ID != existing.ID || res.Data.Role != user.RoleAdmin {
		t.Fatal("should sign into the existing account")
	}
	if authRepo.identities["google:sub-1"] != existing.ID {
		t.Fatal("identity should be linked to the existing user")
	}
}

func TestGoogleSignInSecondLoginMatchesBySub(t *testing.T) {
	svc, authRepo, userRepo := newGoogleTestService(fakeVerifier{claims: goodClaims()})
	first, err := svc.GoogleSignIn(context.Background(), GoogleSignInRequest{IDToken: "x"}, "ua", "ip")
	if err != nil {
		t.Fatal(err)
	}
	second, err := svc.GoogleSignIn(context.Background(), GoogleSignInRequest{IDToken: "x"}, "ua", "ip")
	if err != nil {
		t.Fatal(err)
	}
	if first.Data.ID != second.Data.ID || len(userRepo.users) != 1 || len(authRepo.sessions) != 2 {
		t.Fatal("second login must reuse the same user and create a new session")
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
