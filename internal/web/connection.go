package web

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/dotts-h/copilot-sdk/internal/config"
)

// This file is the Connection page (A1, issue 0068): see the live credential
// (the seam's read-only AuthStatus) and choose the auth method. Choosing is a
// config edit applied at the NEXT launch — there is no runtime auth write
// surface and no live client re-dial (ADR-0039). A pasted token lands in the
// process environment only (the setEnv seam); config persists the ${VAR} NAME,
// never the value (ADR-0020).

// authRung is one row of the rendered credential-precedence ladder. Active
// marks the rung the CONFIGURED method selects (intent, not the live probe —
// the live credential renders separately from AuthStatus).
type authRung struct {
	Label  string
	Desc   string
	Active bool
}

// precedenceRows builds the precedence ladder (spike 0067, finding 7): the
// explicit token outranks the chain the CLI walks on its own. The configured
// method marks its rung; plain auto (no method, no var) marks none — the CLI
// resolves top-down and only the live status says where it landed.
func precedenceRows(method, tokenEnv string) []authRung {
	rows := []authRung{
		{Label: "Explicit token", Desc: "the configured ${VAR}, injected as COPILOT_SDK_AUTH_TOKEN — never written to disk"},
		{Label: "COPILOT_GITHUB_TOKEN", Desc: "process env, inherited by the CLI"},
		{Label: "GITHUB_TOKEN", Desc: "process env, inherited by the CLI"},
		{Label: "GH_TOKEN", Desc: "process env, inherited by the CLI"},
		{Label: "gh CLI token", Desc: "an authenticated GitHub CLI"},
		{Label: "Device-flow login", Desc: "the copilot CLI's stored session"},
	}
	switch {
	case method == "token", method == "" && tokenEnv != "":
		rows[0].Active = true
	case method == "gh":
		rows[4].Active = true
	}
	return rows
}

// connectionPartial renders the Connection page for GET /page/connection.
func (s *Server) connectionPartial() string { return s.renderConnection("", "") }

// renderConnection builds the page: live status, precedence ladder, method
// chooser, and the preflights that make a method that will degrade visible
// (gh missing on PATH; the token var unset in this process).
func (s *Server) renderConnection(note, errMsg string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	status, statusErr := s.client.AuthStatus(ctx)

	s.hub.forgeMu.Lock()
	method := s.config.AuthMethod
	tokenEnv := s.config.GitHubTokenEnv
	s.hub.forgeMu.Unlock()

	var warns []string
	if method == "gh" && !s.commandAvailable("gh") {
		warns = append(warns, "gh not found on PATH — the gh method will degrade to auto at the next launch")
	}
	tokenVarState := "" // "", "set", "unset" — the value itself never renders
	if tokenEnv != "" {
		tokenVarState = "set"
		if s.lookupEnv(tokenEnv) == "" {
			tokenVarState = "unset"
			if method == "token" {
				warns = append(warns, "the token method will degrade to auto while the var is unset")
			}
		}
	}

	data := map[string]any{
		"Note": note, "Err": errMsg, "Warns": warns,
		"Status": status, "StatusErr": "",
		"Method": method, "TokenEnv": tokenEnv, "TokenVarState": tokenVarState,
		"Rungs": precedenceRows(method, tokenEnv),
	}
	if statusErr != nil {
		data["StatusErr"] = statusErr.Error()
	}
	return frag("connectionPage", data)
}

// handleConnectionSave applies the chooser form: the method + var name persist
// through editConfig (validate + rollback) FIRST, then an optional paste lands
// in the process env (never in config, never echoed back). Config-before-env
// ordering matters: the reverse would orphan the secret in the environment —
// or leave it live under a rolled-back config — when the save fails. A field
// the form didn't carry is preserved, not wiped (the settings-page
// preserve-on-absent discipline), so a method-only POST can't clear the var.
func (s *Server) handleConnectionSave(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	_, hasMethod := r.Form["authMethod"]
	_, hasVarField := r.Form["githubTokenEnv"]
	method := strings.TrimSpace(r.FormValue("authMethod"))
	if method == "auto" {
		method = "" // the form's explicit label for the default chain
	}
	varName := strings.TrimSpace(r.FormValue("githubTokenEnv"))
	paste := strings.TrimSpace(r.FormValue("pasteToken"))

	if paste != "" && varName == "" {
		s.writePartial(w, s.renderConnection("", "name the env var that will hold the pasted token — only the name is stored"))
		return
	}
	// An env-var name with whitespace or '=' can never resolve (os.Getenv
	// would always return empty), so the token method would silently degrade
	// forever. Reject here rather than in config.Validate, where it could
	// brick an existing file at Load.
	if strings.ContainsAny(varName, " \t\n=") {
		s.writePartial(w, s.renderConnection("", "the env var name can't contain spaces or '='"))
		return
	}
	err := s.editConfig(func(c *config.Config) {
		if hasMethod {
			c.AuthMethod = method
		}
		if hasVarField {
			c.GitHubTokenEnv = varName
		}
	})
	if err != nil {
		s.writePartial(w, s.renderConnection("", err.Error()))
		return
	}
	const appliesNext = "applies at next launch"
	note := "saved — " + appliesNext
	if paste != "" {
		if err := s.setEnv(varName, paste); err != nil {
			s.writePartial(w, s.renderConnection("", "saved, but storing the token in the process env failed: "+err.Error()))
			return
		}
		note = "saved — the token lives in this process only (export it in your shell to survive a restart); " + appliesNext
	}
	s.writePartial(w, s.renderConnection(note, ""))
}
