package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
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
