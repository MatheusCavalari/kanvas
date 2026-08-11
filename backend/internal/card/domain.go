package card

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

type Column struct {
	ID        uuid.UUID
	BoardID   uuid.UUID
	Title     string
	Position  int32
	CreatedAt time.Time
	UpdatedAt time.Time
}

type Card struct {
	ID          uuid.UUID
	ColumnID    uuid.UUID
	Title       string
	Description string
	Position    int32
	AssigneeID  *uuid.UUID
	DueDate     *time.Time
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

var (
	ErrColumnNotFound   = errors.New("column not found")
	ErrCardNotFound     = errors.New("card not found")
	ErrAssigneeNotFound = errors.New("assignee is not a registered user")
)
