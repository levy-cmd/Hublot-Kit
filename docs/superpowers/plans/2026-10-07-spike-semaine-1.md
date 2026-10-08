# Spike semaine 1 — Plan d'implémentation

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.
>
> **Revue (préférence de l'utilisateur) :** après chaque tâche, une revue par un sous-agent indépendant **et** une revue Codex (agent `codex:codex-rescue`, en lecture seule). Les remarques sont traitées avant de passer à la tâche suivante.

**Goal:** Répondre, preuves à l'appui, aux quatre questions du spike (§11 de la spec) avant d'écrire le moindre code définitif.

**Architecture:** Un module Go jetable dans `spike/`, séparé du futur module principal. Il contient un client minimal de l'API Docker (bibliothèque standard, socket unix), un paquet `procroot` qui implémente la séquence de §5.1 (pidfd → cgroup → racine → `securejoin`), et trois petits programmes (`lsproc`, `changes`, `archive`) lancés par des scripts de scénario. Le code Linux se compile et se teste dans un conteneur `golang`, puisque le poste de dev est un Mac.

**Tech Stack:** Go (dernière stable), `golang.org/x/sys/unix`, `github.com/cyphar/filepath-securejoin`, Docker Engine 29, Lima (VM Ubuntu et Fedora), bash.

**Spec:** `docs/superpowers/specs/2026-10-07-hublot-kit-v0.1-design.md` (§5.1, §5.2, §11 en priorité)

## Global Constraints

- Code jetable : tout `spike/` est supprimé en fin de spike (tâche 8). Les réponses vivent dans `docs/superpowers/spikes/` et dans la spec.
- Dépendances Go du spike : uniquement `golang.org/x/sys` et `github.com/cyphar/filepath-securejoin`. Pas de SDK Docker : on parle à l'API en HTTP sur le socket.
- Version d'API Docker figée dans le spike : `v1.44` (minimum de Docker Engine 29).
- Dans `procroot`, un chemin *du conteneur* ne passe jamais par `filepath.Join` ni `os.Open` : seulement `securejoin.OpenatInRoot` + `securejoin.Reopen`, puis `fstatat` / `readlinkat` relatifs au descripteur. (Les tests utilisent `filepath.Join` sur des chemins *de l'hôte de test*, c'est voulu.)
- Code Linux (`//go:build linux`) testé par `spike/scripts/test.sh` (conteneur `golang:1`), jamais directement sur macOS.
- Scripts compatibles bash 3.2 (celui de macOS) : pas de tableaux vides avec `set -u`.
- Sorties lisibles par un humain : chaque mesure commence par `RESULT`, chaque témoin par `CONTROL`.

## Review Focus

1. **Process cible lancé sous un autre uid** (image distroless `nonroot`, uid 65532) : avec `CAP_SYS_PTRACE` le listing marche, sans lui l'erreur est explicite (permission refusée), jamais un listing vide silencieux. → Tâche 4 (cible `spike-nonroot`) et tâche 5 (variante sans capacité).
2. **Noms de fichiers bizarres** (espace, emoji, retour à la ligne) : listés tels quels. → Tâche 3 `TestListOddNames`, tâche 4 dossier `/odd`.
3. **Conteneur redémarré entre deux appels** : le nouveau PID est utilisé, rien d'obsolète. → Tâche 4, étape « après redémarrage ».
4. **Système de fichiers en lecture seule** (`docker run --read-only`) : le listing marche. → Tâche 4, cible `spike-nonroot` lancée avec `--read-only`.
5. **Boucle de liens symboliques** : erreur rapide, pas de blocage. → Tâche 3 `TestSymlinkLoopFails`.

## Structure des fichiers

```
spike/
  go.mod, go.sum
  Dockerfile                    image de l'agent du spike (les 3 programmes)
  dockerapi/client.go           client API Docker minimal (Inspect, Changes, ArchiveStat, GetArchive)
  dockerapi/client_test.go
  procroot/cgroup.go            CgroupHasID (sans build tag, testable partout)
  procroot/cgroup_test.go
  procroot/procroot.go          Open : pidfd → cgroup → racine → pidfd vivant   (linux)
  procroot/procroot_test.go     (linux)
  procroot/list.go              List, ReadFile via securejoin                      (linux)
  procroot/list_test.go         tests d'évasion                                     (linux)
  cmd/lsproc/main.go            Q1 et Q3
  cmd/changes/main.go           Q2
  cmd/archive/main.go           Q4
  testdata/target/Dockerfile    conteneur cible distroless avec pièges
  scripts/test.sh               go test dans un conteneur Linux
  scripts/q1-listing.sh
  scripts/q3-perms.sh
  scripts/q2-changes.sh
  scripts/q4-archive.sh
docs/superpowers/spikes/2026-10-xx-resultats-spike.md   (tâche 8)
```

---

### Task 1: Outillage

**Files:** aucun dans le dépôt.

**Interfaces:**
- Consumes: rien
- Produces: `go`, `node` 24, `limactl`, deux VM Lima `hk-ubuntu` et `hk-fedora` avec Docker en mode root.

> **À comprendre :** le spike a besoin de vrais noyaux Linux. Docker Desktop en fournit un (sa VM interne), mais pour AppArmor (Ubuntu) et SELinux (Fedora) il faut les vraies distributions : Lima crée ces VM sur le Mac. Ton VPS peut remplacer la VM Ubuntu si tu préfères.

- [ ] **Step 1: Installer Go, Node 24 et Lima**

```bash
brew install go lima
brew install node@24 && brew link --overwrite --force node@24
```

- [ ] **Step 2: Vérifier**

Run: `go version && node --version && limactl --version && docker version --format '{{.Server.Version}}'`
Expected: une version de Go, `v24.x`, une version de Lima, `29.x`.

- [ ] **Step 3: Créer la VM Ubuntu avec Docker en mode root**

```bash
limactl start --name=hk-ubuntu --tty=false template://docker-rootful
limactl shell hk-ubuntu -- sudo docker version --format '{{.Server.Version}}'
```
Expected: `29.x`. Si le modèle n'existe pas sous ce nom : `limactl create --list-templates` et prendre le modèle Docker « rootful ».

- [ ] **Step 4: Créer la VM Fedora et y installer Docker**

```bash
limactl start --name=hk-fedora --tty=false template://fedora
limactl shell hk-fedora -- bash -c 'curl -fsSL https://get.docker.com | sudo sh && sudo systemctl enable --now docker'
limactl shell hk-fedora -- bash -c 'getenforce && sudo docker version --format "{{.Server.Version}}"'
```
Expected: `Enforcing` puis `29.x`.

- [ ] **Step 5: Noter l'heure de début du spike**

Écrire l'heure dans un coin : la tâche 8 mesure le temps réel passé (recalage du planning).

---

### Task 2: Module du spike et client API Docker minimal

**Files:**
- Create: `spike/go.mod`, `spike/dockerapi/client.go`, `spike/dockerapi/client_test.go`, `spike/scripts/test.sh`

**Interfaces:**
- Consumes: rien
- Produces (paquet `dockerapi`) :
  - `func New(socket string) *Client`
  - `func (c *Client) Inspect(ctx context.Context, id string, size bool) (Container, error)`
  - `func (c *Client) Changes(ctx context.Context, id string) ([]Change, error)`
  - `func (c *Client) ArchiveStat(ctx context.Context, id, path string) (PathStat, error)`
  - `func (c *Client) GetArchive(ctx context.Context, id, path string) (io.ReadCloser, error)`
  - `type Container struct { ID string; Name string; State State; SizeRootFs *int64 }`
  - `type State struct { Running bool; Pid int; StartedAt, FinishedAt string }`
  - `type Change struct { Path string; Kind int }` (0 modifié, 1 ajouté, 2 supprimé)
  - `type PathStat struct { Name string; Size int64; Mode uint32; Mtime, LinkTarget string }`

> **À comprendre :** en Go, un `http.Client` peut parler à un socket unix si on remplace sa fonction de connexion (`DialContext`). L'hôte de l'URL (`http://docker`) est alors ignoré. Les tests montent un faux Docker avec `httptest` sur un socket temporaire.

- [ ] **Step 1: Créer le module et le script de test**

```bash
mkdir -p spike/scripts && cd spike
go mod init github.com/levy-cmd/hublot-kit/spike
go get golang.org/x/sys@latest github.com/cyphar/filepath-securejoin@latest
```

`spike/scripts/test.sh` :

```bash
#!/usr/bin/env bash
# Runs the spike's Go tests inside a Linux container (the code is Linux-only).
set -euo pipefail
cd "$(dirname "$0")/.."
docker run --rm -v "$PWD":/src -v hublot-gomod:/go/pkg/mod -w /src golang:1 go test ./... "$@"
```

Run: `chmod +x spike/scripts/test.sh`

- [ ] **Step 2: Écrire les tests qui échouent**

`spike/dockerapi/client_test.go` :

```go
package dockerapi

import (
	"context"
	"encoding/base64"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

// fakeDocker serves h on a unix socket and returns a Client pointed at it.
func fakeDocker(t *testing.T, h http.HandlerFunc) *Client {
	t.Helper()
	sock := filepath.Join(t.TempDir(), "d.sock")
	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewUnstartedServer(h)
	srv.Listener.Close()
	srv.Listener = ln
	srv.Start()
	t.Cleanup(srv.Close)
	return New(sock)
}

func TestInspect(t *testing.T) {
	c := fakeDocker(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1.44/containers/web/json" || r.URL.Query().Get("size") != "1" {
			t.Errorf("unexpected request %s", r.URL)
		}
		w.Write([]byte(`{"Id":"abc123","Name":"/web","State":{"Running":true,"Pid":42,"StartedAt":"t0","FinishedAt":""},"SizeRootFs":2048}`))
	})
	ct, err := c.Inspect(context.Background(), "web", true)
	if err != nil {
		t.Fatal(err)
	}
	if ct.ID != "abc123" || ct.State.Pid != 42 || !ct.State.Running || ct.SizeRootFs == nil || *ct.SizeRootFs != 2048 {
		t.Fatalf("bad decode: %+v", ct)
	}
}

func TestInspectErrorCarriesStatus(t *testing.T) {
	c := fakeDocker(t, func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"message":"No such container: nope"}`, http.StatusNotFound)
	})
	_, err := c.Inspect(context.Background(), "nope", false)
	if err == nil || !strings.Contains(err.Error(), "404") {
		t.Fatalf("want 404 error, got %v", err)
	}
}

func TestChanges(t *testing.T) {
	c := fakeDocker(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`[{"Path":"/added.txt","Kind":1},{"Path":"/etc/issue","Kind":2}]`))
	})
	ch, err := c.Changes(context.Background(), "web")
	if err != nil {
		t.Fatal(err)
	}
	if len(ch) != 2 || ch[0].Path != "/added.txt" || ch[0].Kind != 1 || ch[1].Kind != 2 {
		t.Fatalf("bad decode: %+v", ch)
	}
}

func TestArchiveStat(t *testing.T) {
	stat := base64.StdEncoding.EncodeToString([]byte(`{"name":"os-release","size":12,"mode":420,"mtime":"m","linkTarget":""}`))
	c := fakeDocker(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodHead || r.URL.Query().Get("path") != "/etc/os-release" {
			t.Errorf("unexpected request %s %s", r.Method, r.URL)
		}
		w.Header().Set("X-Docker-Container-Path-Stat", stat)
	})
	st, err := c.ArchiveStat(context.Background(), "web", "/etc/os-release")
	if err != nil {
		t.Fatal(err)
	}
	if st.Name != "os-release" || st.Size != 12 || st.Mode != 420 {
		t.Fatalf("bad decode: %+v", st)
	}
}
```

- [ ] **Step 3: Vérifier qu'ils échouent**

Run: `spike/scripts/test.sh ./dockerapi/`
Expected: FAIL à la compilation (`undefined: New`).

- [ ] **Step 4: Écrire le client**

`spike/dockerapi/client.go` :

```go
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
```

- [ ] **Step 5: Vérifier que les tests passent**

Run: `spike/scripts/test.sh ./dockerapi/ -v`
Expected: PASS pour les 4 tests.

- [ ] **Step 6: Commit**

```bash
git add spike/go.mod spike/go.sum spike/dockerapi spike/scripts/test.sh
git commit -m "spike: client API Docker minimal sur socket unix"
```

---

### Task 3: `procroot` — ouvrir la racine d'un conteneur et lister sans s'échapper

**Files:**
- Create: `spike/procroot/cgroup.go`, `spike/procroot/cgroup_test.go`, `spike/procroot/procroot.go`, `spike/procroot/procroot_test.go`, `spike/procroot/list.go`, `spike/procroot/list_test.go`

**Interfaces:**
- Consumes: rien (n'utilise pas `dockerapi`)
- Produces (paquet `procroot`) :
  - `var ErrChanged error`
  - `func CgroupHasID(cgroup, id string) bool`
  - `func Open(pid int, containerID string) (*os.File, error)` — linux
  - `type Entry struct { Name, Type string; Size int64; Mode fs.FileMode; LinkTarget string }` — `Type` ∈ `file`, `dir`, `symlink`, `other`
  - `func List(root *os.File, dir string, max int) ([]Entry, bool, error)` — linux
  - `func ReadFile(root *os.File, path string) ([]byte, error)` — linux, plafonné à 1 Mio

> **À comprendre :** c'est la tâche la plus importante du spike. Les tests d'évasion s'écrivent **avant** le code : chacun prouve d'abord que le piège est réel (témoin : la lecture naïve s'échappe), puis que `securejoin` le déjoue. Un fichier qui commence par `//go:build linux` n'est compilé que pour Linux.

- [ ] **Step 1: Tests qui échouent — cgroup**

`spike/procroot/cgroup_test.go` :

```go
package procroot

import (
	"strings"
	"testing"
)

func TestCgroupHasID(t *testing.T) {
	id := strings.Repeat("ab", 32)
	cases := []struct {
		cgroup string
		want   bool
	}{
		{"0::/system.slice/docker-" + id + ".scope\n", true},
		{"0::/docker/" + id + "\n", true},
		{"0::/../docker-" + id + ".scope\n", true}, // vu depuis un espace de noms cgroup privé
		{"0::/\n", false},
		{"0::/docker/" + id[:12] + "\n", false},
	}
	for _, c := range cases {
		if got := CgroupHasID(c.cgroup, id); got != c.want {
			t.Errorf("CgroupHasID(%q) = %v, want %v", c.cgroup, got, c.want)
		}
	}
	if CgroupHasID("0::/docker/abc", "abc") {
		t.Error("a short id must never match")
	}
}
```

- [ ] **Step 2: Tests qui échouent — évasion et listing**

`spike/procroot/list_test.go` :

```go
//go:build linux

package procroot

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

// mkRoot creates a fake container root and, outside it, a host marker file.
func mkRoot(t *testing.T) (root *os.File, rootPath, marker string) {
	t.Helper()
	marker = filepath.Join(t.TempDir(), "MARKER")
	must(t, os.WriteFile(marker, []byte("HOST-SECRET"), 0o644))
	rootPath = t.TempDir()
	root, err := os.Open(rootPath)
	must(t, err)
	t.Cleanup(func() { root.Close() })
	return root, rootPath, marker
}

func assertNoEscape(t *testing.T, b []byte, err error) {
	t.Helper()
	if err == nil || strings.Contains(string(b), "HOST-SECRET") {
		t.Fatalf("ESCAPE: read %q err=%v", b, err)
	}
}

func TestReadFileAbsoluteSymlinkStaysInRoot(t *testing.T) {
	root, rp, marker := mkRoot(t)
	must(t, os.Symlink(marker, filepath.Join(rp, "evil")))
	if b, err := os.ReadFile(filepath.Join(rp, "evil")); err != nil || string(b) != "HOST-SECRET" {
		t.Fatalf("control: the naive read should escape, got %q %v", b, err)
	}
	b, err := ReadFile(root, "evil")
	assertNoEscape(t, b, err)
}

func TestReadFileRelativeSymlinkStaysInRoot(t *testing.T) {
	root, rp, marker := mkRoot(t)
	target := strings.Repeat("../", 20) + strings.TrimPrefix(marker, "/")
	must(t, os.Symlink(target, filepath.Join(rp, "evil-rel")))
	if b, err := os.ReadFile(filepath.Join(rp, "evil-rel")); err != nil || string(b) != "HOST-SECRET" {
		t.Fatalf("control: the naive read should escape, got %q %v", b, err)
	}
	b, err := ReadFile(root, "evil-rel")
	assertNoEscape(t, b, err)
}

func TestReadFileDotDotPathStaysInRoot(t *testing.T) {
	root, _, marker := mkRoot(t)
	b, err := ReadFile(root, strings.Repeat("../", 20)+strings.TrimPrefix(marker, "/"))
	assertNoEscape(t, b, err)
}

func TestSymlinkLoopFails(t *testing.T) {
	root, rp, _ := mkRoot(t)
	must(t, os.Symlink("loop2", filepath.Join(rp, "loop1")))
	must(t, os.Symlink("loop1", filepath.Join(rp, "loop2")))
	if _, err := ReadFile(root, "loop1"); err == nil {
		t.Fatal("a symlink loop must fail")
	}
}

func TestListTypesAndLinkTarget(t *testing.T) {
	root, rp, _ := mkRoot(t)
	must(t, os.WriteFile(filepath.Join(rp, "a.txt"), []byte("hello"), 0o640))
	must(t, os.Mkdir(filepath.Join(rp, "dir"), 0o755))
	must(t, os.Symlink("/etc/passwd", filepath.Join(rp, "link")))

	entries, truncated, err := List(root, "/", 100)
	must(t, err)
	if truncated {
		t.Fatal("unexpected truncation")
	}
	got := map[string]Entry{}
	for _, e := range entries {
		got[e.Name] = e
	}
	if e := got["a.txt"]; e.Type != "file" || e.Size != 5 || e.Mode != 0o640 {
		t.Errorf("a.txt: %+v", e)
	}
	if e := got["dir"]; e.Type != "dir" {
		t.Errorf("dir: %+v", e)
	}
	if e := got["link"]; e.Type != "symlink" || e.LinkTarget != "/etc/passwd" {
		t.Errorf("link: %+v (the link must be shown, never followed)", e)
	}
}

func TestListOddNames(t *testing.T) {
	root, rp, _ := mkRoot(t)
	names := []string{"with space", "émoji-🎉", "new\nline"}
	for _, n := range names {
		must(t, os.WriteFile(filepath.Join(rp, n), nil, 0o644))
	}
	entries, _, err := List(root, "/", 100)
	must(t, err)
	seen := map[string]bool{}
	for _, e := range entries {
		seen[e.Name] = true
	}
	for _, n := range names {
		if !seen[n] {
			t.Errorf("missing %q", n)
		}
	}
}

func TestListTruncates(t *testing.T) {
	root, rp, _ := mkRoot(t)
	for i := 0; i < 15; i++ {
		must(t, os.WriteFile(filepath.Join(rp, "f"+strings.Repeat("x", i)), nil, 0o644))
	}
	entries, truncated, err := List(root, "/", 10)
	must(t, err)
	if len(entries) != 10 || !truncated {
		t.Fatalf("want 10 entries truncated, got %d truncated=%v", len(entries), truncated)
	}
	entries, truncated, err = List(root, "/", 15)
	must(t, err)
	if len(entries) != 15 || truncated {
		t.Fatalf("want 15 entries not truncated, got %d truncated=%v", len(entries), truncated)
	}
}
```

`spike/procroot/procroot_test.go` :

```go
//go:build linux

package procroot

import (
	"errors"
	"os"
	"strings"
	"testing"
)

func TestOpenRejectsForeignCgroup(t *testing.T) {
	_, err := Open(os.Getpid(), strings.Repeat("a", 64))
	if !errors.Is(err, ErrChanged) {
		t.Fatalf("want ErrChanged, got %v", err)
	}
}

func TestOpenRejectsMissingPid(t *testing.T) {
	_, err := Open(0x7ffffffe, strings.Repeat("a", 64))
	if !errors.Is(err, ErrChanged) {
		t.Fatalf("want ErrChanged, got %v", err)
	}
}
```

- [ ] **Step 3: Vérifier qu'ils échouent**

Run: `spike/scripts/test.sh ./procroot/`
Expected: FAIL à la compilation (`undefined: CgroupHasID`, `ReadFile`, `List`, `Open`).

- [ ] **Step 4: Vérifier l'API réelle de `securejoin` avant de coder**

Run: `cd spike && GOOS=linux go doc github.com/cyphar/filepath-securejoin OpenatInRoot && GOOS=linux go doc github.com/cyphar/filepath-securejoin Reopen`
Expected: `func OpenatInRoot(root *os.File, unsafePath string) (*os.File, error)` et `func Reopen(handle *os.File, flags int) (*os.File, error)`. Si la doc indique que ces fonctions ont migré vers le sous-paquet `pathrs-lite`, importer ce sous-paquet à la place dans `list.go` : les signatures restent les mêmes. **Noter ce que tu vois** : c'est une réponse du spike (tâche 8).

- [ ] **Step 5: Écrire `cgroup.go`**

```go
package procroot

import (
	"errors"
	"strings"
)

// ErrChanged means the process no longer is (or never was) the container's main process.
var ErrChanged = errors.New("container changed")

// CgroupHasID reports whether a /proc/<pid>/cgroup content belongs to the container
// with this full 64-character ID.
func CgroupHasID(cgroup, id string) bool {
	return len(id) == 64 && strings.Contains(cgroup, id)
}
```

- [ ] **Step 6: Écrire `procroot.go`**

```go
//go:build linux

package procroot

import (
	"fmt"
	"os"

	"golang.org/x/sys/unix"
)

// Open returns an O_PATH handle on the root filesystem of the container whose
// main process is pid. Sequence from spec §5.1: pidfd → cgroup → root → pidfd alive.
func Open(pid int, containerID string) (*os.File, error) {
	pidfd, err := unix.PidfdOpen(pid, 0)
	if err != nil {
		return nil, fmt.Errorf("%w: pidfd_open(%d): %v", ErrChanged, pid, err)
	}
	defer unix.Close(pidfd)

	cg, err := os.ReadFile(fmt.Sprintf("/proc/%d/cgroup", pid))
	if err != nil {
		return nil, fmt.Errorf("%w: read cgroup: %v", ErrChanged, err)
	}
	if !CgroupHasID(string(cg), containerID) {
		return nil, fmt.Errorf("%w: pid %d is not in the container's cgroup", ErrChanged, pid)
	}

	fd, err := unix.Open(fmt.Sprintf("/proc/%d/root", pid), unix.O_PATH|unix.O_DIRECTORY|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, fmt.Errorf("open root of pid %d: %w", pid, err)
	}
	// Still alive => the PID was not recycled between pidfd_open and the open above.
	if err := unix.PidfdSendSignal(pidfd, 0, nil, 0); err != nil {
		unix.Close(fd)
		return nil, fmt.Errorf("%w: process gone: %v", ErrChanged, err)
	}
	return os.NewFile(uintptr(fd), "container-root"), nil
}
```

- [ ] **Step 7: Écrire `list.go`**

```go
//go:build linux

package procroot

import (
	"errors"
	"io"
	"io/fs"
	"os"

	securejoin "github.com/cyphar/filepath-securejoin"
	"golang.org/x/sys/unix"
)

type Entry struct {
	Name       string
	Type       string // file, dir, symlink, other
	Size       int64
	Mode       fs.FileMode
	LinkTarget string // raw, never resolved
}

// openIn resolves path inside root (symlinks included) and reopens it with flags.
func openIn(root *os.File, path string, flags int) (*os.File, error) {
	h, err := securejoin.OpenatInRoot(root, path)
	if err != nil {
		return nil, err
	}
	defer h.Close()
	return securejoin.Reopen(h, flags|unix.O_CLOEXEC)
}

// List returns at most max entries of dir. Entries are stat'ed relative to the
// directory's descriptor, one path component at a time: no symlink is followed.
func List(root *os.File, dir string, max int) ([]Entry, bool, error) {
	d, err := openIn(root, dir, unix.O_RDONLY|unix.O_DIRECTORY)
	if err != nil {
		return nil, false, err
	}
	defer d.Close()

	names, err := d.Readdirnames(max + 1)
	if err != nil && !errors.Is(err, io.EOF) {
		return nil, false, err
	}
	truncated := len(names) > max
	if truncated {
		names = names[:max]
	}

	dfd := int(d.Fd())
	out := make([]Entry, 0, len(names))
	for _, n := range names {
		var st unix.Stat_t
		if err := unix.Fstatat(dfd, n, &st, unix.AT_SYMLINK_NOFOLLOW); err != nil {
			continue // removed while listing
		}
		e := Entry{Name: n, Size: st.Size, Mode: fs.FileMode(st.Mode & 0o777)}
		switch st.Mode & unix.S_IFMT {
		case unix.S_IFDIR:
			e.Type = "dir"
		case unix.S_IFREG:
			e.Type = "file"
		case unix.S_IFLNK:
			e.Type = "symlink"
			buf := make([]byte, 4096)
			if k, err := unix.Readlinkat(dfd, n, buf); err == nil {
				e.LinkTarget = string(buf[:k])
			}
		default:
			e.Type = "other"
		}
		out = append(out, e)
	}
	return out, truncated, nil
}

// ReadFile reads at most 1 MiB of path, resolved inside root.
func ReadFile(root *os.File, path string) ([]byte, error) {
	f, err := openIn(root, path, unix.O_RDONLY)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return io.ReadAll(io.LimitReader(f, 1<<20))
}
```

- [ ] **Step 8: Vérifier que tous les tests passent**

Run: `spike/scripts/test.sh ./procroot/ -v`
Expected: PASS pour les 10 tests, y compris les trois témoins d'évasion. Un échec sur un test `...StaysInRoot` est un **arrêt du spike** : ne pas continuer, revenir à la spec §5.1.

- [ ] **Step 9: Commit**

```bash
git add spike/procroot spike/go.mod spike/go.sum
git commit -m "spike: procroot, racine du conteneur par pidfd et listing via securejoin"
```

---

### Task 4: Q1 — lister un vrai conteneur distroless

**Files:**
- Create: `spike/cmd/lsproc/main.go`, `spike/Dockerfile`, `spike/testdata/target/Dockerfile`, `spike/scripts/q1-listing.sh`

**Interfaces:**
- Consumes: `dockerapi.New`, `(*Client).Inspect`, `procroot.Open`, `procroot.List`, `procroot.ReadFile`
- Produces: image `hublot-spike` (binaires `/usr/local/bin/lsproc`, `changes`, `archive`), images cibles `hublot-spike-target` et `hublot-spike-target-nonroot`, script `q1-listing.sh` qui lit la variable `AGENT_FLAGS` (utilisé par la tâche 5)

> **À comprendre :** l'agent du spike tourne lui-même dans un conteneur avec `--pid=host`, comme le fera Hublot-Kit. Le fichier « secret » est monté dans *l'agent* : un lien absolu mal résolu le lirait, c'est exactement le piège de la spec.

- [ ] **Step 1: Écrire `lsproc`**

`spike/cmd/lsproc/main.go` :

```go
//go:build linux

// lsproc lists a directory of a running container through /proc/<pid>/root.
// Usage: lsproc <container> <dir> [file...]
package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/levy-cmd/hublot-kit/spike/dockerapi"
	"github.com/levy-cmd/hublot-kit/spike/procroot"
)

func fatal(format string, a ...any) {
	fmt.Fprintf(os.Stderr, "FATAL "+format+"\n", a...)
	os.Exit(1)
}

func short(b []byte) string {
	s := string(b)
	if len(s) > 80 {
		s = s[:80] + "…"
	}
	return s
}

func main() {
	if len(os.Args) < 3 {
		fatal("usage: lsproc <container> <dir> [file...]")
	}
	ctx := context.Background()
	ct, err := dockerapi.New("/var/run/docker.sock").Inspect(ctx, os.Args[1], false)
	if err != nil {
		fatal("inspect: %v", err)
	}
	if !ct.State.Running {
		fatal("container is not running")
	}
	cg, _ := os.ReadFile(fmt.Sprintf("/proc/%d/cgroup", ct.State.Pid))
	fmt.Printf("container %s pid=%d cgroup=%q\n", ct.ID[:12], ct.State.Pid, strings.TrimSpace(string(cg)))

	start := time.Now()
	root, err := procroot.Open(ct.State.Pid, ct.ID)
	if err != nil {
		fatal("open root: %v", err)
	}
	defer root.Close()
	entries, truncated, err := procroot.List(root, os.Args[2], 10000)
	if err != nil {
		fatal("list %s: %v", os.Args[2], err)
	}
	fmt.Printf("RESULT list %s: %d entries (truncated=%v) in %s\n", os.Args[2], len(entries), truncated, time.Since(start))
	for _, e := range entries[:min(5, len(entries))] {
		fmt.Printf("  %-7s %s %6d %q -> %q\n", e.Type, e.Mode, e.Size, e.Name, e.LinkTarget)
	}

	for _, f := range os.Args[3:] {
		data, err := procroot.ReadFile(root, f)
		fmt.Printf("RESULT securejoin read %s: %q err=%v\n", f, short(data), err)
		naive, err := os.ReadFile(fmt.Sprintf("/proc/%d/root/%s", ct.State.Pid, strings.TrimPrefix(f, "/")))
		fmt.Printf("CONTROL naive read %s: %q err=%v\n", f, short(naive), err)
	}
}
```

- [ ] **Step 2: Image de l'agent du spike**

`spike/Dockerfile` :

```dockerfile
FROM golang:1 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /out/ ./cmd/...

FROM gcr.io/distroless/static-debian12
COPY --from=build /out/ /usr/local/bin/
```

- [ ] **Step 3: Image cible avec ses pièges**

`spike/testdata/target/Dockerfile` :

```dockerfile
ARG BASE=gcr.io/distroless/static-debian12

FROM golang:1 AS sleeper
RUN printf 'package main\nimport "time"\nfunc main() { for { time.Sleep(time.Hour) } }\n' > /m.go \
 && CGO_ENABLED=0 go build -o /sleep /m.go

FROM busybox:1.36 AS tree
RUN mkdir -p /out/usr/many /out/odd /out/tmp \
 && i=1; while [ $i -le 1000 ]; do touch /out/usr/many/f$i; i=$((i+1)); done \
 && touch "/out/odd/with space" "/out/odd/émoji-🎉" \
 && ln -s /tmp/HOST_MARKER /out/evil \
 && ln -s ../../../../../tmp/HOST_MARKER /out/evil-rel

FROM ${BASE}
COPY --from=tree /out/ /
COPY --from=sleeper /sleep /sleep
ENTRYPOINT ["/sleep"]
```

- [ ] **Step 4: Script du scénario Q1**

`spike/scripts/q1-listing.sh` :

```bash
#!/usr/bin/env bash
# Q1: list containers through /proc/<pid>/root, with symlink traps.
# AGENT_FLAGS: extra flags for the agent container (default: --cap-add SYS_PTRACE).
set -euo pipefail
cd "$(dirname "$0")/.."
AGENT_FLAGS="${AGENT_FLAGS-"--cap-add SYS_PTRACE"}"
MARK=/tmp/hublot-spike
mkdir -p "$MARK" && echo HOST-SECRET > "$MARK/marker"

docker build -q -t hublot-spike . >/dev/null
docker build -q -t hublot-spike-target testdata/target >/dev/null
docker build -q -t hublot-spike-target-nonroot --build-arg BASE=gcr.io/distroless/static-debian12:nonroot testdata/target >/dev/null
docker rm -f spike-root spike-nonroot >/dev/null 2>&1 || true
docker run -d --name spike-root hublot-spike-target >/dev/null
docker run -d --name spike-nonroot --read-only hublot-spike-target-nonroot >/dev/null

run_agent() {
  # shellcheck disable=SC2086  # AGENT_FLAGS is split on purpose
  docker run --rm --pid=host $AGENT_FLAGS \
    -v /var/run/docker.sock:/var/run/docker.sock \
    -v "$MARK/marker:/tmp/HOST_MARKER:ro" \
    hublot-spike lsproc "$@"
}

echo "== cible root";                       run_agent spike-root /usr/many evil evil-rel
echo "== noms bizarres";                    run_agent spike-root /odd
echo "== cible nonroot, --read-only";       run_agent spike-nonroot /usr/many evil
echo "== après redémarrage (nouveau PID)";  docker restart -t 0 spike-root >/dev/null; run_agent spike-root /usr/many
```

Run: `chmod +x spike/scripts/q1-listing.sh`

- [ ] **Step 5: Lancer sur Docker Desktop**

Run: `mkdir -p /tmp/hublot-spike && spike/scripts/q1-listing.sh 2>&1 | tee /tmp/hublot-spike/q1-docker-desktop.txt`
Expected :
- `RESULT list /usr/many: 1000 entries (truncated=false) in …` avec une durée **inférieure à 1 s**, pour les cibles root et nonroot.
- `RESULT securejoin read evil: "" err=…no such file…` (pas de `HOST-SECRET`).
- `CONTROL naive read evil: "HOST-SECRET"` : le piège est réel.
- `/odd` liste `"with space"` et `"émoji-🎉"`.
- Après redémarrage : un PID différent du premier, listing correct.
- La ligne `cgroup=` contient l'ID complet du conteneur.

Si `FATAL open root: ... ErrChanged ... not in the container's cgroup` : recopier la ligne `cgroup=` dans les notes, c'est la réponse de la question « l'ID apparaît-il dans le cgroup ? ». Ne pas affaiblir `CgroupHasID` sans revenir à la spec.

- [ ] **Step 6: Commit**

```bash
git add spike/cmd/lsproc spike/Dockerfile spike/testdata spike/scripts/q1-listing.sh
git commit -m "spike: Q1, listing d'un conteneur distroless via /proc"
```

---

### Task 5: Q3 — droits minimaux sur Ubuntu (AppArmor) et Fedora (SELinux)

**Files:**
- Create: `spike/scripts/q3-perms.sh`

**Interfaces:**
- Consumes: `q1-listing.sh` et sa variable `AGENT_FLAGS`, l'image `hublot-spike`
- Produces: un tableau variante → OK / ÉCHEC par distribution, dans `/tmp/hublot-spike/q3-<hôte>.txt`

> **À comprendre :** on part du minimum et on ajoute des droits un par un. La première variante qui marche est le « jeu de droits minimal » à documenter. La variante sans aucune capacité doit **échouer** sur la cible nonroot : c'est la preuve que `CAP_SYS_PTRACE` est nécessaire.

- [ ] **Step 1: Écrire le script**

`spike/scripts/q3-perms.sh` :

```bash
#!/usr/bin/env bash
# Q3: try permission variants from weakest to strongest, then the host binary.
set -euo pipefail
cd "$(dirname "$0")/.."
OUT=/tmp/hublot-spike; mkdir -p "$OUT"
REPORT="$OUT/q3-$(hostname).txt"; : > "$REPORT"

try() {
  local label="$1" flags="$2" log="$OUT/q3-$(hostname)-$1.log"
  if AGENT_FLAGS="$flags" ./scripts/q1-listing.sh >"$log" 2>&1 \
     && grep -q "RESULT list /usr/many: 1000" "$log" \
     && [ "$(grep -c 'RESULT list /usr/many: 1000' "$log")" -eq 3 ] \
     && ! grep -q "securejoin read.*HOST-SECRET" "$log"; then
    echo "OK     $label ($flags)" | tee -a "$REPORT"
  else
    echo "ECHEC  $label ($flags) -> voir $log" | tee -a "$REPORT"
  fi
}

try 1-aucun          ""
try 2-ptrace         "--cap-add SYS_PTRACE"
try 3-ptrace-aa      "--cap-add SYS_PTRACE --security-opt apparmor=unconfined"
try 4-ptrace-label   "--cap-add SYS_PTRACE --security-opt label=disable"
try 5-privileged     "--privileged"

# Host binary, run as root, no container around the agent.
docker rm -f q3-bin >/dev/null 2>&1 || true
docker create --name q3-bin hublot-spike >/dev/null
docker cp q3-bin:/usr/local/bin/lsproc "$OUT/lsproc" >/dev/null
docker rm q3-bin >/dev/null
echo HOST-SECRET | sudo tee /tmp/HOST_MARKER >/dev/null
if sudo "$OUT/lsproc" spike-nonroot /usr/many evil >"$OUT/q3-$(hostname)-binaire.log" 2>&1 \
   && grep -q "RESULT list /usr/many: 1000" "$OUT/q3-$(hostname)-binaire.log"; then
  echo "OK     6-binaire-hote (root, systemd)" | tee -a "$REPORT"
else
  echo "ECHEC  6-binaire-hote -> voir $OUT/q3-$(hostname)-binaire.log" | tee -a "$REPORT"
fi
```

Run: `chmod +x spike/scripts/q3-perms.sh`

- [ ] **Step 2: Lancer sur Ubuntu**

Run: `limactl shell hk-ubuntu -- sudo bash "$PWD/spike/scripts/q3-perms.sh"`
Expected: `1-aucun` en ÉCHEC (sinon, noter pourquoi : la cible nonroot devrait l'imposer), au moins une variante entre 2 et 4 en OK, `6-binaire-hote` en OK. Copier le rapport : `limactl shell hk-ubuntu -- cat /tmp/hublot-spike/q3-*.txt`.

- [ ] **Step 3: Lancer sur Fedora, SELinux appliqué aux conteneurs**

Par défaut, Docker sur Fedora n'étiquette pas les conteneurs SELinux. On active l'étiquetage pour tester le cas réaliste :

```bash
limactl shell hk-fedora -- bash -c 'echo "{\"selinux-enabled\": true}" | sudo tee /etc/docker/daemon.json && sudo systemctl restart docker'
limactl shell hk-fedora -- sudo bash "$PWD/spike/scripts/q3-perms.sh"
```
Expected: même lecture qu'Ubuntu. Si seule la variante `5-privileged` marche, c'est un résultat important : noter les refus dans `sudo ausearch -m avc -ts recent`.

- [ ] **Step 4: Commit**

```bash
git add spike/scripts/q3-perms.sh
git commit -m "spike: Q3, matrice des droits minimaux"
```

---

### Task 6: Q2 — `/changes` sur containerd et sur overlay2

**Files:**
- Create: `spike/cmd/changes/main.go`, `spike/scripts/q2-changes.sh`

**Interfaces:**
- Consumes: `dockerapi.New`, `(*Client).Changes`
- Produces: `/tmp/hublot-spike/changes-<driver>.txt` par mode de stockage

- [ ] **Step 1: Écrire `changes`**

`spike/cmd/changes/main.go` :

```go
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
```

- [ ] **Step 2: Écrire le script**

`spike/scripts/q2-changes.sh` :

```bash
#!/usr/bin/env bash
# Q2: same manipulations, then /changes, on the current storage backend.
set -euo pipefail
cd "$(dirname "$0")/.."
OUT=/tmp/hublot-spike; mkdir -p "$OUT"
DRIVER=$(docker info -f '{{.Driver}}')
docker build -q -t hublot-spike . >/dev/null
docker rm -f spike-changes >/dev/null 2>&1 || true
docker run -d --name spike-changes alpine:3 \
  sh -c 'echo added > /added.txt; echo changed >> /etc/profile; rm -f /etc/issue; sleep 3600' >/dev/null
sleep 2
docker restart -t 0 spike-changes >/dev/null            # changes must survive a restart
docker exec spike-changes sh -c 'echo second > /second-run.txt'
docker stop -t 0 spike-changes >/dev/null                # /changes must work on a stopped container
docker run --rm -v /var/run/docker.sock:/var/run/docker.sock hublot-spike changes spike-changes \
  | tee "$OUT/changes-$DRIVER.txt"
echo "driver=$DRIVER"
```

Run: `chmod +x spike/scripts/q2-changes.sh`

- [ ] **Step 3: Lancer sur le stockage containerd (défaut d'une installation Docker 29 neuve)**

Run: `limactl shell hk-ubuntu -- sudo bash "$PWD/spike/scripts/q2-changes.sh"`
Expected: au minimum `A /added.txt`, `A /second-run.txt`, `C /etc`, `C /etc/profile`, `D /etc/issue`. Noter le nom du driver affiché.

- [ ] **Step 4: Basculer sur overlay2 et relancer**

```bash
limactl shell hk-ubuntu -- bash -c 'echo "{\"features\":{\"containerd-snapshotter\":false}}" | sudo tee /etc/docker/daemon.json && sudo systemctl restart docker && sudo docker info -f "{{.Driver}}"'
limactl shell hk-ubuntu -- sudo bash "$PWD/spike/scripts/q2-changes.sh"
```
Expected: le driver affiché change (`overlay2`). Les images sont re-téléchargées : c'est normal, les deux stockages sont séparés.

- [ ] **Step 5: Comparer**

Run: `limactl shell hk-ubuntu -- bash -c 'cd /tmp/hublot-spike && ls changes-*.txt && diff changes-*.txt && echo IDENTIQUES'`
Expected: `IDENTIQUES`. Toute différence est recopiée telle quelle dans les résultats.

- [ ] **Step 6: Remettre la VM dans son état d'origine**

Run: `limactl shell hk-ubuntu -- bash -c 'sudo rm /etc/docker/daemon.json && sudo systemctl restart docker'`

- [ ] **Step 7: Commit**

```bash
git add spike/cmd/changes spike/scripts/q2-changes.sh
git commit -m "spike: Q2, /changes sur containerd et overlay2"
```

---

### Task 7: Q4 — conteneur arrêté : `HEAD`, `GET /archive` et volumes

**Files:**
- Create: `spike/cmd/archive/main.go`, `spike/scripts/q4-archive.sh`

**Interfaces:**
- Consumes: `dockerapi.New`, `(*Client).Inspect` (avec `size`), `(*Client).ArchiveStat`, `(*Client).GetArchive`
- Produces: mesures de l'index tar : entrées, octets, durée, présence du fichier de volume

> **À comprendre :** `archive/tar` lit une archive entrée par entrée avec `Next()`. Sur un flux réseau, on ne peut pas sauter le contenu : il est lu et jeté. C'est ce que mesure ce test.

- [ ] **Step 1: Écrire `archive`**

`spike/cmd/archive/main.go` :

```go
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
```

- [ ] **Step 2: Écrire le script**

`spike/scripts/q4-archive.sh` :

```bash
#!/usr/bin/env bash
# Q4: stopped container with a named volume: HEAD/GET archive and full index.
set -euo pipefail
cd "$(dirname "$0")/.."
docker build -q -t hublot-spike . >/dev/null
docker rm -f spike-q4 >/dev/null 2>&1 || true
docker volume rm spike-vol >/dev/null 2>&1 || true
docker volume create spike-vol >/dev/null
docker run --name spike-q4 -v spike-vol:/data alpine:3 \
  sh -c 'echo vol > /data/in-volume.txt; echo root > /in-root.txt'
docker run --rm -v /var/run/docker.sock:/var/run/docker.sock hublot-spike archive spike-q4
```

Run: `chmod +x spike/scripts/q4-archive.sh`

- [ ] **Step 3: Lancer sur Docker Desktop puis sur Ubuntu**

Run: `spike/scripts/q4-archive.sh && limactl shell hk-ubuntu -- sudo bash "$PWD/spike/scripts/q4-archive.sh"`
Expected :
- `running=false` (le conteneur est bien arrêté).
- `RESULT HEAD /etc/os-release: {Name:os-release …} err=<nil>` : sinon, `Stat` lira l'index (spec §5.2).
- `RESULT GET /etc/os-release: first line "NAME=…"`.
- `RESULT index /: … entries, … bytes, …` : noter les trois chiffres.
- `in-root.txt present=true` ; `in-volume.txt present=` **oui ou non**, c'est la réponse à la question 4.

- [ ] **Step 4: Commit**

```bash
git add spike/cmd/archive spike/scripts/q4-archive.sh
git commit -m "spike: Q4, archive et index sur conteneur arrêté"
```

---

### Task 8: Vérifications d'API, résultats, mise à jour de la spec, suppression du spike

**Files:**
- Create: `docs/superpowers/spikes/<date-du-jour>-resultats-spike.md`
- Modify: `docs/superpowers/specs/2026-10-07-hublot-kit-v0.1-design.md` (§3, §5, §8, §11, §12)
- Delete: `spike/`

**Interfaces:**
- Consumes: toutes les sorties `RESULT` des tâches 4 à 7, la réponse de la tâche 3 step 4
- Produces: une spec à jour, qui sert d'entrée au plan du socle

- [ ] **Step 1: Chemin d'import du SDK Docker**

Run: `cd spike && go list -m -versions github.com/moby/moby/client 2>&1 | tail -c 300; go list -m -versions github.com/docker/docker 2>&1 | tail -c 300`
Expected: l'un des deux (ou les deux) liste des versions. Noter lequel suit Docker 29 : c'est l'import à utiliser dans `internal/dockerx`.

- [ ] **Step 2: Écrire le document de résultats**

`docs/superpowers/spikes/<date-du-jour>-resultats-spike.md` :

```markdown
# Résultats du spike — <date>

Temps passé : <heures> (début <heure de la tâche 1>, fin <maintenant>)

| # | Question | Réponse | Preuve (ligne RESULT recopiée) | Conséquence pour la spec |
|---|---|---|---|---|
| 1 | Listing /proc + securejoin sur distroless | | | |
| 1b | ID du conteneur dans /proc/<pid>/cgroup (agent conteneurisé) | | | |
| 2 | /changes identique sur containerd et overlay2 | | | |
| 3 | Droits minimaux Ubuntu | | | |
| 3 | Droits minimaux Fedora (SELinux actif) | | | |
| 4 | HEAD / GET archive sur conteneur arrêté | | | |
| 4 | Volumes inclus dans GET /archive "/" | | | |
| 5 | API securejoin (OpenatInRoot / Reopen, paquet) | | | |
| 6 | Import du SDK Docker | | | |

Index tar mesuré : <entrées> entrées, <octets> octets, <durée> pour une image alpine.
```

Remplir **chaque** case à partir des sorties réelles. Aucune case vide : « non testé » vaut mieux qu'une supposition.

- [ ] **Step 3: Reporter dans la spec**

- §11 : ajouter une colonne ou une ligne « Résultat » sous le tableau, avec un lien vers le document de résultats.
- §8 : remplacer la commande `docker run` par le jeu de droits minimal trouvé (et la consigne Fedora si elle diffère).
- §3 : remplacer « chemin d'import exact à confirmer au spike » par l'import trouvé.
- §5.1 étape 6 : remplacer « à confirmer au spike » par la version et le paquet trouvés.
- §5.2 étape 5 et §5.2 étape 6 : appliquer le plan B si `HEAD` ou les volumes ont échoué.
- §12 : recaler les semaines à partir du temps réel passé (règle : `semaines restantes × heures estimées / heures réelles`).

- [ ] **Step 4: Supprimer le code jetable et commiter**

```bash
git rm -r -q spike
git add docs/superpowers
git commit -m "spike: résultats consignés, spec mise à jour, code jetable supprimé"
```

- [ ] **Step 5: Arrêter les VM**

Run: `limactl stop hk-ubuntu hk-fedora`
Expected: les deux VM s'arrêtent (on les garde pour les tests d'intégration du socle).

---

**Suite :** une fois ce plan exécuté, on écrit le plan de la phase **Socle** à partir de la spec mise à jour.
