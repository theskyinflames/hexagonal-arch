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
