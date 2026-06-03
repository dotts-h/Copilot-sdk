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

// serveEvents is the long-lived SSE stream. It ranges the client's normalized
// Event channel and writes one SSE message per renderable event until the client
// disconnects or the event channel closes. SSE is unidirectional and the right
// fit for one-way token streaming (see docs/WEB_UI_PLAN.md).
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

	// Greet the client so the connection opens immediately.
	writeSSE(w, "ready", "")
	flusher.Flush()

	events := s.client.Events()
	ctx := r.Context()
	for {
		select {
		case <-ctx.Done():
			return
		case e, open := <-events:
			if !open {
				return
			}
			for _, frag := range s.handleEvent(e) {
				writeSSE(w, frag.Event, frag.HTML)
			}
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
