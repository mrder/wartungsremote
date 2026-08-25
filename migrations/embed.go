// Package migrations embeds the SQL migration files so wr-core can apply
// them without requiring a separate file on disk at deploy time.
package migrations

import "embed"

//go:embed *.sql
var FS embed.FS
