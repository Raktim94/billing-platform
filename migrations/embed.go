// Package migrations embeds the platform's SQL migration files so
// apps/server and apps/worker can run them without needing the source
// migrations/ directory present at runtime (e.g. inside a distroless
// Docker image).
package migrations

import "embed"

//go:embed *.sql
var FS embed.FS
