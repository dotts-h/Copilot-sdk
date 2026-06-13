// Copyright (c) 2026 Horia C. Rădulescu
// SPDX-License-Identifier: BUSL-1.1

package web

import (
	"fmt"
	"io"
	"net/http"
	"strings"
)

// fragment is one rendered SSE message: a named event plus its HTML payload.
type fragment struct {
	Event string
	HTML  string
}

// serveEvents is the long-lived SSE stream for one browser session. It
// subscribes to the session's fragment broadcast (fed by the Hub's event pump
// and by self-injected errors) and writes one SSE message per fragment until the
// client disconnects. SSE is unidirectional and the right fit for one-way token
// streaming (see docs/WEB_UI_PLAN.md).
func (s *Server) serveEvents(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}

	// Headers that keep proxies/servers from buffering the stream.
	h := w.Header()
	h.Set("Content-Type", "text/event-stream")
	h.Set("Cache-Control", "no-cache")
	h.Set("Connection", "keep-alive")
	h.Set("X-Accel-Buffering", "no")

	// Subscribe before greeting so no event slips through between the greeting
	// and the subscription.
	ch := s.subscribe()
	defer s.unsubscribe(ch)

	// Greet the client so the connection opens immediately.
	writeSSE(w, "ready", "")
	flusher.Flush()

	ctx := r.Context()
	for {
		select {
		case <-ctx.Done():
			return
		case frag, open := <-ch:
			if !open {
				return
			}
			writeSSE(w, frag.Event, frag.HTML)
			flusher.Flush()
		}
	}
}

// writeSSE writes a single Server-Sent Event. The data is emitted as one or more
// "data:" lines (one per physical line) terminated by a blank line, per the SSE
// wire format.
func writeSSE(w io.Writer, event, data string) {
	var b strings.Builder
	if event != "" {
		fmt.Fprintf(&b, "event: %s\n", event)
	}
	if data == "" {
		// A data line is required for the browser to dispatch the event.
		b.WriteString("data: \n")
	} else {
		for _, line := range strings.Split(data, "\n") {
			fmt.Fprintf(&b, "data: %s\n", line)
		}
	}
	b.WriteString("\n")
	// The error here means the client disconnected; the SSE loop notices via the
	// request context and returns, so there is nothing useful to do with it.
	_, _ = io.WriteString(w, b.String())
}
