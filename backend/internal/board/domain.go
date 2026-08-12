package board

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

type Role string

const (
	RoleOwner  Role = "owner"
	RoleMember Role = "member"
)

type Board struct {
	ID        uuid.UUID
	Name      string
	OwnerID   uuid.UUID
	CreatedAt time.Time
	UpdatedAt time.Time
}

type Member struct {
	BoardID   uuid.UUID
	UserID    uuid.UUID
	Role      Role
	Name      string
	Email     string
	CreatedAt time.Time
}

var (
	ErrNotAMember         = errors.New("user is not a member of this board")
	ErrForbidden          = errors.New("only the board owner can perform this action")
	ErrAlreadyMember      = errors.New("user is already a member of this board")
	ErrMemberUserNotFound = errors.New("no user found with that email")
	ErrCannotRemoveOwner  = errors.New("cannot remove the board owner")
)
