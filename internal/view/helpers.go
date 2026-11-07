// Package view renders full HTML pages with templ. Every page goes through
// Shell so direct and hx-boost requests receive the same document.
package view

import (
	"context"
	"fmt"
	"math"
	"net/url"
	"strings"
	"time"

	"github.com/a-h/templ"
	"github.com/google/uuid"

	"github.com/icylionz/mealplanner/internal/service"
	"github.com/icylionz/mealplanner/internal/units"
)

// BasePath is set once at startup from BASE_PATH.
var BasePath string

// AssetVersion busts caches for static files.
var AssetVersion = "1"

// Path prefixes an app-local path with the base path.
func Path(p string) string { return BasePath + p }

func URL(p string) templ.SafeURL { return templ.SafeURL(BasePath + p) }

func Asset(name string) string { return BasePath + "/static/" + name + "?v=" + AssetVersion }

// Request is per-request view state placed in the context by middleware.
type Request struct {
	CSRF      string
	Principal *service.Principal
	Path      string
	Flash     string
	FlashKind string
	Now       time.Time
}

type ctxKey struct{}

func WithRequest(ctx context.Context, r *Request) context.Context {
	return context.WithValue(ctx, ctxKey{}, r)
}

func Req(ctx context.Context) *Request {
	if r, ok := ctx.Value(ctxKey{}).(*Request); ok {
		return r
	}
	return &Request{Now: time.Now()}
}

func CSRF(ctx context.Context) string { return Req(ctx).CSRF }

func IsOwner(ctx context.Context) bool {
	p := Req(ctx).Principal
	return p != nil && p.Role == "owner"
}

func UserName(ctx context.Context) string {
	if p := Req(ctx).Principal; p != nil {
		return p.Account.Name
	}
	return ""
}

func UserInitial(ctx context.Context) string {
	n := strings.TrimSpace(UserName(ctx))
	if n == "" {
		return "?"
	}
	return strings.ToUpper(string([]rune(n)[0]))
}

func HouseholdName(ctx context.Context) string {
	if p := Req(ctx).Principal; p != nil && p.Household != nil {
		return p.Household.Name
	}
	return ""
}

func RoleLabel(ctx context.Context) string {
	if p := Req(ctx).Principal; p != nil && p.Role != "" {
		return strings.ToUpper(p.Role[:1]) + p.Role[1:]
	}
	return ""
}

// Formatting helpers --------------------------------------------------------

func Num(v float64) string { return units.Format(v) }

func Qty(amount float64, unit string) string { return units.Display(amount, unit) }

func OptNum(v *float64) string {
	if v == nil {
		return ""
	}
	return units.Format(*v)
}

func Money(v float64) string {
	sign := ""
	if v < 0 {
		sign = "-"
		v = -v
	}
	return fmt.Sprintf("%s$%.2f", sign, math.Round(v*100)/100)
}

func ISODate(t time.Time) string { return t.Format("2006-01-02") }

func LongDate(t time.Time) string { return t.Format("Monday, January 2") }

func ShortDate(t time.Time) string { return t.Format("Mon, Jan 2") }

func DayNum(t time.Time) string { return t.Format("2") }

func Weekday(t time.Time) string { return t.Format("Mon") }

func DateTime(t time.Time) string { return t.Format("Jan 2, 2006 15:04") }

func OptDateTime(t *time.Time) string {
	if t == nil {
		return ""
	}
	return DateTime(*t)
}

// Until renders a minute count as "now", "in 45 min", or "in 2 h 5 min".
func Until(mins int) string {
	switch {
	case mins <= 0:
		return "now"
	case mins < 60:
		return fmt.Sprintf("in %d min", mins)
	case mins%60 == 0:
		return fmt.Sprintf("in %d h", mins/60)
	default:
		return fmt.Sprintf("in %d h %d min", mins/60, mins%60)
	}
}

// Clock renders "18:30" as "6:30 PM".
func Clock(hhmm string) string {
	t, err := time.Parse("15:04", hhmm)
	if err != nil {
		return hhmm
	}
	return t.Format("3:04 PM")
}

func Itoa(v int) string { return fmt.Sprint(v) }

func I32(v int32) string { return fmt.Sprint(v) }

func ID(id uuid.UUID) string { return id.String() }

func Minutes(m int32) string {
	if m <= 0 {
		return "—"
	}
	if m < 60 {
		return fmt.Sprintf("%d min", m)
	}
	if m%60 == 0 {
		return fmt.Sprintf("%d h", m/60)
	}
	return fmt.Sprintf("%d h %d min", m/60, m%60)
}

func Plural(n int, one, many string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s", n, one)
	}
	return fmt.Sprintf("%d %s", n, many)
}

func Query(base string, kv ...string) string {
	v := url.Values{}
	for i := 0; i+1 < len(kv); i += 2 {
		if kv[i+1] != "" {
			v.Set(kv[i], kv[i+1])
		}
	}
	if len(v) == 0 {
		return Path(base)
	}
	return Path(base) + "?" + v.Encode()
}

func If(cond bool, a, b string) string {
	if cond {
		return a
	}
	return b
}

func Coverage(e service.Estimate) string {
	return fmt.Sprintf("%d of %d ingredients priced", e.Priced, e.Leaves)
}

func NavClass(active, item string) string {
	if active == item {
		return "app-nav-item is-active"
	}
	return "app-nav-item"
}

func TabClass(active, item string) string {
	if active == item {
		return "mp-tab is-active"
	}
	return "mp-tab"
}

func Join(xs []string, sep string) string { return strings.Join(xs, sep) }

var UnitOptions = units.Common

var WeekdayNames = []string{"Sun", "Mon", "Tue", "Wed", "Thu", "Fri", "Sat"}
