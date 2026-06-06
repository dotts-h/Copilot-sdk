package ctxforge

import (
	"fmt"
	"strings"
)

// Snippet is a saved, reusable prompt the user can insert into the composer — a
// lighter cousin of a Skill. Where a Skill is *system-message context* (a prompt
// fragment compiled into the session when enabled), a Snippet is a *one-shot
// user prompt*: it is never compiled into the system message and never toggled
// into a session. It is purely a library entry surfaced through the composer's
// slash autocomplete (its ID doubles as the /trigger). — see ADR-0015.
type Snippet struct {
	ID string `json:"id"`
	// Name is the human label shown in the library and the autocomplete menu.
	Name string `json:"name"`
	// Body is the prompt text inserted into the composer when the snippet is
	// chosen (or sent as-is when its /trigger is submitted directly).
	Body string `json:"body"`
}

// Validate reports whether the snippet is well-formed.
func (s Snippet) Validate() error {
	if err := validateID("snippet", s.ID); err != nil {
		return err
	}
	if strings.TrimSpace(s.Name) == "" {
		return fmt.Errorf("snippet %q: name is required", s.ID)
	}
	if strings.TrimSpace(s.Body) == "" {
		return fmt.Errorf("snippet %q: body is required", s.ID)
	}
	return nil
}

// Snippet returns the snippet with the given ID, or nil.
func (f *Forge) Snippet(id string) *Snippet {
	for i := range f.Snippets {
		if f.Snippets[i].ID == id {
			return &f.Snippets[i]
		}
	}
	return nil
}

// AddSnippet validates and appends a snippet, rejecting duplicate IDs.
func (f *Forge) AddSnippet(s Snippet) error {
	return f.mutate(func() error {
		if err := s.Validate(); err != nil {
			return err
		}
		if f.Snippet(s.ID) != nil {
			return fmt.Errorf("snippet %q already exists", s.ID)
		}
		f.Snippets = append(f.Snippets, s)
		return nil
	})
}

// UpdateSnippet replaces the snippet identified by id with s, then validates the
// whole forge and rolls back to the prior value if the result is invalid (e.g. a
// blank name, or a rename that collides with another id).
func (f *Forge) UpdateSnippet(id string, s Snippet) error {
	return f.mutate(func() error {
		cur := f.Snippet(id)
		if cur == nil {
			return fmt.Errorf("unknown snippet %q", id)
		}
		*cur = s
		return nil
	})
}

// RemoveSnippet deletes the snippet with the given id. Nothing within the forge
// references a snippet; it routes through mutate for uniform discipline.
func (f *Forge) RemoveSnippet(id string) error {
	return f.mutate(func() error {
		for i := range f.Snippets {
			if f.Snippets[i].ID == id {
				f.Snippets = append(f.Snippets[:i], f.Snippets[i+1:]...)
				return nil
			}
		}
		return fmt.Errorf("unknown snippet %q", id)
	})
}
