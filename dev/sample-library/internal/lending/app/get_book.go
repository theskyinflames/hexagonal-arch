package app

import (
	"context"
	"uuid"
)

// GetBook is a read use case. It returns a view shaped for callers, never
// the entity, so adapters can't change state through it.
type GetBook struct{ BookID uuid.UUID }

type BookView struct {
	ID        uuid.UUID
	Title     string
	Available bool
}

// BookReader is the read-side driven port. It may query storage directly
// and skip the entity: reads need no invariants.
type BookReader interface {
	// BookByID returns domain.ErrNotFound when the book doesn't exist.
	BookByID(ctx context.Context, id uuid.UUID) (BookView, error)
}

type GetBookHandler struct{ Books BookReader }

func (h GetBookHandler) Handle(ctx context.Context, in GetBook) (BookView, error) {
	return h.Books.BookByID(ctx, in.BookID)
}
