package db

import (
	"context"
	"testing"
)

func TestNewPool_InvalidDSN(t *testing.T) {
	_, err := NewPool(context.Background(), "postgres://user:pass@bad host:5432/db")

	if err == nil {
		t.Fatal("expected an error for a malformed DSN")
	}
}
