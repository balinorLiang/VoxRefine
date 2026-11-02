package migrations

import "embed"

// FS contains the ordered SQL migrations shipped with the server.
//
//go:embed *.sql
var FS embed.FS
