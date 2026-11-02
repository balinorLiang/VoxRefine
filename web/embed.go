package webassets

import "embed"

// FS contains the v2 no-build frontend.
//
//go:embed index.html styles.css app.js config.js
var FS embed.FS
