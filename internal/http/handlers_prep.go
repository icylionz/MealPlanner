package http

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"

	"github.com/icylionz/mealplanner/internal/service"
	"github.com/icylionz/mealplanner/internal/view"
)

func (s *Server) prepListView(c echo.Context, status int, errs []string) error {
	sessions, err := s.svc.Prep.Sessions(c.Request().Context(), actor(c).HouseholdID)
	if err != nil {
		return err
	}
	return s.render(c, status, view.PrepListPage(sessions, s.today0(), errs))
}

func (s *Server) prepList(c echo.Context) error { return s.prepListView(c, http.StatusOK, nil) }

func (s *Server) createPrep(c echo.Context) error {
	date, _ := parseDate(c.FormValue("date"))
	id, err := s.svc.Prep.Create(c.Request().Context(), actor(c), c.FormValue("name"), date)
	if msgs, ok := problems(err); ok {
		return s.prepListView(c, http.StatusUnprocessableEntity, msgs)
	}
	if err != nil {
		return err
	}
	return s.redirect(c, "/prep/"+id.String())
}

func (s *Server) prepView(c echo.Context, status int, id uuid.UUID, errs []string) error {
	d, err := s.svc.Prep.Detail(c.Request().Context(), actor(c).HouseholdID, id)
	if err != nil {
		return err
	}
	return s.render(c, status, view.PrepDetailPage(view.PrepPage{D: d, Options: foodOptions(d.Catalog, ""), Errors: errs}))
}

func (s *Server) prepDetail(c echo.Context) error {
	id, err := parseID(c, "id")
	if err != nil {
		return err
	}
	return s.prepView(c, http.StatusOK, id, nil)
}

func (s *Server) prepPrint(c echo.Context) error {
	id, err := parseID(c, "id")
	if err != nil {
		return err
	}
	d, err := s.svc.Prep.Detail(c.Request().Context(), actor(c).HouseholdID, id)
	if err != nil {
		return err
	}
	return s.ok(c, view.PrepPrintPage(d))
}

func (s *Server) prepResult(c echo.Context, id uuid.UUID, err error, msg string) error {
	if msgs, ok := problems(err); ok {
		return s.prepView(c, http.StatusUnprocessableEntity, id, msgs)
	}
	if err != nil {
		return err
	}
	if msg != "" {
		s.flash(c, "ok", msg)
	}
	return s.redirect(c, "/prep/"+id.String())
}

func (s *Server) updatePrep(c echo.Context) error {
	id, err := parseID(c, "id")
	if err != nil {
		return err
	}
	date, _ := parseDate(c.FormValue("date"))
	return s.prepResult(c, id, s.svc.Prep.Update(c.Request().Context(), actor(c), id, c.FormValue("name"), date), "Session saved.")
}

func (s *Server) deletePrep(c echo.Context) error {
	id, err := parseID(c, "id")
	if err != nil {
		return err
	}
	if err := s.svc.Prep.Delete(c.Request().Context(), actor(c), id); err != nil {
		return err
	}
	s.flash(c, "ok", "Prep session deleted.")
	return s.redirect(c, "/prep")
}

func (s *Server) setPrepFood(c echo.Context) error {
	id, err := parseID(c, "id")
	if err != nil {
		return err
	}
	servings, _ := parseFloat(c.FormValue("servings"))
	foodID, perr := uuid.Parse(c.FormValue("food_id"))
	if perr != nil {
		cat, err := s.catalog(c)
		if err != nil {
			return err
		}
		f := cat.FindByName(c.FormValue("food_name"))
		if f == nil {
			return s.prepView(c, http.StatusUnprocessableEntity, id, []string{"Choose a food from the suggestions."})
		}
		foodID = f.ID
	}
	return s.prepResult(c, id, s.svc.Prep.SetFood(c.Request().Context(), actor(c), id, foodID, servings), "")
}

func (s *Server) removePrepFood(c echo.Context) error {
	id, err := parseID(c, "id")
	if err != nil {
		return err
	}
	foodID, err := parseID(c, "food")
	if err != nil {
		return err
	}
	return s.prepResult(c, id, s.svc.Prep.RemoveFood(c.Request().Context(), actor(c), id, foodID), "")
}

// Data import/export ---------------------------------------------------------

func (s *Server) dataPage(c echo.Context) error {
	return s.ok(c, view.DataPageView(view.DataPage{}))
}

func (s *Server) export(c echo.Context) error {
	sections := c.QueryParams()["section"]
	if len(sections) == 0 {
		sections = service.AllSections
	}
	f, err := s.svc.Transfer.Export(c.Request().Context(), actor(c), sections)
	if err != nil {
		return err
	}
	name := fmt.Sprintf("mealplanner-export-%s.json", time.Now().In(s.cfg.Timezone).Format("20060102"))
	c.Response().Header().Set("Content-Disposition", `attachment; filename="`+name+`"`)
	c.Response().Header().Set(echo.HeaderContentType, echo.MIMEApplicationJSONCharsetUTF8)
	c.Response().WriteHeader(http.StatusOK)
	enc := json.NewEncoder(c.Response())
	enc.SetIndent("", "  ")
	return enc.Encode(f)
}

func (s *Server) importData(c echo.Context) error {
	c.Request().Body = http.MaxBytesReader(c.Response(), c.Request().Body, 32<<20)
	fh, err := c.FormFile("file")
	if err != nil {
		return s.render(c, http.StatusUnprocessableEntity, view.DataPageView(view.DataPage{Errors: []string{"Choose an export file to import."}}))
	}
	file, err := fh.Open()
	if err != nil {
		return err
	}
	defer file.Close()
	f, err := service.ParseExport(file)
	if msgs, ok := problems(err); ok {
		s.metrics.Inc("mealplanner_import_failures_total", "kind", "json")
		return s.render(c, http.StatusUnprocessableEntity, view.DataPageView(view.DataPage{Errors: msgs}))
	}
	if err != nil {
		return err
	}
	form, _ := c.MultipartForm()
	var sections []string
	if form != nil {
		sections = form.Value["section"]
	}
	rep, err := s.svc.Transfer.Import(c.Request().Context(), actor(c), f, sections)
	if msgs, ok := problems(err); ok {
		return s.render(c, http.StatusUnprocessableEntity, view.DataPageView(view.DataPage{Errors: msgs}))
	}
	if err != nil {
		return err
	}
	return s.ok(c, view.DataPageView(view.DataPage{Report: &rep}))
}
