package httpserver

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"

	"mealplanner/internal/config"
	"mealplanner/internal/households"
	"mealplanner/internal/view/pages"
)

func TestImportSourceStateRejectsUnsafeSignedURL(t *testing.T) {
	token, err := encodeImportSourceState("session-secret", importSourceState{
		URL: "javascript:alert(1)", HouseholdID: uuid.NewString(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := decodeImportSourceState("session-secret", token); err == nil {
		t.Fatal("signed unsafe source URL was accepted")
	}
}

func TestReconcileRejectsCommitWithAnyUnmatchedLine(t *testing.T) {
	d := pages.ImportReconcileData{
		Name: "Soup", Prep: "10", Cook: "20", Servings: "4", DefaultUnit: "g",
		Lines: []pages.ImportLine{
			{Name: "stock", Amount: "1", Unit: "cup", FoodID: uuid.NewString()},
			{Name: "salt", Amount: "1", Unit: "tsp"},
		},
	}
	form, err := reconcileToForm(d)
	if err == nil {
		t.Fatalf("partially mapped recipe was accepted: %+v", form.Components)
	}
	if len(form.Components) != 0 {
		t.Fatalf("validation returned a partial recipe: %+v", form.Components)
	}
}

func TestImportValidationRendersBadRequest(t *testing.T) {
	s := &Server{cfg: &config.Config{AppEnv: "development"}}
	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/import", strings.NewReader("format=url&input=javascript%3Aalert%281%29"))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.Set(memberCtxKey, &households.Member{Name: "Owner", Role: "owner", Initials: "OW"})
	if err := s.handleImportPost(c); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body: %s", rec.Code, rec.Body.String())
	}
}
