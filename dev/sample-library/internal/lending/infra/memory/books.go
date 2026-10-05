// Package memory is an in-memory driven adapter for tests, demos and local runs.
package memory

import (
	"context"
	"sync"
	"uuid"

	"github.com/acme/library/internal/lending/app"
	"github.com/acme/library/internal/lending/domain"
)

// Compile-time proof that the adapter fits the ports.
var (
	_ domain.BookRepository = (*Books)(nil)
	_ app.BookReader        = (*Books)(nil)
)

// record is the stored state. Storing *domain.Book itself would share the
// pointer with callers, so unsaved changes would leak into the store.
type record struct {
	title  string
	lentTo uuid.UUID
}

type Books struct {
	mu sync.RWMutex
	m  map[uuid.UUID]record
}

func (r *Books) Save(_ context.Context, b *domain.Book) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.m == nil {
		r.m = make(map[uuid.UUID]record)
	}
	r.m[b.ID()] = record{title: b.Title(), lentTo: b.LentTo()}
	return nil
}

func (r *Books) ByID(_ context.Context, id uuid.UUID) (*domain.Book, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	rec, ok := r.m[id]
	if !ok {
		return nil, domain.ErrNotFound
	}
	return domain.RestoreBook(id, rec.title, rec.lentTo), nil
}

func (r *Books) BookByID(_ context.Context, id uuid.UUID) (app.BookView, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	rec, ok := r.m[id]
	if !ok {
		return app.BookView{}, domain.ErrNotFound
	}
	return app.BookView{ID: id, Title: rec.title, Available: rec.lentTo == uuid.Nil()}, nil
}
