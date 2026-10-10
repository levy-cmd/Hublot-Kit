package dockerx

import (
	"bufio"
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	cerrdefs "github.com/containerd/errdefs"
	"github.com/moby/moby/client"
)

// MaxLogLine caps the text of one line of the live view (F6): the size of
// the parts Docker cuts a long line into. The download keeps more.
const MaxLogLine = 16 << 10

// dockerPartial is the size of the parts Docker cuts a long line into
// (measured on Docker 29.7: 16 384 bytes, each behind the same timestamp).
const dockerPartial = 16 << 10

// ErrNoLogs: the container's logging driver cannot be read (--log-driver
// none, syslog…): Docker answers 501 "configured logging driver does not
// support reading".
var ErrNoLogs = errors.New("le pilote de logs de ce conteneur ne permet pas la lecture")

// LogsOptions of GET /containers/{id}/logs: stdout, stderr and timestamps
// are always asked for. Tail is a number of lines or "all"; Since, when
// set, an RFC 3339 timestamp, inclusive; Follow keeps the stream open while
// the container runs (a stopped container's stream ends by itself).
type LogsOptions struct {
	Tail   string
	Since  string
	Follow bool
}

// LogLine is one line of a container's logs, without its line end.
type LogLine struct {
	TS        time.Time // zero when Docker gave none
	Stream    string    // "stdout" | "stderr" (a TTY merges both into stdout)
	Text      string
	Truncated bool // longer than MaxLogLine: the rest was dropped
}

func (e *engine) Logs(ctx context.Context, id string, opt LogsOptions) (io.ReadCloser, error) {
	rc, err := e.c.ContainerLogs(ctx, id, client.ContainerLogsOptions{
		ShowStdout: true, ShowStderr: true, Timestamps: true,
		Follow: opt.Follow, Tail: opt.Tail, Since: opt.Since,
	})
	if err != nil {
		return nil, wrapLogs(err)
	}
	return rc, nil
}

// wrapLogs: wrap, plus a driver Docker cannot read (NotImplemented).
func wrapLogs(err error) error {
	if cerrdefs.IsNotImplemented(err) {
		return fmt.Errorf("%w : %v", ErrNoLogs, err)
	}
	return wrap(err)
}

// ReadLogs reads a logs stream asked with timestamps and calls emit for
// each line, in order. Without a TTY the stream is Docker's multiplexed
// format (8-byte header: stream type, 3 zero bytes, big-endian size);
// with a TTY it is raw, one stream. A line Docker cut into parts is joined
// back (per stream), its text capped at max bytes (Truncated beyond). A
// frame is never read whole into memory, whatever size its header
// announces. It stops at the first error of r or of emit, and returns nil
// at the end of the stream.
func ReadLogs(r io.Reader, tty bool, max int, emit func(LogLine) error) error {
	if tty {
		return readRaw(bufio.NewReaderSize(r, 64<<10), max, emit)
	}
	return readFrames(r, max, emit)
}

// lineBuf assembles the line of one stream.
type lineBuf struct {
	max       int
	stream    string
	ts        time.Time
	open      bool // a part was read, its line has not ended yet
	text      []byte
	truncated bool
}

// add keeps at most max+1 bytes: one more for the "\r" of a TTY line end,
// which flush removes before it judges the length.
func (b *lineBuf) add(p []byte) {
	if room := b.max + 1 - len(b.text); len(p) > room {
		p, b.truncated = p[:room], true
	}
	b.text = append(b.text, p...)
}

func (b *lineBuf) flush(emit func(LogLine) error) error {
	if !b.open {
		return nil
	}
	text := strings.TrimSuffix(string(b.text), "\r")
	if len(text) > b.max {
		text, b.truncated = text[:b.max], true
	}
	l := LogLine{TS: b.ts, Stream: b.stream, Text: text, Truncated: b.truncated}
	b.open, b.text, b.truncated = false, b.text[:0], false
	return emit(l)
}

// splitTS splits "<RFC 3339 timestamp> <text>"; no timestamp: zero, p.
func splitTS(p []byte) (time.Time, []byte) {
	if i := bytes.IndexByte(p[:min(len(p), 64)], ' '); i > 0 {
		if ts, err := time.Parse(time.RFC3339Nano, string(p[:i])); err == nil {
			return ts, p[i+1:]
		}
	}
	return time.Time{}, p
}

func readFrames(r io.Reader, max int, emit func(LogLine) error) error {
	bufs := map[byte]*lineBuf{1: {max: max, stream: "stdout"}, 2: {max: max, stream: "stderr"}}
	bufs[0] = bufs[1] // stdin, as stdcopy: on stdout
	var hdr [8]byte
	chunk := make([]byte, 32<<10)
	for {
		if _, err := io.ReadFull(r, hdr[:]); err != nil {
			if err == io.EOF {
				break
			}
			return err
		}
		size := int64(binary.BigEndian.Uint32(hdr[4:]))
		if hdr[0] == 3 { // Systemerr: an error of the daemon, as text
			msg, _ := io.ReadAll(io.LimitReader(r, min(size, 4<<10)))
			return fmt.Errorf("%w : %s", ErrUnavailable, msg)
		}
		b, ok := bufs[hdr[0]]
		if !ok {
			return fmt.Errorf("%w : type de trame de logs inconnu %d", ErrUnavailable, hdr[0])
		}
		// The payload is read in chunks: only the head (the timestamp) and
		// what fits in max is kept.
		body := io.LimitReader(r, size)
		first := true
		for {
			n, err := io.ReadFull(body, chunk)
			if n > 0 {
				if err := b.take(chunk[:n], first, emit); err != nil {
					return err
				}
				first = false
			}
			if err == io.EOF || err == io.ErrUnexpectedEOF {
				break
			}
			if err != nil {
				return err
			}
		}
		if n, _ := body.(*io.LimitedReader); n.N > 0 {
			return io.ErrUnexpectedEOF // the stream ended inside a frame
		}
	}
	for _, k := range []byte{1, 2} {
		if err := bufs[k].flush(emit); err != nil {
			return err
		}
	}
	return nil
}

// take adds a piece of a frame's payload; head: the frame starts with it
// (and with a timestamp, repeated by every part of a long line). A "\n"
// ends the line: the rest of the frame is a new line of the same time.
func (b *lineBuf) take(p []byte, head bool, emit func(LogLine) error) error {
	if head {
		ts, rest := splitTS(p)
		if !b.open {
			b.ts = ts
		}
		p = rest
	}
	for len(p) > 0 {
		b.open = true
		i := bytes.IndexByte(p, '\n')
		if i < 0 {
			b.add(p)
			return nil
		}
		b.add(p[:i])
		ts := b.ts
		if err := b.flush(emit); err != nil {
			return err
		}
		b.ts, p = ts, p[i+1:]
	}
	return nil
}

// readRaw reads a TTY stream: "<timestamp> <text>\r\n" per line; a long
// line comes as parts of dockerPartial bytes, each behind the same
// timestamp, glued together: that timestamp is removed exactly there. It
// reads byte by byte from a buffer and never waits for more bytes than the
// stream has sent: a live line is emitted as soon as its "\n" arrives.
func readRaw(r *bufio.Reader, max int, emit func(LogLine) error) error {
	b := &lineBuf{max: max, stream: "stdout"}
	var head []byte   // start of a line, until its timestamp is known
	var tsText []byte // the timestamp of the current line, as written, with its space
	part := 0         // bytes of the current part
	match := -1       // at a part boundary: bytes of tsText matched so far
	atStart := true
	feed := func(c byte) error {
		b.open = true
		if c == '\n' {
			atStart = true
			return b.flush(emit)
		}
		b.add([]byte{c})
		if part++; part == dockerPartial && len(tsText) > 0 {
			match = 0
		}
		return nil
	}
	feedAll := func(p []byte) error {
		for _, c := range p {
			if err := feed(c); err != nil {
				return err
			}
		}
		return nil
	}
	for {
		c, err := r.ReadByte()
		if err == io.EOF {
			if err := feedAll(head); err != nil {
				return err
			}
			if match > 0 {
				if err := feedAll(tsText[:match]); err != nil {
					return err
				}
			}
			return b.flush(emit)
		}
		if err != nil {
			return err
		}
		switch {
		case atStart:
			head = append(head, c)
			if c != ' ' && c != '\n' && len(head) < 64 {
				continue
			}
			atStart, b.open, part, tsText = false, true, 0, tsText[:0]
			if ts, rest := splitTS(head); len(rest) == 0 && c == ' ' {
				b.ts, tsText = ts, append(tsText, head...)
			} else {
				b.ts = time.Time{}
				if err := feedAll(head); err != nil {
					return err
				}
			}
			head = head[:0]
		case match >= 0:
			if c == tsText[match] {
				if match++; match == len(tsText) {
					match, part = -1, 0
				}
				continue
			}
			pending := tsText[:match]
			match = -1
			if err := feedAll(pending); err != nil {
				return err
			}
			if err := feed(c); err != nil {
				return err
			}
		default:
			if err := feed(c); err != nil {
				return err
			}
		}
	}
}
