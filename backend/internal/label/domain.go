package label

import (
	"errors"
	"regexp"
	"time"

	"github.com/google/uuid"
)

type Label struct {
	ID        uuid.UUID
	BoardID   uuid.UUID
	Name      string
	Color     string
	CreatedAt time.Time
}

var (
	ErrLabelNotFound = errors.New("label not found")
	ErrDuplicateName = errors.New("a label with this name already exists on the board")
	ErrInvalidColor  = errors.New("color must be a hex color code like #FF5733")
)

var colorRegex = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)

func ValidateColor(color string) error {
	if !colorRegex.MatchString(color) {
		return ErrInvalidColor
	}
	return nil
}
