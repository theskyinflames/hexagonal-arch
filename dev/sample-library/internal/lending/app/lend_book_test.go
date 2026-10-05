package app_test

import (
	"context"
	"errors"
	"testing"
	"uuid"

	"github.com/acme/library/internal/lending/app"
	"github.com/acme/library/internal/lending/domain"
)

// Hand-written fakes of the driven ports: no adapters, no mocking library.
type fakeBooks struct {
	book  *domain.Book
	saved *domain.Book
	err   error
}

func (f *fakeBooks) ByID(context.Context, uuid.UUID) (*domain.Book, error) {
	if f.book == nil {
		return nil, domain.ErrNotFound
	}
	return f.book, nil
}

func (f *fakeBooks) Save(_ context.Context, b *domain.Book) error {
	f.saved = b
	return f.err
}

type fakeMembers struct {
	active bool
	err    error
}

func (f fakeMembers) IsActive(context.Context, uuid.UUID) (bool, error) { return f.active, f.err }

func TestLendBookHandler(t *testing.T) {
	member := uuid.NewV7()
	available := func() *domain.Book { return domain.RestoreBook(uuid.NewV7(), "Dune", uuid.Nil()) }
	errDB := errors.New("db down")

	tests := []struct {
		name      string
		book      *domain.Book
		members   fakeMembers
		saveErr   error
		wantErr   error
		wantSaved bool
	}{
		{name: "lends an available book", book: available(), members: fakeMembers{active: true}, wantSaved: true},
		{name: "rejects an inactive member", book: available(), members: fakeMembers{}, wantErr: domain.ErrInactive},
		{name: "rejects an unknown member", book: available(), members: fakeMembers{err: domain.ErrUnknownMember}, wantErr: domain.ErrUnknownMember},
		{name: "rejects a missing book", members: fakeMembers{active: true}, wantErr: domain.ErrNotFound},
		{name: "rejects a lent book", book: domain.RestoreBook(uuid.NewV7(), "Dune", uuid.NewV7()), members: fakeMembers{active: true}, wantErr: domain.ErrAlreadyLent},
		{name: "fails when save fails", book: available(), members: fakeMembers{active: true}, saveErr: errDB, wantErr: errDB, wantSaved: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			books := &fakeBooks{book: tt.book, err: tt.saveErr}
			h := app.LendBookHandler{Books: books, Members: tt.members}

			err := h.Handle(t.Context(), app.LendBook{BookID: uuid.NewV7(), MemberID: member})

			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}
			if got := books.saved != nil; got != tt.wantSaved {
				t.Fatalf("saved = %v, want %v", got, tt.wantSaved)
			}
		})
	}
}
