package comment

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

type Comment struct {
	ID        uuid.UUID
	CardID    uuid.UUID
	AuthorID  uuid.UUID
	Body      string
	CreatedAt time.Time
	UpdatedAt time.Time
}

var (
	ErrCommentNotFound = errors.New("comment not found")
	ErrNotAuthor       = errors.New("only the comment author can edit this comment")
	ErrForbidden       = errors.New("only the comment author or the board owner can delete this comment")
	ErrCardNotFound    = errors.New("card not found")
	ErrInvalidBody     = errors.New("body must be between 1 and 10000 characters")
)

const maxBodyLength = 10000

// ValidateBody checks that a comment body is non-empty and within the
// maximum allowed length.
func ValidateBody(body string) error {
	n := len([]rune(body))
	if n < 1 || n > maxBodyLength {
		return ErrInvalidBody
	}
	return nil
}
