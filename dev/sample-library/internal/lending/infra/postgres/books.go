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
