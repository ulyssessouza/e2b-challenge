package middleware

import (
	"context"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"

	"e2b-challenge/internal/db"
)

type stubMembershipChecker struct {
	member bool
	owner  bool
	noRows bool
	err    error
}

func (s stubMembershipChecker) GetProjectMembership(_ context.Context, _ db.GetProjectMembershipParams) (db.GetProjectMembershipRow, error) {
	if s.err != nil {
		return db.GetProjectMembershipRow{}, s.err
	}
	if s.noRows {
		return db.GetProjectMembershipRow{}, sql.ErrNoRows
	}
	return db.GetProjectMembershipRow{
		MemberUserID: sql.NullString{String: "u", Valid: s.member},
		IsOwner:      s.owner,
	}, nil
}

func doProjectMembership(t *testing.T, q MembershipChecker, userID string) *echo.HTTPError {
	t.Helper()
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/v1/projects/some-id", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues("some-id")
	if userID != "" {
		c.Set(ContextUserID, userID)
	}

	called := false
	err := ProjectMembership(q)(func(c echo.Context) error {
		called = true
		return nil
	})(c)
	if err == nil {
		if !called {
			t.Fatal("next handler was not called")
		}
		return nil
	}
	he, ok := err.(*echo.HTTPError)
	if !ok {
		t.Fatalf("expected *echo.HTTPError, got %T: %v", err, err)
	}
	return he
}

func TestProjectMembershipAllowsMember(t *testing.T) {
	he := doProjectMembership(t, stubMembershipChecker{member: true}, "user-1")
	if he != nil {
		t.Fatalf("expected member to pass, got %+v", he)
	}
}

func TestProjectMembershipRejectsNonMember(t *testing.T) {
	he := doProjectMembership(t, stubMembershipChecker{}, "user-2")
	if he == nil || he.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for non-member, got %+v", he)
	}
}

func TestProjectMembershipRejectsMissingProject(t *testing.T) {
	he := doProjectMembership(t, stubMembershipChecker{noRows: true}, "user-1")
	if he == nil || he.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for missing project, got %+v", he)
	}
}

func TestProjectMembershipAllowsOwner(t *testing.T) {
	he := doProjectMembership(t, stubMembershipChecker{member: true, owner: true}, "user-1")
	if he != nil {
		t.Fatalf("expected owner to pass, got %+v", he)
	}
}

func TestProjectMembershipRejectsUnauthenticated(t *testing.T) {
	he := doProjectMembership(t, stubMembershipChecker{member: true}, "")
	if he == nil || he.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 when user ID missing from context, got %+v", he)
	}
}
