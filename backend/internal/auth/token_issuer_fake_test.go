package auth

import "github.com/google/uuid"

type fakeTokenIssuer struct{}

func newFakeTokenIssuer() *fakeTokenIssuer {
	return &fakeTokenIssuer{}
}

func (f *fakeTokenIssuer) IssueAccessToken(userID uuid.UUID) (string, error) {
	return "access-" + userID.String(), nil
}
