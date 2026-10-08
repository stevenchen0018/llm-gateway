package admin

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/stevenchen/llm-gateway/internal/domain"
)

func run(err error) *httptest.ResponseRecorder {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	writeAdminError(c, err)
	return w
}

func TestSchemaMismatchIsActionableAndDoesNotLeakSQL(t *testing.T) {
	raw := fmt.Errorf("list providers: %w", &pgconn.PgError{Code: "42P01", Message: `relation "providers" does not exist`})
	w := run(raw)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", w.Code)
	}
	body := w.Body.String()
	if strings.Contains(body, "relation") || strings.Contains(body, "SQLSTATE") {
		t.Errorf("raw SQL error leaked to client: %s", body)
	}
	if !strings.Contains(body, "migrate") {
		t.Errorf("response should tell the operator how to fix it: %s", body)
	}
}

func TestMissingColumnIsAlsoASchemaProblem(t *testing.T) {
	w := run(fmt.Errorf("x: %w", &pgconn.PgError{Code: "42703", Message: `column "category" does not exist`}))
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", w.Code)
	}
}

func TestUnknownErrorsAreSanitized(t *testing.T) {
	w := run(errors.New("dial tcp 10.0.0.5:5432: connect: connection refused"))
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", w.Code)
	}
	if strings.Contains(w.Body.String(), "10.0.0.5") {
		t.Errorf("internal detail leaked: %s", w.Body.String())
	}
}

func TestDomainErrorsKeepTheirStatus(t *testing.T) {
	if got := run(domain.ErrNotFound).Code; got != http.StatusNotFound {
		t.Errorf("not found → %d", got)
	}
	if got := run(fmt.Errorf("%w: bad", domain.ErrInvalidArgument)).Code; got != http.StatusBadRequest {
		t.Errorf("invalid argument → %d", got)
	}
}
