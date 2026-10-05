// Package booktest is the contract every book storage adapter must pass.
// Each adapter's tests call Run, so memory and postgres behave the same and
// the in-memory adapter is a trustworthy stand-in for the real one.
package booktest

import (
	"errors"
	"testing"
	"uuid"

	"github.com/acme/library/internal/lending/app"
	"github.com/acme/library/internal/lending/domain"
)

// Store is what a storage adapter implements: the write and read ports.
type Store interface {
	domain.BookRepository
	app.BookReader
}

// Run checks the port contract. newStore must return an empty store.
func Run(t *testing.T, newStore func(t *testing.T) Store) {
	t.Run("missing book is ErrNotFound", func(t *testing.T) {
		s := newStore(t)
		id := uuid.NewV7()
		if _, err := s.ByID(t.Context(), id); !errors.Is(err, domain.ErrNotFound) {
			t.Fatalf("ByID err = %v, want ErrNotFound", err)
		}
		if _, err := s.BookByID(t.Context(), id); !errors.Is(err, domain.ErrNotFound) {
			t.Fatalf("BookByID err = %v, want ErrNotFound", err)
		}
	})

	t.Run("save then load round-trips state", func(t *testing.T) {
		s := newStore(t)
		b, err := domain.NewBook(uuid.NewV7(), "Dune")
		if err != nil {
			t.Fatal(err)
		}
		if err := s.Save(t.Context(), b); err != nil {
			t.Fatal(err)
		}
		got, err := s.ByID(t.Context(), b.ID())
		if err != nil {
			t.Fatal(err)
		}
		if got.Title() != "Dune" || got.LentTo() != uuid.Nil() {
			t.Fatalf("got %q lent to %v, want Dune available", got.Title(), got.LentTo())
		}
		v, err := s.BookByID(t.Context(), b.ID())
		if err != nil {
			t.Fatal(err)
		}
		if v != (app.BookView{ID: b.ID(), Title: "Dune", Available: true}) {
			t.Fatalf("view = %+v", v)
		}
	})

	t.Run("save overwrites existing state", func(t *testing.T) {
		s := newStore(t)
		b, _ := domain.NewBook(uuid.NewV7(), "Dune")
		if err := s.Save(t.Context(), b); err != nil {
			t.Fatal(err)
		}
		member := uuid.NewV7()
		if err := b.Lend(member); err != nil {
			t.Fatal(err)
		}
		if err := s.Save(t.Context(), b); err != nil {
			t.Fatal(err)
		}
		got, err := s.ByID(t.Context(), b.ID())
		if err != nil {
			t.Fatal(err)
		}
		if got.LentTo() != member {
			t.Fatalf("lent to %v, want %v", got.LentTo(), member)
		}
	})

	t.Run("loaded book doesn't alias stored state", func(t *testing.T) {
		s := newStore(t)
		b, _ := domain.NewBook(uuid.NewV7(), "Dune")
		if err := s.Save(t.Context(), b); err != nil {
			t.Fatal(err)
		}
		loaded, _ := s.ByID(t.Context(), b.ID())
		_ = loaded.Lend(uuid.NewV7()) // changed but not saved
		v, err := s.BookByID(t.Context(), b.ID())
		if err != nil {
			t.Fatal(err)
		}
		if !v.Available {
			t.Fatal("unsaved change leaked into the store")
		}
	})
}
