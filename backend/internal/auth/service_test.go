package auth

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestService_Register_Success(t *testing.T) {
	repo := newFakeRepository()
	issuer := newFakeTokenIssuer()
	svc := NewService(repo, issuer, time.Hour)

	result, err := svc.Register(context.Background(), "Ada Lovelace", "ada@example.com", "supersecret")

	require.NoError(t, err)
	require.Equal(t, "ada@example.com", result.User.Email)
	require.NotEmpty(t, result.AccessToken)
	require.NotEmpty(t, result.RefreshToken)
	require.NotEqual(t, "supersecret", result.User.PasswordHash)
}

func TestService_Register_DuplicateEmail(t *testing.T) {
	repo := newFakeRepository()
	issuer := newFakeTokenIssuer()
	svc := NewService(repo, issuer, time.Hour)
	ctx := context.Background()

	_, err := svc.Register(ctx, "Ada", "ada@example.com", "supersecret")
	require.NoError(t, err)

	_, err = svc.Register(ctx, "Someone Else", "ada@example.com", "otherpass")

	require.True(t, errors.Is(err, ErrEmailTaken))
}
