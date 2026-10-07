// changes prints GET /containers/{id}/changes as sorted "A|C|D path" lines.
package main

import (
	"context"
	"fmt"
	"os"
	"sort"

	"github.com/levy-cmd/hublot-kit/spike/dockerapi"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: changes <container>")
		os.Exit(2)
	}
	ch, err := dockerapi.New("/var/run/docker.sock").Changes(context.Background(), os.Args[1])
	if err != nil {
		fmt.Fprintln(os.Stderr, "FATAL", err)
		os.Exit(1)
	}
	kinds := map[int]string{0: "C", 1: "A", 2: "D"}
	lines := make([]string, 0, len(ch))
	for _, c := range ch {
		lines = append(lines, kinds[c.Kind]+" "+c.Path)
	}
	sort.Strings(lines)
	for _, l := range lines {
		fmt.Println(l)
	}
}
