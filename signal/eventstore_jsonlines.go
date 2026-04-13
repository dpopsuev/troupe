// eventstore_jsonlines.go — JSON-Lines file adapter for EventStore.
//
// Extracted from DurableBus. Each Event is one JSON line.
// Append writes one line. ReadSince scans from line N. Close flushes.
//
// TRP-TSK-77, TRP-GOL-13
package signal

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"sync"
)

var _ EventStore = (*JSONLinesStore)(nil)

// JSONLinesStore persists events as JSON-Lines (one JSON object per line).
type JSONLinesStore struct {
	mu     sync.Mutex
	path   string
	file   *os.File
	enc    *json.Encoder
	count  int
	closed bool
}

// NewJSONLinesStore opens or creates a JSON-Lines file for event storage.
func NewJSONLinesStore(path string) (*JSONLinesStore, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644) //nolint:mnd // standard file permissions
	if err != nil {
		return nil, fmt.Errorf("open event store %s: %w", path, err)
	}

	// Count existing lines for accurate Len().
	count, _ := countLines(path)

	return &JSONLinesStore{
		path:  path,
		file:  f,
		enc:   json.NewEncoder(f),
		count: count,
	}, nil
}

func (s *JSONLinesStore) Append(e Event) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return fmt.Errorf("event store closed")
	}
	if err := s.enc.Encode(e); err != nil {
		return fmt.Errorf("write event: %w", err)
	}
	s.count++
	return nil
}

func (s *JSONLinesStore) ReadSince(index int) ([]Event, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if index < 0 {
		index = 0
	}

	f, err := os.Open(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read event store: %w", err)
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	var events []Event
	lineNum := 0
	for scanner.Scan() {
		if lineNum >= index {
			var e Event
			if err := json.Unmarshal(scanner.Bytes(), &e); err != nil {
				continue // skip malformed lines
			}
			events = append(events, e)
		}
		lineNum++
	}
	if err := scanner.Err(); err != nil {
		return events, fmt.Errorf("scan event store: %w", err)
	}
	if len(events) == 0 {
		return nil, nil
	}
	return events, nil
}

func (s *JSONLinesStore) Len() (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.count, nil
}

func (s *JSONLinesStore) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil
	}
	s.closed = true
	if s.file != nil {
		err := s.file.Close()
		s.file = nil
		s.enc = nil
		return err
	}
	return nil
}

// Path returns the file path of the store.
func (s *JSONLinesStore) Path() string { return s.path }

// countLines counts lines in a file (for initial Len on reopen).
func countLines(path string) (int, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	count := 0
	for scanner.Scan() {
		count++
	}
	return count, scanner.Err()
}
