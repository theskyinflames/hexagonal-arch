// Package domain is the center of the lending hexagon: entities, rules,
// domain errors and the driven ports the domain needs. It imports only the
// standard library: no SQL, HTTP, JSON tags or adapter packages.
package domain

import (
	"context"
	"errors"
	"uuid"
)

// Domain errors. Adapters map them to protocol codes (driving side) and
// translate driver errors into them (driven side).
var (
	ErrNotFound      = errors.New("not found")
	ErrEmptyTitle    = errors.New("title must not be empty")
	ErrAlreadyLent   = errors.New("book is already lent")
	ErrUnknownMember = errors.New("unknown member")
	ErrInactive      = errors.New("member is not active")
)

// Book is an entity: it guards its invariants. Fields are unexported so
// state only changes through its methods.
type Book struct {
	id     uuid.UUID
	title  string
	lentTo uuid.UUID // Nil when available
}

// NewBook is the only way to create a valid Book.
func NewBook(id uuid.UUID, title string) (*Book, error) {
	if title == "" {
		return nil, ErrEmptyTitle
	}
	return &Book{id: id, title: title, lentTo: uuid.Nil()}, nil
}

// RestoreBook rebuilds a Book from stored state, for driven adapters only.
func RestoreBook(id uuid.UUID, title string, lentTo uuid.UUID) *Book {
	return &Book{id: id, title: title, lentTo: lentTo}
}

// Lend enforces the rule: a book is lent to one member at a time.
func (b *Book) Lend(member uuid.UUID) error {
	if b.lentTo != uuid.Nil() {
		return ErrAlreadyLent
	}
	b.lentTo = member
	return nil
}

func (b *Book) ID() uuid.UUID     { return b.id }
func (b *Book) Title() string     { return b.title }
func (b *Book) LentTo() uuid.UUID { return b.lentTo }

// BookRepository is a driven port. The domain declares it in its own
// language; infra/memory and infra/postgres implement it.
type BookRepository interface {
	Save(ctx context.Context, b *Book) error
	// ByID returns ErrNotFound when the book doesn't exist.
	ByID(ctx context.Context, id uuid.UUID) (*Book, error)
}
