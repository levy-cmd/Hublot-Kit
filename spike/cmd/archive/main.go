// archive probes a stopped container: HEAD/GET /archive on a file, then a full
// tar-header index of "/" (entries, bytes, duration, volume content present?).
package main

import (
	"archive/tar"
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/levy-cmd/hublot-kit/spike/dockerapi"
)

type countingReader struct {
	r io.Reader
	n int64
}

func (c *countingReader) Read(p []byte) (int, error) {
	k, err := c.r.Read(p)
	c.n += int64(k)
	return k, err
}

func fatal(format string, a ...any) {
	fmt.Fprintf(os.Stderr, "FATAL "+format+"\n", a...)
	os.Exit(1)
}

// probeGet fetches /archive for path and describes the first tar entry.
func probeGet(ctx context.Context, dc *dockerapi.Client, id, path string) {
	rc, err := dc.GetArchive(ctx, id, path)
	if err != nil {
		fmt.Printf("RESULT GET %s: err=%v\n", path, err)
		return
	}
	defer rc.Close()
	tr := tar.NewReader(rc)
	h, err := tr.Next()
	if err != nil {
		fmt.Printf("RESULT GET %s: tar err=%v\n", path, err)
		return
	}
	fmt.Printf("RESULT GET %s: typeflag=%q(%d) name=%q linkname=%q size=%d\n",
		path, h.Typeflag, h.Typeflag, h.Name, h.Linkname, h.Size)
	if h.Typeflag == tar.TypeReg || h.Typeflag == tar.TypeRegA {
		line, err := bufio.NewReader(tr).ReadString('\n')
		fmt.Printf("RESULT GET %s: first line %q err=%v\n", path, strings.TrimSpace(line), err)
	}
}

func main() {
	if len(os.Args) != 2 {
		fatal("usage: archive <container>")
	}
	id := os.Args[1]
	ctx := context.Background()
	dc := dockerapi.New("/var/run/docker.sock")

	ct, err := dc.Inspect(ctx, id, true)
	if err != nil {
		fatal("inspect: %v", err)
	}
	var size int64
	if ct.SizeRootFs != nil {
		size = *ct.SizeRootFs
	}
	fmt.Printf("running=%v SizeRootFs=%d\n", ct.State.Running, size)

	st, err := dc.ArchiveStat(ctx, id, "/etc/os-release")
	fmt.Printf("RESULT HEAD /etc/os-release: %+v err=%v\n", st, err)

	probeGet(ctx, dc, id, "/etc/os-release")
	probeGet(ctx, dc, id, "/usr/lib/os-release")

	start := time.Now()
	rc, err := dc.GetArchive(ctx, id, "/")
	if err != nil {
		fatal("GET archive /: %v", err)
	}
	defer rc.Close()
	cr := &countingReader{r: rc}
	tr := tar.NewReader(cr)
	entries, inRoot, inVolume := 0, false, false
	for {
		h, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			fatal("tar: %v", err)
		}
		entries++
		if h.Typeflag == tar.TypeReg || h.Typeflag == tar.TypeRegA {
			name := strings.TrimLeft(strings.TrimPrefix(h.Name, "./"), "/")
			inRoot = inRoot || name == "in-root.txt"
			inVolume = inVolume || name == "data/in-volume.txt"
		}
	}
	fmt.Printf("RESULT index /: %d entries, %d bytes read through tar end, %s\n", entries, cr.n, time.Since(start))
	fmt.Printf("RESULT in-root.txt present=%v, in-volume.txt (volume) present=%v\n", inRoot, inVolume)
}
