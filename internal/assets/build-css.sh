#!/usr/bin/env sh
# Compile the Tailwind stylesheet into internal/assets/static/app.css.
#
# Uses the standalone Tailwind CLI (single binary, no Node/npm) so the same build
# works locally and inside the golang-alpine Docker stage. The binary is pinned
# and cached under .cache/ (gitignored); set TAILWIND_BIN to reuse a preinstalled
# one and skip the download.
set -eu

VERSION="v3.4.17"
SCRIPT_DIR="$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)"
REPO_ROOT="$(CDPATH= cd -- "$SCRIPT_DIR/../.." && pwd)"
cd "$REPO_ROOT"

CONFIG="internal/assets/tailwind.config.js"
INPUT="internal/assets/tailwind.input.css"
OUTPUT="internal/assets/static/app.css"

if [ -n "${TAILWIND_BIN:-}" ]; then
	BIN="$TAILWIND_BIN"
else
	case "$(uname -m)" in
		x86_64|amd64) ARCH="x64" ;;
		aarch64|arm64) ARCH="arm64" ;;
		*) echo "unsupported arch: $(uname -m)" >&2; exit 1 ;;
	esac
	OS="linux"
	case "$(uname -s)" in
		Darwin) OS="macos" ;;
	esac
	# Tailwind ships a glibc-only standalone binary (Node SEA). On musl/alpine it
	# runs via gcompat + libstdc++ (installed in the Docker build stage).
	CACHE_DIR="$REPO_ROOT/.cache"
	BIN="$CACHE_DIR/tailwindcss-$VERSION-$OS-$ARCH"
	if [ ! -x "$BIN" ]; then
		mkdir -p "$CACHE_DIR"
		URL="https://github.com/tailwindlabs/tailwindcss/releases/download/$VERSION/tailwindcss-$OS-$ARCH"
		echo "downloading tailwindcss $VERSION ($OS-$ARCH)..." >&2
		if command -v curl >/dev/null 2>&1; then
			curl -sSfL "$URL" -o "$BIN"
		else
			wget -qO "$BIN" "$URL"
		fi
		chmod +x "$BIN"
	fi
fi

"$BIN" --config "$CONFIG" --input "$INPUT" --output "$OUTPUT" --minify
echo "wrote $OUTPUT" >&2
