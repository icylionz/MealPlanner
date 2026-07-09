// Package view holds cross-cutting view helpers: base-path aware URLs and
// date formatting shared by templ pages.
package view

import (
	"context"
	"fmt"
	"time"
)

type ctxKey int

const basePathKey ctxKey = iota

// WithBasePath stores the deployment base path in the request context.
func WithBasePath(ctx context.Context, basePath string) context.Context {
	return context.WithValue(ctx, basePathKey, basePath)
}

// Href prefixes an app-absolute path with the deployment base path.
func Href(ctx context.Context, path string) string {
	bp, _ := ctx.Value(basePathKey).(string)
	return bp + path
}

// DateFormat is the wire format for dates in URLs and forms.
const DateFormat = "2006-01-02"

// ParseDate parses a YYYY-MM-DD string, falling back to today.
func ParseDate(s string) time.Time {
	t, err := time.Parse(DateFormat, s)
	if err != nil {
		now := time.Now()
		return time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	}
	return t
}

// DayLabelLong renders "Monday, 7 July 2026".
func DayLabelLong(date string) string {
	return ParseDate(date).Format("Monday, 2 January 2006")
}

// DayLabelMedium renders "Monday, 7 July".
func DayLabelMedium(date string) string {
	return ParseDate(date).Format("Monday, 2 January")
}

// DayLabelShort renders "Mon, 7 Jul".
func DayLabelShort(date string) string {
	return ParseDate(date).Format("Mon, 2 Jan")
}

// Itoa renders an int.
func Itoa(n int) string { return fmt.Sprintf("%d", n) }

// Plural returns singular or plural depending on n.
func Plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}
