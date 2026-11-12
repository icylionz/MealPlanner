package ui

import (
	"fmt"
	"html"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/a-h/templ"
	"github.com/google/uuid"

	"github.com/icylionz/mealplanner/internal/core"
	"github.com/icylionz/mealplanner/internal/units"
)

// Flash is a one-shot message shown after a redirect.
type Flash struct {
	Tone string // info, success, warning, danger
	Text string
}

// Shell is what every page needs from the request: where the app is mounted,
// who is looking, and which tab is active.
type Shell struct {
	Base      string
	CSRF      string
	Title     string
	Subtitle  string
	Active    string
	Back      string // optional app path for the "back" link above the content
	BackLabel string
	Narrow    bool
	User      string
	Member    int
	Household string
	Owner     bool
	Flash     *Flash
	// Action is the screen's one main action, rendered in the header.
	Action templ.Component
}

// URL prefixes an app path with the deployment base path.
func (s Shell) URL(path string) templ.SafeURL { return templ.SafeURL(s.Base + path) }

// Path is URL as a plain string, for form actions built with query strings.
func (s Shell) Path(path string) string { return s.Base + path }

func (s Shell) pageTitle() string {
	if s.Title == "" || s.Title == "Meals" {
		return "Meals"
	}
	return s.Title + " · Meals"
}

// Icon renders a Phosphor icon inline. size is in px (22 rows and tabs, 16 chips).
func Icon(name string, size int) templ.Component {
	return templ.Raw(fmt.Sprintf(
		`<svg class="hh-icon" width="%d" height="%d" viewBox="0 0 256 256" fill="currentColor" aria-hidden="true">%s</svg>`,
		size, size, iconPaths[name]))
}

// Track renders a progress track filled to pct in the Meals app colour.
func Track(label string, pct int) templ.Component {
	if pct < 0 {
		pct = 0
	}
	if pct > 100 {
		pct = 100
	}
	return templ.Raw(fmt.Sprintf(
		`<div class="hh-track" role="progressbar" aria-valuemin="0" aria-valuemax="100" aria-valuenow="%d" aria-label="%s"><div class="hh-track__fill hh-track__fill--meals" style="width:%d%%"></div></div>`,
		pct, html.EscapeString(label), pct))
}

func memberClass(n int) string {
	if n < 1 || n > 8 {
		n = 1
	}
	return "mp-m" + strconv.Itoa(n)
}

// MemberIndex reads the stored member colour ("1".."8").
func MemberIndex(color string) int {
	n, err := strconv.Atoi(color)
	if err != nil || n < 1 || n > 8 {
		return 1
	}
	return n
}

// Person is a household member as shown beside the things they own.
type Person struct {
	Name   string
	Member int
}

// People maps account ids to household members.
type People map[uuid.UUID]Person

func (p People) of(id *uuid.UUID) (Person, bool) {
	if id == nil {
		return Person{}, false
	}
	person, ok := p[*id]
	return person, ok
}

func qty(amount float64, unit string) string {
	if amount == 0 && unit == "" {
		return ""
	}
	if unit == "" || unit == "each" {
		return units.Format(amount)
	}
	if unit == "serving" || unit == "batch" || unit == "clove" || unit == "slice" || unit == "can" || unit == "pinch" ||
		unit == "bunch" || unit == "sprig" || unit == "stick" || unit == "packet" || unit == "head" || unit == "stalk" || unit == "handful" {
		return units.Format(amount) + " " + plural(amount, unit)
	}
	return units.Format(amount) + " " + unit
}

func plural(n float64, word string) string {
	if n == 1 {
		return word
	}
	if strings.HasSuffix(word, "ch") {
		return word + "es"
	}
	if word == "leaf" {
		return "leaves"
	}
	return word + "s"
}

func count(n int, one, many string) string {
	if n == 1 {
		return "1 " + one
	}
	return strconv.Itoa(n) + " " + many
}

func num(v float64) string { return units.Format(v) }

func numPtr(v *float64) string {
	if v == nil {
		return ""
	}
	return units.Format(*v)
}

func intStr(v int) string {
	if v == 0 {
		return ""
	}
	return strconv.Itoa(v)
}

func money(v float64) string { return "$" + strconv.FormatFloat(v, 'f', 2, 64) }

func moneyPtr(v *float64) string {
	if v == nil {
		return ""
	}
	return strconv.FormatFloat(*v, 'f', 2, 64)
}

func isoDate(t time.Time) string { return t.Format("2006-01-02") }

// clock turns "18:30" into "6:30 pm".
func clock(hhmm string) string {
	t, err := time.Parse("15:04", hhmm)
	if err != nil {
		return hhmm
	}
	return strings.ToLower(t.Format("3:04 pm"))
}

func longDate(t time.Time) string  { return t.Format("Monday 2 January") }
func shortDate(t time.Time) string { return t.Format("Mon 2 Jan") }
func dayName(t time.Time) string   { return t.Format("Mon") }

func minutes(m int) string {
	if m < 60 {
		return strconv.Itoa(m) + " min"
	}
	if m%60 == 0 {
		return strconv.Itoa(m/60) + " h"
	}
	return fmt.Sprintf("%d h %d min", m/60, m%60)
}

func servesText(v float64) string { return "Serves " + units.Format(v) }

// foodMeta is the caption under a food's name in lists.
func foodMeta(f *core.Food) string {
	if !f.IsRecipe() {
		parts := []string{"Ingredient"}
		if f.Unit != "" {
			parts = append(parts, "measured in "+f.Unit)
		}
		return strings.Join(parts, " · ")
	}
	parts := []string{servesText(f.Servings), count(len(f.Components), "ingredient", "ingredients")}
	if t := f.PrepMin + f.CookMin; t > 0 {
		parts = append(parts, minutes(t))
	}
	return strings.Join(parts, " · ")
}

func mealMeta(m core.Meal, cat *core.Catalog) string {
	parts := []string{servesText(m.Servings)}
	if m.Title != "" {
		parts = append(parts, cat.Name(m.FoodID))
	}
	if n := len(m.Extras); n > 0 {
		names := make([]string, 0, n)
		for _, e := range m.Extras {
			names = append(names, cat.Name(e.FoodID))
		}
		parts = append(parts, "with "+strings.Join(names, ", "))
	}
	return strings.Join(parts, " · ")
}

func query(path string, kv ...string) string {
	v := url.Values{}
	for i := 0; i+1 < len(kv); i += 2 {
		if kv[i+1] != "" {
			v.Set(kv[i], kv[i+1])
		}
	}
	if len(v) == 0 {
		return path
	}
	return path + "?" + v.Encode()
}

func estimateText(e core.Estimate) string {
	if !e.Known() {
		return "No costs recorded yet"
	}
	if e.Partial() {
		return fmt.Sprintf("%d of %d ingredients priced, so the real cost will be higher.", e.Priced, e.Leaves)
	}
	return "Every ingredient priced."
}

func variantSuffix(v string) string {
	if v == "" {
		return ""
	}
	return ", " + v
}

func sectionName(s string) string {
	switch s {
	case "foods":
		return "Foods"
	case "plan":
		return "Planned meals"
	case "grocery":
		return "Grocery lists"
	case "prep":
		return "Prep sessions"
	}
	return s
}

// ---------- header subtitles and flash text, used by handlers ----------

func TodaySubtitle(day time.Time, household string) string {
	return longDate(day) + " · " + household
}

func WeekSubtitle(start time.Time) string {
	end := start.AddDate(0, 0, 6)
	if start.Month() == end.Month() {
		return "Week of " + start.Format("2") + " to " + end.Format("2 January")
	}
	return "Week of " + start.Format("2 January") + " to " + end.Format("2 January")
}

func MealSubtitle(m core.Meal) string { return longDate(m.Date) + ", " + clock(m.Time) }

func PrepSubtitle(day time.Time) string { return longDate(day) }

func BasketSubtitle(done, total int) string {
	if total == 0 {
		return "Nothing on it yet"
	}
	return fmt.Sprintf("%d of %s in the basket", done, count(total, "item", "items"))
}

func ItemSubtitle(amount float64, unit, variant string) string {
	return join(qty(amount, unit), variant)
}

func AddedText(n int) string {
	return count(n, "item", "items") + " added or topped up."
}
