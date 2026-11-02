package glossarydata

import "embed"

// FS contains the canonical glossary seed data.
//
//go:embed glossary.json
var FS embed.FS
