// Package web embeds the browser UI so the binary can serve it from any directory.
package web

import "embed"

// Files holds index.html, app.js and style.css.
//
//go:embed index.html app.js style.css
var Files embed.FS
