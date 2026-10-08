// Package audit writes the append-only JSON-lines audit log.
package audit

import (
	"encoding/json"
	"os"
	"sync"
	"time"
)

type Event struct {
	TS        time.Time `json:"ts"`
	Action    string    `json:"action"`
	Container string    `json:"container,omitempty"`
	Path      string    `json:"path,omitempty"`
	Result    string    `json:"result"`
	Error     string    `json:"error,omitempty"`
	Remote    string    `json:"remote,omitempty"`
}

type Log struct {
	mu sync.Mutex
	f  *os.File
}

func Open(path string) (*Log, error) {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0o600)
	if err != nil {
		return nil, err
	}
	if err := f.Chmod(0o600); err != nil {
		f.Close()
		return nil, err
	}
	return &Log{f: f}, nil
}

// Write appends one event as a single JSON line. TS defaults to now (UTC).
func (l *Log) Write(e Event) error {
	if e.TS.IsZero() {
		e.TS = time.Now().UTC()
	}
	b, err := json.Marshal(e)
	if err != nil {
		return err
	}
	b = append(b, '\n')
	l.mu.Lock()
	defer l.mu.Unlock()
	_, err = l.f.Write(b)
	return err
}

func (l *Log) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.f.Close()
}
