// Package migrations embeds Kaart's goose migration files so the server ships
// as a single binary with no external SQL to deploy alongside it.
package migrations

import "embed"

// FS holds every .sql migration in this directory.
//
//go:embed *.sql
var FS embed.FS
