// Package migrations embeds SQL migration files so the server can
// self-migrate at startup.
package migrations

import "embed"

//go:embed *.sql
var FS embed.FS
