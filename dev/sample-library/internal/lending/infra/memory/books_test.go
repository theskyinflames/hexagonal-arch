package memory_test

import (
	"testing"

	"github.com/acme/library/internal/lending/infra/booktest"
	"github.com/acme/library/internal/lending/infra/memory"
)

func TestBooksContract(t *testing.T) {
	booktest.Run(t, func(*testing.T) booktest.Store { return &memory.Books{} })
}
