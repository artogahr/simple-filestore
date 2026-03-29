// Package assets embeds static files and templates into the binary.
package assets

import "embed"

//go:embed all:templates all:static
var FS embed.FS
