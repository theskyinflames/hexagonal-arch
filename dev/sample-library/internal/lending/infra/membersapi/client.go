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
