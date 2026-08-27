package httpserver

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/labstack/echo/v4"

	"mealplanner/internal/transfer"
	"mealplanner/internal/view/pages"
)

// handleData renders the export/import screen.
func (s *Server) handleData(c echo.Context) error {
	return s.render(c, pages.Data(pages.DataData{Member: s.member(c)}))
}

// handleDataExport streams a full-fidelity JSON archive as a download (FR15.1).
func (s *Server) handleDataExport(c echo.Context) error {
	arc, err := s.transfer.Export(c.Request().Context(), s.household(c))
	if err != nil {
		return err
	}
	body, err := json.MarshalIndent(arc, "", "  ")
	if err != nil {
		return err
	}
	filename := fmt.Sprintf("backbone-plate-%s.json", time.Now().Format("2006-01-02"))
	c.Response().Header().Set(echo.HeaderContentDisposition, fmt.Sprintf("attachment; filename=%q", filename))
	return c.Blob(http.StatusOK, echo.MIMEApplicationJSON, body)
}

// handleDataImport reads the uploaded archive, merges the chosen sections, and
// renders the run report (FR15.2–FR15.5).
func (s *Server) handleDataImport(c echo.Context) error {
	member := s.member(c)
	fail := func(status int, msg string) error {
		return s.renderStatus(c, status, pages.Data(pages.DataData{Member: member, Error: msg, Selected: formSections(c)}))
	}

	fh, err := c.FormFile("file")
	if err != nil {
		return fail(http.StatusBadRequest, "Choose a JSON backup file to import.")
	}
	f, err := fh.Open()
	if err != nil {
		return fail(http.StatusBadRequest, "Couldn’t open the uploaded file.")
	}
	defer f.Close()

	var arc transfer.Archive
	dec := json.NewDecoder(f)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&arc); err != nil {
		return fail(http.StatusBadRequest, "That file isn’t a valid Backbone Plate export: "+err.Error())
	}
	if err := arc.ValidateURLs(); err != nil {
		return fail(http.StatusUnprocessableEntity, "That archive contains an invalid URL: "+err.Error())
	}

	selected := formSections(c)
	if !anySelected(selected) {
		return fail(http.StatusUnprocessableEntity, "Pick at least one section to import.")
	}

	report, err := s.transfer.Import(c.Request().Context(), s.household(c), &arc, selected)
	if err != nil {
		return fail(http.StatusUnprocessableEntity, err.Error())
	}
	return s.render(c, pages.Data(pages.DataData{Member: member, Report: report, Selected: selected}))
}

// formSections reads the ticked import sections into a set.
func formSections(c echo.Context) map[string]bool {
	sel := map[string]bool{}
	if form, err := c.FormParams(); err == nil {
		for _, name := range form["section"] {
			sel[name] = true
		}
	}
	return sel
}

func anySelected(sel map[string]bool) bool {
	for _, v := range sel {
		if v {
			return true
		}
	}
	return false
}
