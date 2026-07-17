package siteassets

import "embed"

// Files contains the complete static website template.
//
//go:embed assets
var Files embed.FS
