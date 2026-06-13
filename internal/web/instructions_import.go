// Copyright (c) 2026 Horia C. Rădulescu
// SPDX-License-Identifier: BUSL-1.1

package web

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/dotts-h/copilot-sdk/internal/ctxforge"
)

// This file implements "import project instructions" (backlog #9, ADR 0003):
// a one-shot copy of the well-known instruction files in the working directory
// into forge Instructions, so a project bootstraps its always-on guidance.

// importedPriority is the default priority assigned to imported instructions —
// mid-range, so hand-authored instructions can be ordered before or after them.
const importedPriority = 50

// importSources maps a well-known instruction file (relative to the working
// dir) to the forge Instruction id/title it imports into.
var importSources = []struct{ path, id, title string }{
	{".github/copilot-instructions.md", "import-copilot-instructions", "Copilot instructions (.github/copilot-instructions.md)"},
	{"AGENTS.md", "import-agents-md", "AGENTS.md"},
	{"CLAUDE.md", "import-claude-md", "CLAUDE.md"},
}

// importInstructionFiles reads the well-known instruction files under dir and
// returns an Instruction for each one present and non-empty. Pure (no forge
// mutation) so it is unit-testable against a temp dir.
func importInstructionFiles(dir string) []ctxforge.Instruction {
	var out []ctxforge.Instruction
	for _, src := range importSources {
		data, err := os.ReadFile(filepath.Join(dir, src.path))
		if err != nil {
			continue // missing/unreadable — skip
		}
		body := strings.TrimSpace(string(data))
		if body == "" {
			continue
		}
		out = append(out, ctxforge.Instruction{
			ID: src.id, Title: src.title, Body: body, Priority: importedPriority, Enabled: true,
		})
	}
	return out
}

// handleInstructionImport imports the working-directory instruction files into
// the forge (creating or updating each), then re-renders the list.
func (s *Server) handleInstructionImport(w http.ResponseWriter, r *http.Request) {
	for _, in := range importInstructionFiles(s.hub.workdir) {
		in := in
		err := s.editForge(func() error {
			if s.forge.Instruction(in.ID) != nil {
				return s.forge.UpdateInstruction(in.ID, in)
			}
			return s.forge.AddInstruction(in)
		})
		if err != nil {
			s.logger.Printf("import instruction %q: %v", in.ID, err)
		}
	}
	s.writePartial(w, s.instructionsPartial())
}
