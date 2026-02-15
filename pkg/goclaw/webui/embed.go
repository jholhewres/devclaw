package webui

import "embed"

// distFS contains the built React SPA.
// The dist/ directory is populated by `make web-build` which runs
// `npm run build` in web/ and copies the output here.
//
//go:embed all:dist
var distFS embed.FS
