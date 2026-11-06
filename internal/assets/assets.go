// Package assets embeds the static files served by the app. app.css is built
// ahead of time by build-css.sh (standalone Tailwind CLI).
package assets

import "embed"

//go:embed static
var Static embed.FS
