package jwt

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestIssuer_IssueAndParse_RoundTrip(t *testing.T) {
	issuer := NewIssuer("test-secret", time.Hour)
	userID := uuid.New()

	token, err := issuer.IssueAccessToken(userID)
	require.NoError(t, err)
	require.NotEmpty(t, token)

	parsedID, err := issuer.ParseAccessToken(token)
	require.NoError(t, err)
	require.Equal(t, userID, parsedID)
}

func TestIssuer_ParseAccessToken_WrongSecret(t *testing.T) {
	issuer := NewIssuer("test-secret", time.Hour)
	other := NewIssuer("other-secret", time.Hour)
	userID := uuid.New()

	token, err := issuer.IssueAccessToken(userID)
	require.NoError(t, err)

	_, err = other.ParseAccessToken(token)
	require.ErrorIs(t, err, ErrInvalidToken)
}

func TestIssuer_ParseAccessToken_Expired(t *testing.T) {
	issuer := NewIssuer("test-secret", -time.Minute)
	userID := uuid.New()

	token, err := issuer.IssueAccessToken(userID)
	require.NoError(t, err)

	_, err = issuer.ParseAccessToken(token)
	require.ErrorIs(t, err, ErrInvalidToken)
}
