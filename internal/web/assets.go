// Package web is the server-rendered htmx frontend for my-orchestra. It replaces
// the Bubble Tea TUI (see docs/WEB_UI_PLAN.md): the non-UI core
// (internal/copilot, ctxforge, telemetry, config) is reused unchanged, and the
// normalized copilot.Event stream is rendered to the browser as HTML fragments
// over Server-Sent Events.
package web

import (
	"embed"
	"html/template"
)

//go:embed templates/*.html
var templateFS embed.FS

//go:embed static/*
var staticFS embed.FS

// pageTemplates holds the parsed page/layout templates.
var pageTemplates = template.Must(template.ParseFS(templateFS, "templates/*.html"))
