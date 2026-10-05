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
