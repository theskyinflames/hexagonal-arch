# Hexagonal patterns — worked example

A complete, compiling service (module `github.com/acme/library`, Go 1.27) with
one bounded context, `lending`, built without the CQRS platform. Copy the shape,
not the names. Every snippet was verified with `go vet` and `go test`. On
Go < 1.27 the import is `github.com/google/uuid`; write `uuid.Must(uuid.NewV7())`
for `uuid.NewV7()` and `uuid.Nil` for `uuid.Nil()`.

With the CQRS platform (`internal/platform/cqrs`), use cases are commands and
queries: take sections 1–4 from the cqrs-eda skill's patterns instead, and keep
sections 5–10 here, except that driving adapters call `cqrs.Send`/`cqrs.Ask`
instead of use-case interfaces.

## Contents

1. Domain: entity, domain errors, repository port
2. Write use cases, and a driven port to another system
3. Read use case and read port
4. Testing a use case with fakes
5. Driven adapter: in-memory storage
6. Driven adapter: PostgreSQL
7. Contract test shared by every storage adapter
8. Driven adapter: remote service (anti-corruption layer)
9. Driving adapter: HTTP
10. Composition root (`cmd/<service>/main.go`)

## 1. Domain: entity, domain errors, repository port

Pure Go. The domain declares the ports it needs in its own language.

`internal/lending/domain/book.go`

```go
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
```

## 2. Write use cases, and a driven port to another system

One type per use case with a `Handle` method: the driving ports. Ports that only use cases need (other systems, read models) are declared in `app`.

`internal/lending/app/lend_book.go`

```go
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
```

`internal/lending/app/add_book.go`

```go
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
```

## 3. Read use case and read port

Reads return views, never entities.

`internal/lending/app/get_book.go`

```go
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
```

## 4. Testing a use case with fakes

Hand-written fakes of the driven ports, table-driven, asserting on errors and effects.

`internal/lending/app/lend_book_test.go`

```go
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
```

## 5. Driven adapter: in-memory storage

Stores records, not entities, and rebuilds the entity on load. Compile-time assertions prove it fits the ports.

`internal/lending/infra/memory/books.go`

```go
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
```

## 6. Driven adapter: PostgreSQL

SQL, NULLs and driver errors stop here; callers get domain values and `domain.ErrNotFound`. Register the driver (e.g. `_ "github.com/jackc/pgx/v5/stdlib"`) in `main`.

`internal/lending/infra/postgres/books.go`

```go
// Package postgres is the database/sql driven adapter. Its tests run
// booktest.Run against a real database, e.g. with testcontainers-go.
package postgres

import (
	"context"
	"database/sql"
	"errors"
	"uuid"

	"github.com/acme/library/internal/lending/app"
	"github.com/acme/library/internal/lending/domain"
)

var (
	_ domain.BookRepository = Books{}
	_ app.BookReader        = Books{}
)

// Books maps between rows and the domain. SQL, NULLs and driver errors stop here.
type Books struct{ DB *sql.DB }

func (r Books) Save(ctx context.Context, b *domain.Book) error {
	_, err := r.DB.ExecContext(ctx,
		`INSERT INTO books (id, title, lent_to) VALUES ($1, $2, $3)
		 ON CONFLICT (id) DO UPDATE SET title = EXCLUDED.title, lent_to = EXCLUDED.lent_to`,
		b.ID().String(), b.Title(), nullable(b.LentTo()))
	return err
}

func (r Books) ByID(ctx context.Context, id uuid.UUID) (*domain.Book, error) {
	title, lentTo, err := r.row(ctx, id)
	if err != nil {
		return nil, err
	}
	return domain.RestoreBook(id, title, lentTo), nil
}

func (r Books) BookByID(ctx context.Context, id uuid.UUID) (app.BookView, error) {
	title, lentTo, err := r.row(ctx, id)
	if err != nil {
		return app.BookView{}, err
	}
	return app.BookView{ID: id, Title: title, Available: lentTo == uuid.Nil()}, nil
}

func (r Books) row(ctx context.Context, id uuid.UUID) (string, uuid.UUID, error) {
	var (
		title  string
		lentTo sql.NullString
	)
	err := r.DB.QueryRowContext(ctx,
		`SELECT title, lent_to FROM books WHERE id = $1`, id.String()).Scan(&title, &lentTo)
	if errors.Is(err, sql.ErrNoRows) {
		return "", uuid.Nil(), domain.ErrNotFound // callers never see sql errors
	}
	if err != nil {
		return "", uuid.Nil(), err
	}
	if !lentTo.Valid {
		return title, uuid.Nil(), nil
	}
	member, err := uuid.Parse(lentTo.String)
	return title, member, err
}

func nullable(id uuid.UUID) sql.NullString {
	return sql.NullString{String: id.String(), Valid: id != uuid.Nil()}
}
```

## 7. Contract test shared by every storage adapter

One suite per port; each adapter's test calls it. For postgres, run the same `booktest.Run` against a real database (testcontainers-go, or skip when no database URL is set).

`internal/lending/infra/booktest/contract.go`

```go
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
```

`internal/lending/infra/memory/books_test.go`

```go
package memory_test

import (
	"testing"

	"github.com/acme/library/internal/lending/infra/booktest"
	"github.com/acme/library/internal/lending/infra/memory"
)

func TestBooksContract(t *testing.T) {
	booktest.Run(t, func(*testing.T) booktest.Store { return &memory.Books{} })
}
```

## 8. Driven adapter: remote service (anti-corruption layer)

The remote API's paths, payloads and status codes stop here. Test it against `httptest.Server`.

`internal/lending/infra/membersapi/client.go`

```go
// Package membersapi is a driven adapter for the remote members service. It
// is an anti-corruption layer: the remote API's shapes and status codes stop
// here, and lending only sees app.MemberDirectory.
package membersapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"uuid"

	"github.com/acme/library/internal/lending/app"
	"github.com/acme/library/internal/lending/domain"
)

var _ app.MemberDirectory = Client{}

type Client struct {
	BaseURL string
	HTTP    *http.Client // set a Timeout: the port has no deadline of its own
}

// member is the remote representation. It never leaves this package.
type member struct {
	Status string `json:"status"`
}

func (c Client) IsActive(ctx context.Context, id uuid.UUID) (bool, error) {
	u, err := url.JoinPath(c.BaseURL, "members", id.String())
	if err != nil {
		return false, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return false, err
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return false, fmt.Errorf("members service: %w", err)
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK:
	case http.StatusNotFound:
		return false, domain.ErrUnknownMember
	default:
		return false, fmt.Errorf("members service: unexpected status %d", resp.StatusCode)
	}
	var m member
	if err := json.NewDecoder(resp.Body).Decode(&m); err != nil {
		return false, fmt.Errorf("members service: decode: %w", err)
	}
	return m.Status == "active", nil
}
```

`internal/lending/infra/membersapi/client_test.go`

```go
package membersapi_test

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"uuid"

	"github.com/acme/library/internal/lending/domain"
	"github.com/acme/library/internal/lending/infra/membersapi"
)

// The remote service is faked at the HTTP level, so the test covers the
// translation the adapter exists for: paths, status codes, payloads.
func TestClientIsActive(t *testing.T) {
	tests := []struct {
		name       string
		status     int
		body       string
		wantActive bool
		wantErr    error
		wantAnyErr bool
	}{
		{name: "active member", status: 200, body: `{"status":"active"}`, wantActive: true},
		{name: "suspended member", status: 200, body: `{"status":"suspended"}`},
		{name: "unknown member", status: 404, wantErr: domain.ErrUnknownMember},
		{name: "remote failure", status: 503, wantAnyErr: true},
		{name: "bad payload", status: 200, body: `{`, wantAnyErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			id := uuid.NewV7()
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/members/"+id.String() {
					t.Errorf("path = %s", r.URL.Path)
				}
				w.WriteHeader(tt.status)
				_, _ = w.Write([]byte(tt.body))
			}))
			defer srv.Close()
			c := membersapi.Client{BaseURL: srv.URL, HTTP: srv.Client()}

			active, err := c.IsActive(t.Context(), id)

			if tt.wantAnyErr {
				if err == nil {
					t.Fatal("want an error")
				}
				return
			}
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}
			if active != tt.wantActive {
				t.Fatalf("active = %v, want %v", active, tt.wantActive)
			}
		})
	}
}
```

## 9. Driving adapter: HTTP

Decode → use case → map errors → encode. The adapter declares the use cases it needs as small interfaces, owns its JSON shapes, and maps every domain error in one place.

`internal/lending/infra/httpapi/handlers.go`

```go
// Package httpapi is the HTTP driving adapter: decode, call a use case, map
// errors, encode. JSON shapes and status codes live here, not in app or domain.
package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"uuid"

	"github.com/acme/library/internal/lending/app"
	"github.com/acme/library/internal/lending/domain"
)

// The adapter declares the use cases it needs as small interfaces, so its
// tests can use fakes. *Handler types from app satisfy them.
type (
	BookAdder interface {
		Handle(ctx context.Context, in app.AddBook) error
	}
	BookLender interface {
		Handle(ctx context.Context, in app.LendBook) error
	}
	BookGetter interface {
		Handle(ctx context.Context, in app.GetBook) (app.BookView, error)
	}
)

type UseCases struct {
	AddBook  BookAdder
	LendBook BookLender
	GetBook  BookGetter
}

// Transport shapes. They are separate from app.BookView so the API can
// evolve without touching the core.
type (
	addBookRequest struct {
		Title string `json:"title"`
	}
	lendBookRequest struct {
		MemberID string `json:"member_id"`
	}
	bookResponse struct {
		ID        string `json:"id"`
		Title     string `json:"title"`
		Available bool   `json:"available"`
	}
)

func Routes(mux *http.ServeMux, uc UseCases, log *slog.Logger) {
	mux.HandleFunc("POST /books", func(w http.ResponseWriter, r *http.Request) {
		var in addBookRequest
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			http.Error(w, "invalid JSON", http.StatusBadRequest)
			return
		}
		id := uuid.NewV7()
		if err := uc.AddBook.Handle(r.Context(), app.AddBook{BookID: id, Title: in.Title}); err != nil {
			writeError(w, r, log, err)
			return
		}
		w.Header().Set("Location", "/books/"+id.String())
		w.WriteHeader(http.StatusCreated)
	})

	mux.HandleFunc("POST /books/{id}/loans", func(w http.ResponseWriter, r *http.Request) {
		bookID, err := uuid.Parse(r.PathValue("id"))
		if err != nil {
			http.Error(w, "invalid book id", http.StatusBadRequest)
			return
		}
		var in lendBookRequest
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			http.Error(w, "invalid JSON", http.StatusBadRequest)
			return
		}
		memberID, err := uuid.Parse(in.MemberID)
		if err != nil {
			http.Error(w, "invalid member_id", http.StatusBadRequest)
			return
		}
		if err := uc.LendBook.Handle(r.Context(), app.LendBook{BookID: bookID, MemberID: memberID}); err != nil {
			writeError(w, r, log, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})

	mux.HandleFunc("GET /books/{id}", func(w http.ResponseWriter, r *http.Request) {
		id, err := uuid.Parse(r.PathValue("id"))
		if err != nil {
			http.Error(w, "invalid book id", http.StatusBadRequest)
			return
		}
		v, err := uc.GetBook.Handle(r.Context(), app.GetBook{BookID: id})
		if err != nil {
			writeError(w, r, log, err)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(bookResponse{ID: v.ID.String(), Title: v.Title, Available: v.Available})
	})
}

// writeError maps every domain error to a status code in one place. The
// adapter is the boundary, so it logs unexpected errors once, here.
func writeError(w http.ResponseWriter, r *http.Request, log *slog.Logger, err error) {
	switch {
	case errors.Is(err, domain.ErrNotFound):
		http.Error(w, err.Error(), http.StatusNotFound)
	case errors.Is(err, domain.ErrEmptyTitle):
		http.Error(w, err.Error(), http.StatusBadRequest)
	case errors.Is(err, domain.ErrAlreadyLent),
		errors.Is(err, domain.ErrInactive),
		errors.Is(err, domain.ErrUnknownMember):
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
	default:
		log.ErrorContext(r.Context(), "request failed", "method", r.Method, "path", r.URL.Path, "error", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
	}
}
```

`internal/lending/infra/httpapi/handlers_test.go`

```go
package httpapi_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"uuid"

	"github.com/acme/library/internal/lending/app"
	"github.com/acme/library/internal/lending/domain"
	"github.com/acme/library/internal/lending/infra/httpapi"
)

// fakeLender stands in for the use case: the adapter test covers decoding,
// error mapping and encoding, not business rules.
type fakeLender struct {
	got app.LendBook
	err error
}

func (f *fakeLender) Handle(_ context.Context, in app.LendBook) error {
	f.got = in
	return f.err
}

func TestLendBookRoute(t *testing.T) {
	bookID, memberID := uuid.NewV7(), uuid.NewV7()
	tests := []struct {
		name       string
		path       string
		body       string
		ucErr      error
		wantStatus int
	}{
		{name: "lends", path: "/books/" + bookID.String() + "/loans", body: `{"member_id":"` + memberID.String() + `"}`, wantStatus: http.StatusNoContent},
		{name: "bad book id", path: "/books/nope/loans", body: `{}`, wantStatus: http.StatusBadRequest},
		{name: "bad member id", path: "/books/" + bookID.String() + "/loans", body: `{"member_id":"nope"}`, wantStatus: http.StatusBadRequest},
		{name: "missing book", path: "/books/" + bookID.String() + "/loans", body: `{"member_id":"` + memberID.String() + `"}`, ucErr: domain.ErrNotFound, wantStatus: http.StatusNotFound},
		{name: "already lent", path: "/books/" + bookID.String() + "/loans", body: `{"member_id":"` + memberID.String() + `"}`, ucErr: domain.ErrAlreadyLent, wantStatus: http.StatusUnprocessableEntity},
		{name: "wrapped domain error", path: "/books/" + bookID.String() + "/loans", body: `{"member_id":"` + memberID.String() + `"}`, ucErr: errors.Join(errors.New("check member"), domain.ErrUnknownMember), wantStatus: http.StatusUnprocessableEntity},
		{name: "unexpected error", path: "/books/" + bookID.String() + "/loans", body: `{"member_id":"` + memberID.String() + `"}`, ucErr: errors.New("db down"), wantStatus: http.StatusInternalServerError},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lender := &fakeLender{err: tt.ucErr}
			mux := http.NewServeMux()
			httpapi.Routes(mux, httpapi.UseCases{LendBook: lender}, slog.New(slog.NewTextHandler(io.Discard, nil)))

			rec := httptest.NewRecorder()
			mux.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, tt.path, strings.NewReader(tt.body)))

			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d (%s)", rec.Code, tt.wantStatus, rec.Body)
			}
			if tt.wantStatus == http.StatusNoContent && lender.got != (app.LendBook{BookID: bookID, MemberID: memberID}) {
				t.Fatalf("use case got %+v", lender.got)
			}
		})
	}
}
```

## 10. Composition root

The only place that knows the concrete adapters. Config is read here and passed down as values.

`cmd/library/main.go`

```go
package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/acme/library/internal/lending/app"
	"github.com/acme/library/internal/lending/infra/httpapi"
	"github.com/acme/library/internal/lending/infra/membersapi"
	"github.com/acme/library/internal/lending/infra/postgres"
)

func main() {
	if err := run(); err != nil {
		slog.Error("library stopped", "error", err)
		os.Exit(1)
	}
}

// config is read here and nowhere else; adapters receive plain values.
type config struct {
	addr       string
	dbURL      string
	membersURL string
}

func loadConfig() (config, error) {
	c := config{
		addr:       os.Getenv("ADDR"),
		dbURL:      os.Getenv("DATABASE_URL"),
		membersURL: os.Getenv("MEMBERS_URL"),
	}
	if c.addr == "" {
		c.addr = ":8080"
	}
	if c.dbURL == "" || c.membersURL == "" {
		return c, errors.New("DATABASE_URL and MEMBERS_URL are required")
	}
	return c, nil
}

// run is the composition root: the only place that knows the concrete
// adapters. It builds driven adapters, injects them into use cases and
// hands the use cases to the driving adapters.
func run() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	cfg, err := loadConfig()
	if err != nil {
		return err
	}
	db, err := sql.Open("pgx", cfg.dbURL)
	if err != nil {
		return fmt.Errorf("open db: %w", err)
	}
	defer db.Close()

	// Driven adapters.
	books := postgres.Books{DB: db}
	members := membersapi.Client{BaseURL: cfg.membersURL, HTTP: &http.Client{Timeout: 3 * time.Second}}

	// Use cases.
	uc := httpapi.UseCases{
		AddBook:  app.AddBookHandler{Books: books},
		LendBook: app.LendBookHandler{Books: books, Members: members},
		GetBook:  app.GetBookHandler{Books: books},
	}

	// Driving adapters.
	mux := http.NewServeMux()
	httpapi.Routes(mux, uc, log)
	srv := &http.Server{Addr: cfg.addr, Handler: mux, ReadHeaderTimeout: 5 * time.Second}

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
	}()
	log.Info("listening", "addr", cfg.addr)
	if err := srv.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}
```
