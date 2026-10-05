package app

import (
	"context"
	"fmt"
	"uuid"

	"github.com/acme/library/internal/lending/domain"
)

// AddBook takes the ID from the caller, so the caller can read the book
// afterwards without the write use case returning data.
type AddBook struct {
	BookID uuid.UUID
	Title  string
}

type AddBookHandler struct {
	Books domain.BookRepository
}

func (h AddBookHandler) Handle(ctx context.Context, in AddBook) error {
	b, err := domain.NewBook(in.BookID, in.Title)
	if err != nil {
		return err
	}
	if err := h.Books.Save(ctx, b); err != nil {
		return fmt.Errorf("save book %s: %w", in.BookID, err)
	}
	return nil
}
