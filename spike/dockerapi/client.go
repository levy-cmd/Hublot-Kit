// Package dockerapi is a minimal Docker Engine API client over the unix socket,
// standard library only. Throwaway spike code.
package dockerapi

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
)

type Client struct {
	hc   *http.Client
	base string
}

func New(socket string) *Client {
	tr := &http.Transport{DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
		var d net.Dialer
		return d.DialContext(ctx, "unix", socket)
	}}
	return &Client{hc: &http.Client{Transport: tr}, base: "http://docker/v1.44"}
}

type State struct {
	Running    bool
	Pid        int
	StartedAt  string
	FinishedAt string
}

type Container struct {
	ID         string `json:"Id"`
	Name       string
	State      State
	SizeRootFs *int64
}

// Change.Kind: 0 modified, 1 added, 2 deleted.
type Change struct {
	Path string
	Kind int
}

type PathStat struct {
	Name       string `json:"name"`
	Size       int64  `json:"size"`
	Mode       uint32 `json:"mode"`
	Mtime      string `json:"mtime"`
	LinkTarget string `json:"linkTarget"`
}

func (c *Client) do(ctx context.Context, method, path string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, method, c.base+path, nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.hc.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode/100 != 2 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		resp.Body.Close()
		return nil, fmt.Errorf("docker %s %s: %d %s", method, path, resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return resp, nil
}

func (c *Client) getJSON(ctx context.Context, path string, v any) error {
	resp, err := c.do(ctx, http.MethodGet, path)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return json.NewDecoder(resp.Body).Decode(v)
}

func containerPath(id, suffix string) string {
	return "/containers/" + url.PathEscape(id) + suffix
}

func (c *Client) Inspect(ctx context.Context, id string, size bool) (Container, error) {
	p := containerPath(id, "/json")
	if size {
		p += "?size=1"
	}
	var ct Container
	err := c.getJSON(ctx, p, &ct)
	return ct, err
}

func (c *Client) Changes(ctx context.Context, id string) ([]Change, error) {
	var ch []Change
	err := c.getJSON(ctx, containerPath(id, "/changes"), &ch)
	return ch, err
}

func (c *Client) ArchiveStat(ctx context.Context, id, path string) (PathStat, error) {
	resp, err := c.do(ctx, http.MethodHead, containerPath(id, "/archive?path="+url.QueryEscape(path)))
	if err != nil {
		return PathStat{}, err
	}
	resp.Body.Close()
	h := resp.Header.Get("X-Docker-Container-Path-Stat")
	raw, err := base64.StdEncoding.DecodeString(h)
	if err != nil {
		// Unsure which base64 alphabet the daemon uses: try the URL one before giving up.
		if raw, err = base64.URLEncoding.DecodeString(h); err != nil {
			return PathStat{}, fmt.Errorf("decode path stat %q: %w", h, err)
		}
	}
	var st PathStat
	return st, json.Unmarshal(raw, &st)
}

func (c *Client) GetArchive(ctx context.Context, id, path string) (io.ReadCloser, error) {
	resp, err := c.do(ctx, http.MethodGet, containerPath(id, "/archive?path="+url.QueryEscape(path)))
	if err != nil {
		return nil, err
	}
	return resp.Body, nil
}
