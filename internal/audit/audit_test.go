package audit

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func readLines(t *testing.T, p string) []Event {
	t.Helper()
	f, err := os.Open(p)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	var out []Event
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		var e Event
		if err := json.Unmarshal(sc.Bytes(), &e); err != nil {
			t.Fatalf("invalid JSON line %q: %v", sc.Text(), err)
		}
		out = append(out, e)
	}
	return out
}

func TestWriteAppendsJSONLines(t *testing.T) {
	p := filepath.Join(t.TempDir(), "audit.log")
	l, err := Open(p)
	if err != nil {
		t.Fatal(err)
	}
	if err := l.Write(Event{Action: "login.fail", Result: "refused", Remote: "192.0.2.1"}); err != nil {
		t.Fatal(err)
	}
	if err := l.Write(Event{Action: "file.write", Container: "abc", Path: "/etc/app.conf", Result: "ok"}); err != nil {
		t.Fatal(err)
	}
	l.Close()

	got := readLines(t, p)
	if len(got) != 2 || got[0].Action != "login.fail" || got[0].Remote != "192.0.2.1" || got[1].Path != "/etc/app.conf" {
		t.Fatalf("unexpected lines: %+v", got)
	}
	if got[0].TS.IsZero() {
		t.Fatal("ts must be set")
	}
}

func TestFileMode0600AndReopenAppends(t *testing.T) {
	p := filepath.Join(t.TempDir(), "audit.log")
	for i := 0; i < 2; i++ {
		l, err := Open(p)
		if err != nil {
			t.Fatal(err)
		}
		if err := l.Write(Event{Action: "env.reveal", Result: "ok"}); err != nil {
			t.Fatal(err)
		}
		l.Close()
	}
	if n := len(readLines(t, p)); n != 2 {
		t.Fatalf("reopen must append, got %d lines", n)
	}
	st, err := os.Stat(p)
	if err != nil {
		t.Fatal(err)
	}
	if st.Mode().Perm() != 0o600 {
		t.Fatalf("mode = %v, want 0600", st.Mode().Perm())
	}
}

func TestConcurrentWritesStayWhole(t *testing.T) {
	p := filepath.Join(t.TempDir(), "audit.log")
	l, err := Open(p)
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := l.Write(Event{Action: "file.write", Result: "ok"}); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	l.Close()
	if n := len(readLines(t, p)); n != 50 {
		t.Fatalf("got %d lines, want 50", n)
	}
}
