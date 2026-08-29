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
	role sql.NullString
	err  error
}

func (s stubMembershipChecker) GetProjectMembership(_ context.Context, _ db.GetProjectMembershipParams) (sql.NullString, error) {
	return s.role, s.err
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
	he := doProjectMembership(t, stubMembershipChecker{role: sql.NullString{String: "member", Valid: true}}, "user-1")
	if he != nil {
		t.Fatalf("expected member to pass, got %+v", he)
	}
}

func TestProjectMembershipRejectsNonMember(t *testing.T) {
	he := doProjectMembership(t, stubMembershipChecker{role: sql.NullString{}}, "user-2")
	if he == nil || he.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for non-member, got %+v", he)
	}
}

func TestProjectMembershipRejectsMissingProject(t *testing.T) {
	he := doProjectMembership(t, stubMembershipChecker{err: sql.ErrNoRows}, "user-1")
	if he == nil || he.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for missing project, got %+v", he)
	}
}

func TestProjectMembershipRejectsUnauthenticated(t *testing.T) {
	he := doProjectMembership(t, stubMembershipChecker{role: sql.NullString{String: "member", Valid: true}}, "")
	if he == nil || he.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 when user ID missing from context, got %+v", he)
	}
}
