package auth

import (
	"context"
	"errors"
	"fmt"
	"nihon-no-hikari-api/config"
	"nihon-no-hikari-api/internal/user"
	"nihon-no-hikari-api/pkg/db"
	"nihon-no-hikari-api/pkg/utils"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type Service interface {
	Register(ctx context.Context, req RegisterRequest) error
	SignIn(ctx context.Context, req SignInRequest, userAgent string, clientIP string) (*SignInResponse, error)
	Logout(ctx context.Context, req LogoutRequest) error
	RefreshAccessToken(ctx context.Context, req TokenRequest) (*RefreshTokenResponse, error)
}

type service struct {
	tx       db.WithTx
	cfg      *config.Config
	authRepo Repository
	userRepo user.Repository
}

// Logout implements [Service].
func (s *service) Logout(ctx context.Context, req LogoutRequest) error {
	err := s.authRepo.BlockSessionById(ctx, req.ID)
	if err != nil {
		return err
	}
	return nil
}

// RefreshAccessToken implements [Service].
func (s *service) RefreshAccessToken(ctx context.Context, req TokenRequest) (*RefreshTokenResponse, error) {
	refreshPayload, err := utils.VerifyToken(req.Token, s.cfg.JWTRefreshSecretKey)
	if err != nil {
		return nil, fmt.Errorf("invalid refresh token: %w", err)
	}

	session, err := s.authRepo.GetSession(ctx, req.Token)
	if err != nil {
		if errors.Is(err, utils.ErrSessionNotFound) {
			return nil, utils.ErrSessionBlocked
		}
		return nil, err
	}

	if time.Now().After(session.ExpiresAt) {
		return nil, utils.ErrSessionExpired
	}

	accessToken, accessPayload, err := utils.CreateToken(session.ID, session.UserID, refreshPayload.Email, s.cfg.AccessTokenDuration, s.cfg.JWTSecretKey)
	if err != nil {
		return nil, fmt.Errorf("failed to create access token: %w", err)
	}

	newRefreshToken, newRefreshPayload, err := utils.CreateToken(session.ID, session.UserID, refreshPayload.Email, s.cfg.RefreshTokenDuration, s.cfg.JWTRefreshSecretKey)
	if err != nil {
		return nil, fmt.Errorf("failed to create refresh token: %w", err)
	}

	err = s.authRepo.UpdateRefreshToken(ctx, session.ID, newRefreshToken)
	if err != nil {
		return nil, err
	}

	return &RefreshTokenResponse{
		Success: true,
		Message: "sucessful",
		Data: AuthSession{
			ID:                    session.ID,
			AccessToken:           accessToken,
			AccessTokenExpiresAt:  accessPayload.ExpiresAt.Time,
			RefreshToken:          newRefreshToken,
			RefreshTokenExpiresAt: newRefreshPayload.ExpiresAt.Time,
		},
	}, nil
}

// Register implements [Service].
func (s *service) Register(ctx context.Context, req RegisterRequest) error {
	hashPassword, err := utils.HashPassword(req.Password)
	if err != nil {
		return errors.New("failed to hash password")
	}
	req.Password = hashPassword

	err = s.tx.Exec(ctx, func(tx pgx.Tx) error {
		_, err := s.userRepo.CreateTx(ctx, tx, user.UserParams{
			DisplayName:     req.DisplayName,
			Email:           req.Email,
			Password:        req.Password,
			ConfirmPassword: req.ConfirmPassword,
		})
		if err != nil {
			return err
		}

		return nil
	})
	if err != nil {
		return err
	}
	return nil
}

// SignIn implements [Service].
func (s *service) SignIn(ctx context.Context, req SignInRequest, userAgent string, clientIP string) (*SignInResponse, error) {
	userDetail, err := s.userRepo.FindBy(ctx, user.FindUserParam{Email: req.Email})
	if err != nil {
		if errors.Is(err, utils.ErrUserNotFound) {
			return nil, utils.ErrUserNotRegistered
		}
		return nil, err
	}

	isPasswordMatch := utils.CheckPasswordMatch(req.Password, userDetail.Password)
	if !isPasswordMatch {
		return nil, utils.ErrInvalidCredentials
	}

	accessTokenID, err := uuid.NewRandom()
	if err != nil {
		return nil, fmt.Errorf("failed to create access token id: %w", err)
	}
	accessToken, accessPayload, err := utils.CreateToken(accessTokenID, userDetail.ID, userDetail.Email, s.cfg.AccessTokenDuration, s.cfg.JWTSecretKey)
	if err != nil {
		return nil, fmt.Errorf("failed to create access token: %w", err)
	}

	refreshTokenID, err := uuid.NewRandom()
	if err != nil {
		return nil, fmt.Errorf("failed to create refresh token id: %w", err)
	}
	refreshToken, refreshPayload, err := utils.CreateToken(refreshTokenID, userDetail.ID, userDetail.Email, s.cfg.RefreshTokenDuration, s.cfg.JWTRefreshSecretKey)
	if err != nil {
		return nil, fmt.Errorf("failed to create refresh token: %w", err)
	}

	session := UserSession{
		ID:           accessPayload.ID,
		UserID:       userDetail.ID,
		RefreshToken: refreshToken,
		UserAgent:    userAgent,
		ClientIP:     clientIP,
		isBlocked:    false,
		ExpiresAt:    refreshPayload.ExpiresAt.Time,
	}

	err = s.authRepo.CreateSession(ctx, session)
	if err != nil {
		return nil, fmt.Errorf("failed to create user session: %w", err)
	}

	res := &SignInResponse{
		Success: true,
		Message: "successful",
		Data: SignInData{
			ID:          userDetail.ID,
			DisplayName: userDetail.DisplayName,
			Email:       userDetail.Email,
			AvatarURL:   userDetail.AvatarURL,
			Session: AuthSession{
				ID:                    session.ID,
				AccessToken:           accessToken,
				AccessTokenExpiresAt:  accessPayload.ExpiresAt.Time,
				RefreshToken:          refreshToken,
				RefreshTokenExpiresAt: refreshPayload.ExpiresAt.Time,
			},
		},
	}

	return res, nil
}

func NewService(tx db.WithTx, cfg *config.Config, authRepo Repository, userRepo user.Repository) Service {
	return &service{
		tx:       tx,
		cfg:      cfg,
		authRepo: authRepo,
		userRepo: userRepo,
	}
}
