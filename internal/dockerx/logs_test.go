package dockerx

// Tests of the logs reader (F6), written before the code. The byte layouts
// are those measured on Docker 29.7 (plan Observation, "Constats"): one
// frame per message, the timestamp at the head of every frame, a line over
// 16 KiB cut into parts that repeat the same timestamp; with a TTY, no
// frames, "\r\n" line ends, and the parts glued with their timestamps.

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"
	"testing"
	"time"

	cerrdefs "github.com/containerd/errdefs"
	"github.com/moby/moby/api/types/container"
)

const (
	ts1 = "2026-10-10T07:14:22.769046552Z"
	ts2 = "2026-10-10T07:14:23.775474053Z"
)

// frame is one frame of Docker's multiplexed stream: type, 3 zero bytes,
// big-endian size, payload.
func frame(typ byte, payload string) []byte {
	h := make([]byte, 8, 8+len(payload))
	h[0] = typ
	binary.BigEndian.PutUint32(h[4:], uint32(len(payload)))
	return append(h, payload...)
}

func frames(fs ...[]byte) io.Reader { return bytes.NewReader(bytes.Join(fs, nil)) }

func readAll(t *testing.T, r io.Reader, tty bool) []LogLine {
	t.Helper()
	var out []LogLine
	if err := ReadLogs(r, tty, MaxLogLine, func(l LogLine) error { out = append(out, l); return nil }); err != nil {
		t.Fatalf("ReadLogs : %v", err)
	}
	return out
}

func mustTS(t *testing.T, s string) time.Time {
	t.Helper()
	ts, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		t.Fatal(err)
	}
	return ts
}

func TestReadLogsMultiplexed(t *testing.T) {
	got := readAll(t, frames(
		frame(2, ts1+" err1\n"),
		frame(1, ts1+" out1\n"),
		frame(1, ts2+" nonl\n"), // Docker keeps a line without "\n" until it ends
		frame(0, ts2+" stdin is stdout\n"),
	), false)
	want := []LogLine{
		{TS: mustTS(t, ts1), Stream: "stderr", Text: "err1"},
		{TS: mustTS(t, ts1), Stream: "stdout", Text: "out1"},
		{TS: mustTS(t, ts2), Stream: "stdout", Text: "nonl"},
		{TS: mustTS(t, ts2), Stream: "stdout", Text: "stdin is stdout"},
	}
	if !slices.Equal(got, want) {
		t.Fatalf("got %+v", got)
	}
}

// Measured: a 40 000-byte line comes as 16 384 + 16 384 + 7 232 bytes, each
// part behind the same timestamp. It is one line, cut at MaxLogLine.
func TestReadLogsPartialMessages(t *testing.T) {
	a := strings.Repeat("a", 16384)
	got := readAll(t, frames(
		frame(1, ts1+" "+a),
		frame(1, ts1+" "+a),
		frame(1, ts1+" "+strings.Repeat("a", 7232)+"\n"),
		frame(2, ts2+" short\n"),
	), false)
	if len(got) != 2 {
		t.Fatalf("%d lignes, attendu 2", len(got))
	}
	if got[0].Text != strings.Repeat("a", MaxLogLine) || !got[0].Truncated || got[0].Stream != "stdout" {
		t.Errorf("ligne longue : %d octets, tronquée %v", len(got[0].Text), got[0].Truncated)
	}
	if got[1] != (LogLine{TS: mustTS(t, ts2), Stream: "stderr", Text: "short"}) {
		t.Errorf("ligne suivante : %+v", got[1])
	}
}

// Parts of stdout and stderr may interleave: each stream joins its own.
func TestReadLogsPartialsPerStream(t *testing.T) {
	got := readAll(t, frames(
		frame(1, ts1+" out-"),
		frame(2, ts2+" err\n"),
		frame(1, ts1+" end\n"),
	), false)
	if len(got) != 2 || got[0].Text != "err" || got[1].Text != "out-end" || got[1].Stream != "stdout" {
		t.Fatalf("got %+v", got)
	}
}

func TestReadLogsTTY(t *testing.T) {
	got := readAll(t, strings.NewReader(ts1+" out1\r\n"+ts1+" err1\r\n"+ts2+" a\rb\r\n"), true)
	want := []LogLine{
		{TS: mustTS(t, ts1), Stream: "stdout", Text: "out1"},
		{TS: mustTS(t, ts1), Stream: "stdout", Text: "err1"},
		{TS: mustTS(t, ts2), Stream: "stdout", Text: "a\rb"}, // only the final \r goes
	}
	if !slices.Equal(got, want) {
		t.Fatalf("got %+v", got)
	}
}

// Measured with a TTY: "TS <16 384 bytes>TS <16 384 bytes>TS <rest>\r\n".
// The timestamps glued at the 16 KiB boundaries are not part of the text.
func TestReadLogsTTYPartialMessages(t *testing.T) {
	a := strings.Repeat("a", 16384)
	b := strings.Repeat("b", 16384)
	in := ts1 + " " + a + ts1 + " " + b + ts1 + " ccc\r\n" + ts2 + " short\r\n"
	got := readAll(t, strings.NewReader(in), true)
	if len(got) != 2 || got[1].Text != "short" || got[1].TS != mustTS(t, ts2) {
		t.Fatalf("got %d lignes : %+v", len(got), got[len(got)-1])
	}
	if got[0].Text != a || !got[0].Truncated || strings.Contains(got[0].Text, "Z ") {
		t.Errorf("ligne longue : %d octets, tronquée %v", len(got[0].Text), got[0].Truncated)
	}
	// A line of exactly 16 KiB: its last part is empty, behind the same
	// timestamp; the line is whole, not truncated.
	got = readAll(t, strings.NewReader(ts1+" "+a+ts1+" \r\n"), true)
	if len(got) != 1 || got[0].Text != a || got[0].Truncated {
		t.Errorf("ligne de 16 Kio exactement : %d lignes, tronquée %v", len(got), len(got) > 0 && got[0].Truncated)
	}
	// Not at a boundary, the same text stays: it belongs to the line.
	got = readAll(t, strings.NewReader(ts1+" at "+ts1+" done\r\n"), true)
	if len(got) != 1 || got[0].Text != "at "+ts1+" done" {
		t.Errorf("horodatage dans le texte : %+v", got)
	}
}

// The cap is the caller's: the live view keeps MaxLogLine, the download
// a larger one.
func TestReadLogsCap(t *testing.T) {
	for _, tty := range []bool{false, true} {
		in := io.Reader(strings.NewReader(ts1 + " abcdef\r\n" + ts1 + " abcd\r\n"))
		if !tty {
			in = frames(frame(1, ts1+" abcdef\n"), frame(1, ts1+" abcd\n"))
		}
		var got []LogLine
		ReadLogs(in, tty, 4, func(l LogLine) error { got = append(got, l); return nil })
		if len(got) != 2 || got[0].Text != "abcd" || !got[0].Truncated || got[1].Text != "abcd" || got[1].Truncated {
			t.Errorf("tty=%v : %+v", tty, got)
		}
	}
}

func TestReadLogsMessageEndsAtEOF(t *testing.T) {
	got := readAll(t, frames(frame(1, ts1+" last words")), false)
	if len(got) != 1 || got[0].Text != "last words" {
		t.Fatalf("multiplexé : %+v", got)
	}
	got = readAll(t, strings.NewReader(ts1+" tty last"), true)
	if len(got) != 1 || got[0].Text != "tty last" {
		t.Fatalf("TTY : %+v", got)
	}
}

// Several lines in one frame, a line without timestamp: kept, never lost.
func TestReadLogsOddFrames(t *testing.T) {
	got := readAll(t, frames(frame(1, ts1+" a\nb\n"), frame(1, "no timestamp\n")), false)
	if len(got) != 3 || got[0].Text != "a" || got[1].Text != "b" || got[1].TS != mustTS(t, ts1) ||
		got[2].Text != "no timestamp" || !got[2].TS.IsZero() {
		t.Fatalf("got %+v", got)
	}
}

// A frame of type 3 is an error of the daemon; a cut header or payload is
// an error too, and a header announcing 1 GiB is never allocated.
func TestReadLogsErrors(t *testing.T) {
	if err := ReadLogs(frames(frame(3, "log file is gone")), false, MaxLogLine, func(LogLine) error { return nil }); err == nil || !strings.Contains(err.Error(), "log file is gone") {
		t.Errorf("trame d'erreur : %v", err)
	}
	if err := ReadLogs(bytes.NewReader([]byte{1, 0, 0}), false, MaxLogLine, func(LogLine) error { return nil }); !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Errorf("en-tête coupé : %v", err)
	}
	huge := []byte{1, 0, 0, 0, 0x40, 0, 0, 0} // 1 GiB announced, 0 sent
	allocs := testing.AllocsPerRun(1, func() {
		ReadLogs(bytes.NewReader(huge), false, MaxLogLine, func(LogLine) error { return nil })
	})
	if allocs > 100 {
		t.Errorf("%v allocations pour une trame annoncée de 1 Gio", allocs)
	}
	stop := errors.New("stop")
	n := 0
	err := ReadLogs(frames(frame(1, ts1+" a\n"), frame(1, ts1+" b\n")), false, MaxLogLine, func(LogLine) error { n++; return stop })
	if !errors.Is(err, stop) || n != 1 {
		t.Errorf("emit en erreur : %v après %d lignes", err, n)
	}
}

func TestFromInspectTtyAndLogDriver(t *testing.T) {
	got := fromInspect(container.InspectResponse{ID: "x", Config: &container.Config{Tty: true},
		HostConfig: &container.HostConfig{LogConfig: container.LogConfig{Type: "json-file"}}})
	if !got.Tty || got.LogDriver != "json-file" {
		t.Fatalf("got %+v", got)
	}
}

func TestWrapLogs(t *testing.T) {
	err := wrapLogs(fmt.Errorf("configured logging driver does not support reading: %w", cerrdefs.ErrNotImplemented))
	if !errors.Is(err, ErrNoLogs) || errors.Is(err, ErrUnavailable) {
		t.Errorf("pilote sans lecture : %v", err)
	}
	if err := wrapLogs(fmt.Errorf("x: %w", cerrdefs.ErrNotFound)); !errors.Is(err, ErrNotFound) {
		t.Errorf("introuvable : %v", err)
	}
}

func TestFakeLogs(t *testing.T) {
	f := &Fake{LogStreams: map[string][]byte{"abc": []byte("RAW")}, LogsGate: make(chan struct{})}
	ctx, cancel := context.WithCancel(context.Background())
	rc, err := f.Logs(ctx, "abc", LogsOptions{Tail: "10", Follow: true})
	if err != nil {
		t.Fatal(err)
	}
	b := make([]byte, 3)
	if _, err := io.ReadFull(rc, b); err != nil || string(b) != "RAW" {
		t.Fatalf("lu %q %v", b, err)
	}
	done := make(chan error, 1)
	go func() { _, err := rc.Read(b); done <- err }()
	select {
	case <-done:
		t.Fatal("la porte doit retenir la suite du flux")
	case <-time.After(50 * time.Millisecond):
	}
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("après annulation : %v", err)
	}
	rc.Close()
	if f.LogsCloses.Load() != 1 || !slices.Equal(f.LogsCalls(), []LogsOptions{{Tail: "10", Follow: true}}) {
		t.Fatalf("fermetures %d, appels %+v", f.LogsCloses.Load(), f.LogsCalls())
	}
	if _, err := f.Logs(context.Background(), "nope", LogsOptions{}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("absent : %v", err)
	}
}
