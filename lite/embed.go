package liteassets

import "embed"

// FS contains the standalone lightweight frontend.
//
//go:embed index.html styles.css app.js config.js data/*
var FS embed.FS
