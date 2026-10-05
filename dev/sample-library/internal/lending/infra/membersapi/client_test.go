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
