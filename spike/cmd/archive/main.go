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

	rc, err := dc.GetArchive(ctx, id, "/etc/os-release")
	if err != nil {
		fmt.Printf("RESULT GET /etc/os-release: err=%v\n", err)
	} else {
		tr := tar.NewReader(rc)
		if _, err := tr.Next(); err == nil {
			line, _ := bufio.NewReader(tr).ReadString('\n')
			fmt.Printf("RESULT GET /etc/os-release: first line %q\n", strings.TrimSpace(line))
		} else {
			fmt.Printf("RESULT GET /etc/os-release: tar err=%v\n", err)
		}
		rc.Close()
	}

	start := time.Now()
	rc, err = dc.GetArchive(ctx, id, "/")
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
		inRoot = inRoot || strings.HasSuffix(h.Name, "in-root.txt")
		inVolume = inVolume || strings.HasSuffix(h.Name, "in-volume.txt")
	}
	fmt.Printf("RESULT index /: %d entries, %d bytes, %s\n", entries, cr.n, time.Since(start))
	fmt.Printf("RESULT in-root.txt present=%v, in-volume.txt (volume) present=%v\n", inRoot, inVolume)
}
