package telemetry

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"sync"
)

// This file single-sources the on-disk machinery that the spend ledger
// (history.go) and the workflow-run history (runs.go) both need: a versioned
// JSON envelope persisted atomically (temp-file + rename), load-or-empty on a
// missing file, reject a corrupt file, tolerate a newer schema version, and an
// ephemeral (dir == "") mode that never touches disk. Before H1 each store
// carried its own near-identical copy of all of it (the only differences being
// the file name, the envelope's array key, the schema version, and a per-record
// stamp on append); AppendOnlyStore[T] collapses that duplication so the
// persistence discipline lives in one place. SpendStore/RunStore become thin
// typed wrappers over it.
//
// INVARIANT: the on-disk JSON tags are the stable contract — the envelope's
// `version` + array key (`records` / `runs`) and every record field must NOT
// drift. A file written before H1 loads byte-for-byte identically, and the bytes
// this store writes are diff-identical to the pre-H1 stores' output (the
// envelope marshaler reproduces the exact `{"version":N,"<key>":[…]}` shape and
// `MarshalIndent` re-indents it the same). The package stays pure and
// dependency-free. — ADR-0009 (spend ledger), ADR-0022 (run history), CONTRACTS §4.

// AppendOnlyStore is the shared append-only, versioned-envelope store behind the
// spend ledger and the run history. It is goroutine-safe and atomic on write. A
// store with an empty dir is ephemeral (in-memory only) — used by the offline
// demo and tests so they never touch a real config directory.
//
// T is the record type. A store is parameterized at construction over the
// on-disk file name, the envelope's array key (`records` / `runs`), the current
// schema version stamped on write, a noun used in error messages, and an
// optional per-record stamp applied on Append (e.g. defaulting a zero timestamp).
type AppendOnlyStore[T any] struct {
	mu      sync.Mutex
	dir     string
	file    string    // on-disk file name, e.g. "spend.json"
	key     string    // envelope array key, e.g. "records" — part of the stable contract
	what    string    // noun for error messages, e.g. "spend history"
	version int       // schema version stamped on write
	stamp   func(T) T // per-record normalize applied on Append (nil = identity)
	records []T
}

// loadAppendOnlyStore reads dir/file, returning an empty store when the file is
// absent (first run) and an error when it is present but unparseable. An empty
// dir yields an ephemeral, in-memory-only store. The records array under key is
// the stable contract; a newer version's extra envelope/record fields are
// ignored, so the file stays forward-readable.
func loadAppendOnlyStore[T any](dir, file, key, what string, version int, stamp func(T) T) (*AppendOnlyStore[T], error) {
	s := &AppendOnlyStore[T]{dir: dir, file: file, key: key, what: what, version: version, stamp: stamp}
	if dir == "" {
		return s, nil
	}
	data, err := os.ReadFile(filepath.Join(dir, file))
	if os.IsNotExist(err) {
		return s, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", what, err)
	}
	records, err := decodeEnvelope[T](data, key)
	if err != nil {
		return nil, fmt.Errorf("parse %s %s: %w", what, filepath.Join(dir, file), err)
	}
	s.records = records
	return s, nil
}

// decodeEnvelope pulls the record array out of the versioned envelope by its key,
// ignoring the version tag and any unknown fields (forward-compatibility — the
// array is the stable surface). A missing key yields no records (an empty store),
// not an error. A top-level value that isn't a JSON object, or a malformed array,
// is rejected.
func decodeEnvelope[T any](data []byte, key string) ([]T, error) {
	var env map[string]json.RawMessage
	if err := json.Unmarshal(data, &env); err != nil {
		return nil, err
	}
	raw, ok := env[key]
	if !ok {
		return nil, nil
	}
	var records []T
	if err := json.Unmarshal(raw, &records); err != nil {
		return nil, err
	}
	return records, nil
}

// Append records one entry (applying the store's per-record stamp first) and
// persists the whole history atomically. An ephemeral store keeps it in memory
// only. The atomic temp-file + rename means a crash mid-write never leaves a
// partial file — the prior history stays intact.
func (s *AppendOnlyStore[T]) Append(r T) error {
	if s.stamp != nil {
		r = s.stamp(r)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.records = append(s.records, r)
	return s.save()
}

// save writes the in-memory records to disk atomically. Caller holds s.mu. A
// no-op for an ephemeral store. Mirrors config.Save: temp-file + rename.
func (s *AppendOnlyStore[T]) save() error {
	if s.dir == "" {
		return nil
	}
	data, err := encodeEnvelope(s.version, s.key, s.records)
	if err != nil {
		return fmt.Errorf("encode %s: %w", s.what, err)
	}
	if err := os.MkdirAll(s.dir, 0o755); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}
	path := filepath.Join(s.dir, s.file)
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return fmt.Errorf("write %s: %w", s.what, err)
	}
	return os.Rename(tmp, path)
}

// Records returns a snapshot copy of the history, safe to read without the lock.
func (s *AppendOnlyStore[T]) Records() []T {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]T, len(s.records))
	copy(out, s.records)
	return out
}

// Count returns how many records the history holds.
func (s *AppendOnlyStore[T]) Count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.records)
}

// envelope marshals to the exact on-disk shape `{"version":N,"<key>":[…]}` —
// version first, then the array under its key. Built by hand (rather than a
// struct with a fixed JSON tag) so a single generic store can carry either array
// key while staying byte-identical to the pre-H1 per-store envelopes:
// json.MarshalIndent calls this MarshalJSON for the compact form, then re-indents
// it, reproducing the prior output exactly (a nil slice still renders `null`).
type envelope[T any] struct {
	version int
	key     string
	records []T
}

func (e envelope[T]) MarshalJSON() ([]byte, error) {
	var b bytes.Buffer
	b.WriteString(`{"version":`)
	b.WriteString(strconv.Itoa(e.version))
	b.WriteByte(',')
	kb, err := json.Marshal(e.key)
	if err != nil {
		return nil, err
	}
	b.Write(kb)
	b.WriteByte(':')
	rb, err := json.Marshal(e.records)
	if err != nil {
		return nil, err
	}
	b.Write(rb)
	b.WriteByte('}')
	return b.Bytes(), nil
}

// encodeEnvelope renders the versioned envelope with the same 2-space indent the
// pre-H1 stores used, byte-for-byte.
func encodeEnvelope[T any](version int, key string, records []T) ([]byte, error) {
	return json.MarshalIndent(envelope[T]{version: version, key: key, records: records}, "", "  ")
}
