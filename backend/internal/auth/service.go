package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

type TokenIssuer interface {
	IssueAccessToken(userID uuid.UUID) (string, error)
}

type Service struct {
	repo       Repository
	tokens     TokenIssuer
	refreshTTL time.Duration
	now        func() time.Time
}

func NewService(repo Repository, tokens TokenIssuer, refreshTTL time.Duration) *Service {
	return &Service{repo: repo, tokens: tokens, refreshTTL: refreshTTL, now: time.Now}
}

type AuthResult struct {
	User             User
	AccessToken      string
	RefreshToken     string
	RefreshExpiresAt time.Time
}

func (s *Service) Register(ctx context.Context, name, email, password string) (AuthResult, error) {
	email = strings.ToLower(strings.TrimSpace(email))

	if _, err := s.repo.GetUserByEmail(ctx, email); err == nil {
		return AuthResult{}, ErrEmailTaken
	} else if !errors.Is(err, ErrNotFound) {
		return AuthResult{}, err
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return AuthResult{}, err
	}

	user, err := s.repo.CreateUser(ctx, User{
		ID:           uuid.New(),
		Name:         name,
		Email:        email,
		PasswordHash: string(hash),
	})
	if err != nil {
		if errors.Is(err, ErrEmailTaken) {
			return AuthResult{}, ErrEmailTaken
		}
		return AuthResult{}, err
	}

	return s.issueAuthResult(ctx, user)
}

func (s *Service) issueAuthResult(ctx context.Context, user User) (AuthResult, error) {
	accessToken, err := s.tokens.IssueAccessToken(user.ID)
	if err != nil {
		return AuthResult{}, err
	}

	rawRefreshToken, err := generateRefreshToken()
	if err != nil {
		return AuthResult{}, err
	}

	expiresAt := s.now().Add(s.refreshTTL)
	if err := s.repo.CreateRefreshToken(ctx, RefreshToken{
		ID:        uuid.New(),
		UserID:    user.ID,
		TokenHash: hashToken(rawRefreshToken),
		ExpiresAt: expiresAt,
	}); err != nil {
		return AuthResult{}, err
	}

	return AuthResult{
		User:             user,
		AccessToken:      accessToken,
		RefreshToken:     rawRefreshToken,
		RefreshExpiresAt: expiresAt,
	}, nil
}

func generateRefreshToken() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func (s *Service) Login(ctx context.Context, email, password string) (AuthResult, error) {
	email = strings.ToLower(strings.TrimSpace(email))

	user, err := s.repo.GetUserByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return AuthResult{}, ErrInvalidCredentials
		}
		return AuthResult{}, err
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)); err != nil {
		return AuthResult{}, ErrInvalidCredentials
	}

	return s.issueAuthResult(ctx, user)
}

func (s *Service) Refresh(ctx context.Context, rawRefreshToken string) (AuthResult, error) {
	hash := hashToken(rawRefreshToken)
	stored, err := s.repo.GetRefreshTokenByHash(ctx, hash)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return AuthResult{}, ErrRefreshTokenInvalid
		}
		return AuthResult{}, err
	}

	if stored.RevokedAt != nil || s.now().After(stored.ExpiresAt) {
		return AuthResult{}, ErrRefreshTokenInvalid
	}

	if err := s.repo.RevokeRefreshToken(ctx, stored.ID); err != nil {
		return AuthResult{}, err
	}

	user, err := s.repo.GetUserByID(ctx, stored.UserID)
	if err != nil {
		return AuthResult{}, err
	}

	return s.issueAuthResult(ctx, user)
}

func (s *Service) Logout(ctx context.Context, rawRefreshToken string) error {
	hash := hashToken(rawRefreshToken)
	stored, err := s.repo.GetRefreshTokenByHash(ctx, hash)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil
		}
		return err
	}
	return s.repo.RevokeRefreshToken(ctx, stored.ID)
}

func (s *Service) UserByID(ctx context.Context, id uuid.UUID) (User, error) {
	user, err := s.repo.GetUserByID(ctx, id)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return User{}, ErrUserNotFound
		}
		return User{}, err
	}
	return user, nil
}
