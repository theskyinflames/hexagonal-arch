// Package app holds the use cases: the driving ports of the hexagon. Each use
// case is one type with a Handle method. It imports domain, never infra.
package app

import (
	"context"
	"fmt"
	"uuid"

	"github.com/acme/library/internal/lending/domain"
)

// MemberDirectory is a driven port for another system (the members service).
// It speaks lending's language; infra/membersapi translates to the remote API.
type MemberDirectory interface {
	// IsActive returns domain.ErrUnknownMember when the member doesn't exist.
	IsActive(ctx context.Context, member uuid.UUID) (bool, error)
}

// LendBook is the input of the use case: plain data, no transport types.
type LendBook struct {
	BookID   uuid.UUID
	MemberID uuid.UUID
}

// LendBookHandler orchestrates: check the member, load, call the domain, save.
// The rule itself lives in domain.Book.Lend.
type LendBookHandler struct {
	Books   domain.BookRepository
	Members MemberDirectory
}

func (h LendBookHandler) Handle(ctx context.Context, in LendBook) error {
	active, err := h.Members.IsActive(ctx, in.MemberID)
	if err != nil {
		return fmt.Errorf("check member %s: %w", in.MemberID, err)
	}
	if !active {
		return domain.ErrInactive
	}
	b, err := h.Books.ByID(ctx, in.BookID)
	if err != nil {
		return fmt.Errorf("load book %s: %w", in.BookID, err)
	}
	if err := b.Lend(in.MemberID); err != nil {
		return err
	}
	if err := h.Books.Save(ctx, b); err != nil {
		return fmt.Errorf("save book %s: %w", in.BookID, err)
	}
	return nil
}
