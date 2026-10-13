package files

// Tests of the only writer (spec §5.3, §10), written before the code. An
// in-memory view (fsview.MemFS) plays the container; a fake Docker records
// what PutArchive receives; a recorder keeps the order audit → write → audit.

import (
	"archive/tar"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/levy-cmd/hublot-kit/internal/audit"
	"github.com/levy-cmd/hublot-kit/internal/dockerx"
	"github.com/levy-cmd/hublot-kit/internal/fsview"
)

const cid = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

// ctr is the Inspect of the test container: running, no mount.
var ctr = dockerx.ContainerInfo{ID: cid, Name: "web", Running: true, Pid: 42}

// journal records audit events and PutArchive calls in one ordered list.
type journal struct {
	mu       sync.Mutex
	steps    []string
	events   []audit.Event
	auditErr error             // returned by Write when set
	onEvent  func(audit.Event) // called by Write, outside the lock
}

func (j *journal) Write(e audit.Event) error {
	if j.onEvent != nil {
		j.onEvent(e)
	}
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.auditErr != nil {
		return j.auditErr
	}
	j.steps = append(j.steps, "audit:"+e.Result)
	j.events = append(j.events, e)
	return nil
}

func (j *journal) record(s string) {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.steps = append(j.steps, s)
}

func (j *journal) Steps() []string {
	j.mu.Lock()
	defer j.mu.Unlock()
	return slices.Clone(j.steps)
}

// putter wraps the fake Docker: it records the call in the journal and the
// state of the context it received.
type putter struct {
	*dockerx.Fake
	j      *journal
	ctxErr error
}

func (p *putter) PutArchive(ctx context.Context, id, dir string, r io.Reader) error {
	p.j.record("put:" + dir)
	p.ctxErr = ctx.Err()
	return p.Fake.PutArchive(ctx, id, dir, r)
}

type rig struct {
	w      *Writer
	mem    *fsview.MemFS
	fake   *dockerx.Fake
	put    *putter
	j      *journal
	opens  int
	forgot []string
}

func newRig(t *testing.T, readOnly bool) *rig {
	t.Helper()
	r := &rig{mem: fsview.NewMemFS(), fake: &dockerx.Fake{}, j: &journal{}}
	r.put = &putter{Fake: r.fake, j: r.j}
	r.mem.AddDir("/etc").AddFile("/etc/app.conf", []byte("127.0.0.1 localhost\n"))
	r.mem.Entries["/etc"] = fsview.Entry{Name: "etc", Type: fsview.TypeDir, Mode: 0o755, UID: 7, GID: 8}
	r.mem.Entries["/etc/app.conf"] = fsview.Entry{Name: "app.conf", Type: fsview.TypeFile, Mode: 0o640 | fs.ModeSetgid, UID: 101, GID: 102, Size: 20}
	r.w = &Writer{
		ReadOnly: readOnly,
		Docker:   r.put,
		Audit:    r.j,
		Open: func(_ context.Context, id string) (fsview.FS, error) {
			r.opens++
			if id != cid {
				t.Errorf("vue ouverte sur %q, attendu l'ID complet", id)
			}
			return r.mem, nil
		},
		Forget: func(id string) { r.forgot = append(r.forgot, id); r.j.record("forget:" + id) },
	}
	return r
}

var confTag = ETag([]byte("127.0.0.1 localhost\n"))

// nothingWritten: no audit line, no PutArchive.
func (r *rig) nothingWritten(t *testing.T) {
	t.Helper()
	if s := r.j.Steps(); len(s) != 0 {
		t.Fatalf("rien ne doit être écrit ni journalisé : %v", s)
	}
	if n := len(r.fake.PutCalls()); n != 0 {
		t.Fatalf("PutArchive appelé %d fois", n)
	}
}

// entryOf reads the single entry of a tar sent to PutArchive, to io.EOF.
func entryOf(t *testing.T, b []byte) (*tar.Header, string) {
	t.Helper()
	tr := tar.NewReader(bytes.NewReader(b))
	h, err := tr.Next()
	if err != nil {
		t.Fatalf("tar : %v", err)
	}
	body, err := io.ReadAll(tr)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tr.Next(); err != io.EOF {
		t.Fatalf("une seule entrée attendue : %v", err)
	}
	return h, string(body)
}

func TestETag(t *testing.T) {
	// SHA-256 of "abc", quoted: the ETag header of the preview (spec §6).
	if got := ETag([]byte("abc")); got != `"ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad"` {
		t.Fatalf("ETag : %s", got)
	}
}

func TestReadOnlyRefusesFirst(t *testing.T) {
	r := newRig(t, true)
	if err := r.w.Allowed(); !errors.Is(err, ErrReadOnly) {
		t.Fatalf("Allowed : %v", err)
	}
	if _, err := r.w.Edit(ctx, ctr, "/etc/app.conf", []byte("x"), confTag); !errors.Is(err, ErrReadOnly) {
		t.Fatalf("Edit : %v", err)
	}
	// Read-only beats every other refusal, an invalid name included.
	if _, err := r.w.Upload(ctx, ctr, "/etc", "a/b", []byte("x"), "*"); !errors.Is(err, ErrReadOnly) {
		t.Fatalf("Upload : %v", err)
	}
	if r.opens != 0 {
		t.Fatal("en lecture seule, la vue du conteneur ne s'ouvre même pas")
	}
	r.nothingWritten(t)
}

var ctx = context.Background()

func TestEditIfMatch(t *testing.T) {
	for _, tag := range []string{"", "*", `W/` + confTag, strings.Trim(confTag, `"`), ETag([]byte("autre"))} {
		r := newRig(t, false)
		if _, err := r.w.Edit(ctx, ctr, "/etc/app.conf", []byte("x"), tag); !errors.Is(err, ErrConflict) {
			t.Errorf("If-Match %q : %v, attendu ErrConflict", tag, err)
		}
		r.nothingWritten(t)
	}
}

func TestEditOrderAndTar(t *testing.T) {
	r := newRig(t, false)
	got, err := r.w.Edit(ctx, ctr, "/etc/../etc//app.conf", []byte("10.0.0.1 db\n"), confTag)
	if err != nil {
		t.Fatal(err)
	}
	if got != ETag([]byte("10.0.0.1 db\n")) {
		t.Errorf("nouvel ETag : %s", got)
	}
	if s := r.j.Steps(); !slices.Equal(s, []string{"audit:intent", "put:/etc", "forget:" + cid, "audit:ok"}) {
		t.Fatalf("ordre : %v, attendu audit → écriture → oubli de l'index → audit", s)
	}
	for _, e := range r.j.events {
		if e.Action != "file.write" || e.Container != cid || e.Path != "/etc/app.conf" {
			t.Errorf("événement : %+v", e)
		}
	}
	puts := r.fake.PutCalls()
	if len(puts) != 1 || puts[0].ID != cid || puts[0].Dir != "/etc" {
		t.Fatalf("PutArchive : %d appel(s)", len(puts))
	}
	h, body := entryOf(t, puts[0].Tar)
	if h.Name != "app.conf" || h.Typeflag != tar.TypeReg || body != "10.0.0.1 db\n" || h.Size != int64(len(body)) {
		t.Errorf("entrée : %+v %q", h, body)
	}
	if h.Mode != 0o2640 || h.Uid != 101 || h.Gid != 102 {
		t.Errorf("mode %o uid %d gid %d : ceux du fichier existant attendus (2640, 101, 102)", h.Mode, h.Uid, h.Gid)
	}
	if !slices.Equal(r.forgot, []string{cid}) {
		t.Errorf("Forget : %v", r.forgot)
	}
}

func TestUploadOrderAndTar(t *testing.T) {
	r := newRig(t, false)
	got, err := r.w.Upload(ctx, ctr, "/etc/", "new.conf", []byte("port=80\n"), "*")
	if err != nil || got != "/etc/new.conf" {
		t.Fatalf("Upload : %q %v", got, err)
	}
	if s := r.j.Steps(); !slices.Equal(s, []string{"audit:intent", "put:/etc", "forget:" + cid, "audit:ok"}) {
		t.Fatalf("ordre : %v, attendu audit → écriture → oubli de l'index → audit", s)
	}
	if e := r.j.events[0]; e.Action != "file.upload" || e.Path != "/etc/new.conf" {
		t.Errorf("événement : %+v", e)
	}
	if !slices.Equal(r.forgot, []string{cid}) {
		t.Errorf("Forget après un envoi : %v", r.forgot)
	}
	h, body := entryOf(t, r.fake.PutCalls()[0].Tar)
	if h.Name != "new.conf" || body != "port=80\n" || h.Mode != 0o644 || h.Uid != 7 || h.Gid != 8 {
		t.Errorf("nouveau fichier : 0644 et uid/gid du dossier parent attendus : %+v", h)
	}
	// At the root, the parent sent is "/" and the path written "/x".
	r = newRig(t, false)
	if got, err := r.w.Upload(ctx, ctr, "/", "x", nil, "*"); err != nil || got != "/x" || r.fake.PutCalls()[0].Dir != "/" {
		t.Fatalf("racine : %q %v", got, err)
	}
}

func TestUploadInvalidName(t *testing.T) {
	for _, n := range []string{"", ".", "..", "a/b", "/x", "x/", "a\x00b", strings.Repeat("n", 256),
		`a\b`, "a\nb", "a\x7fb", "a\u0085b", "\xffx", // backslash, C0, DEL, C1, not UTF-8
		"txt.\u202eexe", "a\u2028b", "a\u2029b", "a\u200bb", // bidi override, line and paragraph separators, zero width
		"a\u2066b", "a\u2067b", "a\u2068b", "a\u2069b", "a\u200eb", "a\u061cb", "a\tb", // bidi isolates, marks, tab
		strings.Repeat("é", 128)} { // 256 bytes in 128 runes
		r := newRig(t, false)
		if _, err := r.w.Upload(ctx, ctr, "/etc", n, []byte("x"), "*"); !errors.Is(err, ErrInvalidName) {
			t.Errorf("nom %q : %v, attendu ErrInvalidName", n, err)
		}
		if r.opens != 0 {
			t.Errorf("nom %q : refusé avant d'ouvrir la vue", n)
		}
		r.nothingWritten(t)
	}
	for _, n := range []string{strings.Repeat("n", 255), strings.Repeat("é", 127), "été 2026.txt", "-x"} {
		r := newRig(t, false)
		if _, err := r.w.Upload(ctx, ctr, "/etc", n, nil, "*"); err != nil {
			t.Errorf("nom %q : %v", n, err)
		}
	}
}

func TestUploadNeverReplaces(t *testing.T) {
	r := newRig(t, false)
	if _, err := r.w.Upload(ctx, ctr, "/etc", "app.conf", []byte("x"), "*"); !errors.Is(err, ErrConflict) {
		t.Fatalf("nom existant : %v, attendu ErrConflict", err)
	}
	r.nothingWritten(t)
	r = newRig(t, false)
	r.mem.AddDir("/etc/conf.d")
	if _, err := r.w.Upload(ctx, ctr, "/etc", "conf.d", []byte("x"), "*"); !errors.Is(err, ErrConflict) {
		t.Fatalf("dossier existant : %v, attendu ErrConflict", err)
	}
	r.nothingWritten(t)
	// The name is absent: only the precondition refuses, and the same name
	// with "*" is created.
	r = newRig(t, false)
	for _, h := range []string{"", `"*"`, "W/*"} {
		if _, err := r.w.Upload(ctx, ctr, "/etc", "new", []byte("x"), h); !errors.Is(err, ErrConflict) {
			t.Fatalf("nom absent, If-None-Match %q : %v, attendu ErrConflict", h, err)
		}
		r.nothingWritten(t)
	}
	if got, err := r.w.Upload(ctx, ctr, "/etc", "new", []byte("x"), "*"); err != nil || got != "/etc/new" {
		t.Fatalf("même nom avec * : %q %v, attendu la création", got, err)
	}
	if n := len(r.fake.PutCalls()); n != 1 {
		t.Fatalf("%d appels à PutArchive, attendu 1", n)
	}
}

func TestLinksRefused(t *testing.T) {
	setup := func(t *testing.T) *rig {
		r := newRig(t, false)
		r.mem.AddLink("/etc/alias", "app.conf").AddLink("/conf", "etc").AddLink("/etc/dangling", "/nope")
		return r
	}
	cases := []struct {
		name string
		run  func(r *rig) error
	}{
		{"lien final", func(r *rig) error { _, err := r.w.Edit(ctx, ctr, "/etc/alias", []byte("x"), confTag); return err }},
		{"lien parent", func(r *rig) error { _, err := r.w.Edit(ctx, ctr, "/conf/app.conf", []byte("x"), confTag); return err }},
		{"dossier d'envoi lié", func(r *rig) error { _, err := r.w.Upload(ctx, ctr, "/conf", "new", []byte("x"), "*"); return err }},
		{"envoi sur un lien pendant", func(r *rig) error { _, err := r.w.Upload(ctx, ctr, "/etc", "dangling", []byte("x"), "*"); return err }},
		{"envoi sur un lien", func(r *rig) error { _, err := r.w.Upload(ctx, ctr, "/etc", "alias", []byte("x"), "*"); return err }},
	}
	for _, c := range cases {
		r := setup(t)
		if err := c.run(r); !errors.Is(err, fsview.ErrLinkInPath) {
			t.Errorf("%s : %v, attendu ErrLinkInPath", c.name, err)
		}
		r.nothingWritten(t)
	}
}

func TestEditNotRegular(t *testing.T) {
	r := newRig(t, false)
	r.mem.Entries["/srv"] = fsview.Entry{Name: "srv", Type: fsview.TypeDir}
	r.mem.Entries["/srv/app.sock"] = fsview.Entry{Name: "app.sock", Type: fsview.TypeOther}
	for _, p := range []string{"/srv/app.sock", "/etc", "/"} {
		if _, err := r.w.Edit(ctx, ctr, p, []byte("x"), confTag); !errors.Is(err, fsview.ErrNotRegular) {
			t.Errorf("%s : %v, attendu ErrNotRegular", p, err)
		}
	}
	r.nothingWritten(t)
	if _, err := r.w.Upload(ctx, ctr, "/etc/app.conf", "x", []byte("x"), "*"); !errors.Is(err, fsview.ErrNotDir) {
		t.Errorf("envoi dans un fichier : %v, attendu ErrNotDir", err)
	}
	if _, err := r.w.Edit(ctx, ctr, "/etc/absent", []byte("x"), confTag); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("absent : %v", err)
	}
	r.nothingWritten(t)
}

func TestSizeCaps(t *testing.T) {
	r := newRig(t, false)
	if _, err := r.w.Edit(ctx, ctr, "/etc/app.conf", make([]byte, MaxEditBytes+1), confTag); !errors.Is(err, fsview.ErrTooLarge) {
		t.Errorf("édition : %v", err)
	}
	if _, err := r.w.Upload(ctx, ctr, "/etc", "big", make([]byte, MaxUploadBytes+1), "*"); !errors.Is(err, fsview.ErrTooLarge) {
		t.Errorf("envoi : %v", err)
	}
	// The current file is read with a cap too: its hash never needs more.
	r.mem.AddFile("/etc/huge", make([]byte, MaxEditBytes+1))
	if _, err := r.w.Edit(ctx, ctr, "/etc/huge", []byte("x"), ETag(make([]byte, MaxEditBytes+1))); !errors.Is(err, fsview.ErrTooLarge) {
		t.Errorf("fichier actuel trop gros : %v", err)
	}
	r.nothingWritten(t)
}

func TestViewErrorsPassThrough(t *testing.T) {
	for _, want := range []error{fsview.ErrForbiddenPath, &fsview.IndexingError{Progress: 0.4}, dockerx.ErrUnavailable} {
		r := newRig(t, false)
		r.w.Open = func(context.Context, string) (fsview.FS, error) { return nil, want }
		if _, err := r.w.Edit(ctx, ctr, "/etc/app.conf", []byte("x"), confTag); !errors.Is(err, want) {
			t.Errorf("Edit : %v, attendu %v", err, want)
		}
		if _, err := r.w.Upload(ctx, ctr, "/etc", "n", []byte("x"), "*"); !errors.Is(err, want) {
			t.Errorf("Upload : %v, attendu %v", err, want)
		}
		r.nothingWritten(t)
	}
	r := newRig(t, false)
	r.mem.Err = fsview.ErrForbiddenPath // e.g. the parent directory is on a pseudo filesystem
	if _, err := r.w.Upload(ctx, ctr, "/etc", "n", []byte("x"), "*"); !errors.Is(err, fsview.ErrForbiddenPath) {
		t.Errorf("dossier interdit : %v", err)
	}
	r.nothingWritten(t)
}

func TestAuditIntentFailureWritesNothing(t *testing.T) {
	r := newRig(t, false)
	r.j.auditErr = errors.New("disque plein")
	if _, err := r.w.Edit(ctx, ctr, "/etc/app.conf", []byte("x"), confTag); err == nil {
		t.Fatal("sans ligne d'intention, pas d'écriture")
	}
	if n := len(r.fake.PutCalls()); n != 0 {
		t.Fatalf("PutArchive appelé %d fois", n)
	}
}

func TestPutFailureIsAudited(t *testing.T) {
	r := newRig(t, false)
	r.fake.PutErr = dockerx.ErrRefused
	if _, err := r.w.Edit(ctx, ctr, "/etc/app.conf", []byte("x"), confTag); !errors.Is(err, dockerx.ErrRefused) {
		t.Fatalf("Edit : %v", err)
	}
	if s := r.j.Steps(); !slices.Equal(s, []string{"audit:intent", "put:/etc", "forget:" + cid, "audit:error"}) {
		t.Fatalf("ordre : %v, attendu audit → écriture (échouée) → oubli de l'index → audit", s)
	}
	if e := r.j.events[1]; !strings.Contains(e.Error, "refusé") {
		t.Errorf("résultat : %+v", e)
	}
	if !slices.Equal(r.forgot, []string{cid}) {
		t.Errorf("un échec de PutArchive retire aussi l'index : %v", r.forgot)
	}
}

// Once the intent is in the journal, the write goes through even if the
// request ends: Docker removes the file before creating it again, a cut
// write would lose it.
func TestPutSurvivesRequestEnd(t *testing.T) {
	r := newRig(t, false)
	c, cancel := context.WithCancel(ctx)
	r.j.onEvent = func(e audit.Event) {
		if e.Result == "intent" {
			cancel()
		}
	}
	if _, err := r.w.Edit(c, ctr, "/etc/app.conf", []byte("x"), confTag); err != nil {
		t.Fatal(err)
	}
	if r.put.ctxErr != nil {
		t.Fatalf("PutArchive a reçu un contexte annulé : %v", r.put.ctxErr)
	}
	if s := r.j.Steps(); !slices.Equal(s, []string{"audit:intent", "put:/etc", "forget:" + cid, "audit:ok"}) {
		t.Fatalf("la requête finie, l'écriture et son résultat suivent : %v", s)
	}
}

// Stopped container through the real Views (spec §5.2 step 1): indexing
// while the index is built, then a write drops it, and the next access
// builds it again.
func TestStoppedContainerIndexDropped(t *testing.T) {
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	for _, h := range []*tar.Header{
		{Name: "/", Typeflag: tar.TypeDir, Mode: 0o755},
		{Name: "/etc/", Typeflag: tar.TypeDir, Mode: 0o755},
		{Name: "/etc/app.conf", Typeflag: tar.TypeReg, Mode: 0o644, Size: 4},
	} {
		tw.WriteHeader(h)
		if h.Size > 0 {
			io.WriteString(tw, "old\n")
		}
	}
	tw.Close()
	var file bytes.Buffer
	fw := tar.NewWriter(&file)
	fw.WriteHeader(&tar.Header{Name: "app.conf", Typeflag: tar.TypeReg, Mode: 0o644, Size: 4})
	io.WriteString(fw, "old\n")
	fw.Close()
	info := dockerx.ContainerInfo{ID: cid, Name: "db", FinishedAt: "2026-10-09T10:00:00Z"}
	fake := &dockerx.Fake{
		Infos:    map[string]dockerx.ContainerInfo{cid: info},
		Archives: map[string][]byte{cid + ":/": buf.Bytes(), cid + ":/etc/app.conf": file.Bytes()},
		// The gate keeps the first build in flight: ErrIndexing is certain,
		// whatever the speed of the machine.
		ArchiveGate: make(chan struct{}),
	}
	var release sync.Once
	open := func() { release.Do(func() { close(fake.ArchiveGate) }) }
	t.Cleanup(open)
	views := fsview.NewViews(fake, fsview.Options{})
	w := &Writer{Docker: fake, Audit: &journal{}, Open: views.For, Forget: views.Forget}
	if _, err := w.Edit(ctx, info, "/etc/app.conf", []byte("new\n"), ETag([]byte("old\n"))); !errors.Is(err, fsview.ErrIndexing) {
		t.Fatalf("pendant l'indexation : %v, attendu ErrIndexing", err)
	}
	open()
	for deadline := time.Now().Add(5 * time.Second); ; time.Sleep(5 * time.Millisecond) {
		_, err := w.Edit(ctx, info, "/etc/app.conf", []byte("new\n"), ETag([]byte("old\n")))
		if err == nil {
			break
		}
		if !errors.Is(err, fsview.ErrIndexing) || time.Now().After(deadline) {
			t.Fatal(err)
		}
	}
	builds := func() int {
		n := 0
		for _, c := range fake.Calls() {
			if c == cid+":/" {
				n++
			}
		}
		return n
	}
	if n := builds(); n != 1 {
		t.Fatalf("une construction attendue, %d", n)
	}
	// The index is gone: this access starts a new build, which may or may
	// not be done when it answers. The proof is the second GetArchive of "/".
	if fsys, err := views.For(ctx, cid); err == nil {
		fsys.Close()
	} else if !errors.Is(err, fsview.ErrIndexing) {
		t.Fatalf("après l'écriture : %v", err)
	}
	// The new build runs in a goroutine: wait for its GetArchive of "/".
	for deadline := time.Now().Add(5 * time.Second); builds() != 2; time.Sleep(5 * time.Millisecond) {
		if time.Now().After(deadline) {
			t.Fatalf("l'accès suivant reconstruit l'index : %d construction(s)", builds())
		}
	}
}

// Spec §5.3: the path and the name are checked (step 2) before the
// precondition (step 3). Without If-None-Match, a bad name is still
// invalid_name and a link is still link_in_path, not conflict.
func TestUploadChecksPathBeforePrecondition(t *testing.T) {
	r := newRig(t, false)
	r.mem.AddLink("/conf", "etc")
	if _, err := r.w.Upload(ctx, ctr, "/etc", "../x", []byte("x"), ""); !errors.Is(err, ErrInvalidName) {
		t.Errorf("../x sans If-None-Match : %v, attendu ErrInvalidName", err)
	}
	if _, err := r.w.Upload(ctx, ctr, "/conf", "x", []byte("x"), ""); !errors.Is(err, fsview.ErrLinkInPath) {
		t.Errorf("dossier lié sans If-None-Match : %v, attendu ErrLinkInPath", err)
	}
	r.nothingWritten(t)
}

// Q22: a container whose mounts fall under the host-bind rule (spec §5.2
// step 7: a bind of /, /proc, /sys, /dev…) gets no
// write at all, running or not: no view opened, nothing written.
func TestHostBindRefusesEveryWrite(t *testing.T) {
	r := newRig(t, false)
	c := ctr
	c.Mounts = []dockerx.Mount{{Type: "bind", Source: "/", Destination: "/host"}}
	if _, err := r.w.Edit(ctx, c, "/etc/app.conf", []byte("x"), confTag); !errors.Is(err, fsview.ErrForbiddenPath) {
		t.Errorf("édition : %v, attendu ErrForbiddenPath", err)
	}
	if _, err := r.w.Upload(ctx, c, "/etc", "n", []byte("x"), "*"); !errors.Is(err, fsview.ErrForbiddenPath) {
		t.Errorf("envoi : %v, attendu ErrForbiddenPath", err)
	}
	if r.opens != 0 {
		t.Error("refusé sur Inspect, avant d'ouvrir la vue")
	}
	r.nothingWritten(t)
	// Control, and the scope of HostBind (Q22, option a, open): without the
	// bind of /, or with a bind of the host's /etc, the write goes through.
	for _, m := range [][]dockerx.Mount{nil, {{Type: "bind", Source: "/etc", Destination: "/hostetc"}}} {
		r := newRig(t, false)
		c := ctr
		c.Mounts = m
		if _, err := r.w.Edit(ctx, c, "/etc/app.conf", []byte("x"), confTag); err != nil {
			t.Errorf("montages %v : %v", m, err)
		}
	}
}

// Q21: Docker writes in its own view of the mounts, not in the container's.
// A file mounted on its own and the files Docker manages answer "device or
// resource busy"; a tmpfs, /dev, /proc or /sys take the write and the
// container never sees it (measured). Refused before the intent.
func TestRefusesWhereDockerDoesNotWrite(t *testing.T) {
	c := ctr
	c.Mounts = []dockerx.Mount{
		{Type: "bind", Source: "/srv/app.conf", Destination: "/etc/app.conf"},
		{Type: "tmpfs", Destination: "/run/t"},
		{Type: "bind", Source: "/srv/conf", Destination: "/etc/conf.d"},
	}
	c.Tmpfs = []string{"/scratch"}
	cases := []struct {
		name string
		run  func(r *rig) error
	}{
		{"fichier monté", func(r *rig) error { _, err := r.w.Edit(ctx, c, "/etc/app.conf", []byte("x"), confTag); return err }},
		{"/etc/hosts", func(r *rig) error { _, err := r.w.Edit(ctx, c, "/etc/hosts", []byte("x"), confTag); return err }},
		{"/etc/resolv.conf", func(r *rig) error { _, err := r.w.Upload(ctx, c, "/etc", "resolv.conf", []byte("x"), "*"); return err }},
		{"/etc/hostname", func(r *rig) error { _, err := r.w.Edit(ctx, c, "/etc/hostname", []byte("x"), confTag); return err }},
		{"--mount type=tmpfs", func(r *rig) error { _, err := r.w.Upload(ctx, c, "/run/t", "n", []byte("x"), "*"); return err }},
		{"--tmpfs", func(r *rig) error { _, err := r.w.Upload(ctx, c, "/scratch/sub", "n", []byte("x"), "*"); return err }},
		{"/dev", func(r *rig) error { _, err := r.w.Upload(ctx, c, "/dev/shm", "n", []byte("x"), "*"); return err }},
		{"/proc", func(r *rig) error { _, err := r.w.Upload(ctx, c, "/proc", "n", []byte("x"), "*"); return err }},
		{"/sys", func(r *rig) error { _, err := r.w.Upload(ctx, c, "/sys/x", "n", []byte("x"), "*"); return err }},
		{"--tmpfs, chemin non nettoyé", func(r *rig) error { _, err := r.w.Upload(ctx, c, "/scratch/./sub/", "n", []byte("x"), "*"); return err }},
		{"/proc, chemin non nettoyé", func(r *rig) error { _, err := r.w.Upload(ctx, c, "//proc/", "n", []byte("x"), "*"); return err }},
		{"/etc/hosts, chemin non nettoyé", func(r *rig) error { _, err := r.w.Edit(ctx, c, "/etc//hosts", []byte("x"), confTag); return err }},
	}
	for _, k := range cases {
		r := newRig(t, false)
		if err := k.run(r); !errors.Is(err, fsview.ErrForbiddenPath) {
			t.Errorf("%s : %v, attendu ErrForbiddenPath", k.name, err)
		}
		if r.opens != 0 {
			t.Errorf("%s : refusé avant d'ouvrir la vue", k.name)
		}
		r.nothingWritten(t)
	}
	// Control: a bind of a directory is in Docker's view, the write lands
	// where the container sees it.
	r := newRig(t, false)
	r.mem.AddDir("/etc/conf.d")
	if _, err := r.w.Upload(ctx, c, "/etc/conf.d", "n", []byte("x"), "*"); err != nil {
		t.Errorf("dossier monté : %v", err)
	}
	// Negative controls: a longer name is not the forbidden prefix.
	r = newRig(t, false)
	r.mem.AddDir("/procx").AddDir("/etc/hosts.d").AddDir("/scratch2")
	for _, dir := range []string{"/procx", "/etc/hosts.d", "/scratch2"} {
		if _, err := r.w.Upload(ctx, c, dir, "n", []byte("x"), "*"); err != nil {
			t.Errorf("%s : %v, la règle ne vise que le dossier et ce qu'il contient", dir, err)
		}
	}
}

// A request that ends once the lock is held (the guard answered 504 while
// the checks ran) never starts a write: only the check of put stops it,
// MemFS ignores the context.
func TestEndedRequestWritesNothing(t *testing.T) {
	r := newRig(t, false)
	c, cancel := context.WithCancel(ctx)
	r.w.Open = func(context.Context, string) (fsview.FS, error) {
		cancel() // the lock is held: the request ends now
		return r.mem, nil
	}
	if _, err := r.w.Edit(c, ctr, "/etc/app.conf", []byte("x"), confTag); !errors.Is(err, context.Canceled) {
		t.Fatalf("requête terminée : %v", err)
	}
	r.nothingWritten(t)
}

// A request abandoned while it waits for the lock writes nothing and does
// not keep the lock.
func TestLockWaitAbandoned(t *testing.T) {
	r := newRig(t, false)
	g := &gatedPutter{Fake: r.fake, entered: make(chan struct{}, 2), gate: make(chan struct{}), apply: func(string, []byte) {}}
	r.w.Docker = g
	var opens atomic.Int32
	r.w.Open = func(context.Context, string) (fsview.FS, error) { opens.Add(1); return r.mem, nil }
	first := make(chan error, 1)
	go func() {
		_, err := r.w.Edit(ctx, ctr, "/etc/app.conf", []byte("v2\n"), confTag)
		first <- err
	}()
	<-g.entered // the first write holds the lock, inside PutArchive
	c, cancel := context.WithCancel(ctx)
	waiting := make(chan error, 1)
	go func() {
		_, err := r.w.Upload(c, ctr, "/etc", "n", []byte("x"), "*")
		waiting <- err
	}()
	// The upload really waits for the lock (its context is still alive):
	// it neither returns nor opens the view. Only then does the request end.
	select {
	case err := <-waiting:
		t.Fatalf("l'envoi devait attendre le verrou : %v", err)
	case <-time.After(150 * time.Millisecond):
	}
	if n := opens.Load(); n != 1 {
		t.Fatalf("%d ouverture(s) de la vue, attendu 1 (celle de l'écriture en cours)", n)
	}
	cancel() // only ctx.Done can free the waiting upload
	select {
	case err := <-waiting:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("attente abandonnée : %v, attendu context.Canceled", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("l'attente du verrou ne s'arrête pas avec la requête")
	}
	close(g.gate)
	if err := <-first; err != nil {
		t.Fatal(err)
	}
	if n := len(r.fake.PutCalls()); n != 1 {
		t.Fatalf("%d appels à PutArchive, attendu 1 (l'envoi abandonné n'écrit rien)", n)
	}
	if n := opens.Load(); n != 1 {
		t.Fatalf("%d ouverture(s) de la vue, l'envoi abandonné n'a rien à ouvrir", n)
	}
	r.w.Docker = r.put
	if _, err := r.w.Upload(ctx, ctr, "/etc", "n2", []byte("x"), "*"); err != nil {
		t.Fatalf("le verrou est resté pris : %v", err)
	}
}

// A panic once the lock is held (the guard recovers it) releases the lock.
func TestLockReleasedOnPanic(t *testing.T) {
	r := newRig(t, false)
	open := r.w.Open
	r.w.Open = func(context.Context, string) (fsview.FS, error) { panic("vue cassée") }
	func() {
		defer func() {
			if recover() == nil {
				t.Error("la panique devait remonter")
			}
		}()
		r.w.Edit(ctx, ctr, "/etc/app.conf", []byte("x"), confTag)
	}()
	r.w.Open = open
	done := make(chan error, 1)
	go func() {
		_, err := r.w.Edit(ctx, ctr, "/etc/app.conf", []byte("x"), confTag)
		done <- err
	}()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("verrou gardé après une panique")
	}
}

// The name is checked before the rules of the Inspect: a bad name toward a
// forbidden place is invalid_name.
func TestUploadNameBeforeInspectRules(t *testing.T) {
	r := newRig(t, false)
	c := ctr
	c.Mounts = []dockerx.Mount{{Type: "bind", Source: "/", Destination: "/host"}}
	for _, dir := range []string{"/proc", "/host"} {
		if _, err := r.w.Upload(ctx, c, dir, "../x", []byte("x"), ""); !errors.Is(err, ErrInvalidName) {
			t.Errorf("../x dans %s : %v, attendu ErrInvalidName", dir, err)
		}
	}
	r.nothingWritten(t)
}

// gatedPutter blocks every PutArchive on gate, then applies the write to the
// in-memory view, as Docker would.
type gatedPutter struct {
	*dockerx.Fake
	entered chan struct{}
	gate    chan struct{}
	apply   func(dir string, tarball []byte)
}

func (g *gatedPutter) PutArchive(ctx context.Context, id, dir string, r io.Reader) error {
	b, _ := io.ReadAll(r)
	g.entered <- struct{}{}
	<-g.gate
	g.apply(dir, b)
	return g.Fake.PutArchive(ctx, id, dir, bytes.NewReader(b))
}

// Two writes on one container never interleave (Q23): two tabs with the same
// If-Match, or two uploads of one name, give one write and one conflict.
func TestWritesAreSerialized(t *testing.T) {
	for _, k := range []struct {
		name  string
		write func(w *Writer) error
	}{
		{"deux éditions, même If-Match", func(w *Writer) error {
			_, err := w.Edit(ctx, ctr, "/etc/app.conf", []byte("v2\n"), confTag)
			return err
		}},
		{"deux envois, même nom", func(w *Writer) error {
			_, err := w.Upload(ctx, ctr, "/etc", "new.conf", []byte("v2\n"), "*")
			return err
		}},
	} {
		r := newRig(t, false)
		g := &gatedPutter{Fake: r.fake, entered: make(chan struct{}, 2), gate: make(chan struct{}),
			apply: func(dir string, tarball []byte) {
				// Not entryOf: t.Fatalf must not run outside the test goroutine.
				tr := tar.NewReader(bytes.NewReader(tarball))
				h, err := tr.Next()
				if err != nil {
					t.Errorf("tar : %v", err)
					return
				}
				body, _ := io.ReadAll(tr)
				r.mem.AddFile(dir+"/"+h.Name, body)
			}}
		r.w.Docker = g
		var opens atomic.Int32
		r.w.Open = func(context.Context, string) (fsview.FS, error) { opens.Add(1); return r.mem, nil }
		errs := make(chan error, 2)
		go func() { errs <- k.write(r.w) }()
		<-g.entered // the first write is inside PutArchive
		go func() { errs <- k.write(r.w) }()
		select {
		case <-g.entered:
			t.Fatalf("%s : la seconde écriture a passé ses contrôles pendant la première", k.name)
		case <-time.After(200 * time.Millisecond):
		}
		// The lock comes before the view: a second request that opened it (or
		// read the file) now would work on a view older than the first write.
		if n := opens.Load(); n != 1 {
			t.Fatalf("%s : %d ouverture(s) de la vue pendant la première écriture, attendu 1 : verrou pris trop tard", k.name, n)
		}
		close(g.gate)
		e1, e2 := <-errs, <-errs
		if (e1 == nil) == (e2 == nil) || !(errors.Is(e1, ErrConflict) || errors.Is(e2, ErrConflict)) {
			t.Errorf("%s : %v, %v ; attendu une écriture et un conflit", k.name, e1, e2)
		}
		if n := len(r.fake.PutCalls()); n != 1 {
			t.Errorf("%s : %d appels à PutArchive, attendu 1", k.name, n)
		}
	}
}

// mountView is MemFS plus the mount points of a running container
// (fsview.MountPointChecker, implemented by procfs).
type mountView struct {
	*fsview.MemFS
	points map[string]bool
}

func (m mountView) IsMountPoint(_ context.Context, p string) (bool, error) { return m.points[p], nil }

// Q21: a mount point Inspect does not show (a Swarm config, docker-init, a
// CDI file) is refused for an edit, through the view, before the intent.
func TestEditRefusesMountPoint(t *testing.T) {
	r := newRig(t, false)
	view := mountView{MemFS: r.mem, points: map[string]bool{"/etc/app.conf": true}}
	r.w.Open = func(context.Context, string) (fsview.FS, error) { return view, nil }
	if _, err := r.w.Edit(ctx, ctr, "/etc/app.conf", []byte("x"), confTag); !errors.Is(err, fsview.ErrForbiddenPath) {
		t.Fatalf("point de montage : %v, attendu ErrForbiddenPath", err)
	}
	r.nothingWritten(t)
	view.points = map[string]bool{}
	r.w.Open = func(context.Context, string) (fsview.FS, error) { return view, nil }
	if _, err := r.w.Edit(ctx, ctr, "/etc/app.conf", []byte("x"), confTag); err != nil {
		t.Fatalf("contrôle, fichier ordinaire : %v", err)
	}
}

// rootView is MemFS plus the root check of a view (fsview.WriteRootChecker,
// implemented by procfs and tarindex): err, or the tmpfs mount points.
type rootView struct {
	*fsview.MemFS
	err    error
	points []string
}

func (v rootView) CheckWriteRoot(context.Context) ([]string, error) { return v.points, v.err }

// Review I1: with a chrooted PID 1 the view shows another tree than the one
// Docker writes in. The view says so: no edit, no upload, no audit line, no
// PutArchive. A view that vouches for its root writes as usual.
func TestWriteRefusedWhenRootIsNotContainerRoot(t *testing.T) {
	r := newRig(t, false)
	view := rootView{MemFS: r.mem, err: fmt.Errorf("%w : racine du process déplacée", fsview.ErrForbiddenPath)}
	r.w.Open = func(context.Context, string) (fsview.FS, error) { return view, nil }
	if _, err := r.w.Edit(ctx, ctr, "/etc/app.conf", []byte("x"), confTag); !errors.Is(err, fsview.ErrForbiddenPath) {
		t.Fatalf("édition, racine déplacée : %v, attendu ErrForbiddenPath", err)
	}
	r.nothingWritten(t)
	if _, err := r.w.Upload(ctx, ctr, "/etc", "new.conf", []byte("x"), "*"); !errors.Is(err, fsview.ErrForbiddenPath) {
		t.Fatalf("envoi, racine déplacée : %v, attendu ErrForbiddenPath", err)
	}
	r.nothingWritten(t)
	view.err = nil
	r.w.Open = func(context.Context, string) (fsview.FS, error) { return view, nil }
	if _, err := r.w.Edit(ctx, ctr, "/etc/app.conf", []byte("x"), confTag); err != nil {
		t.Fatalf("contrôle, racine du conteneur : %v", err)
	}
}

// Spec §13: a --tmpfs under a link (/var/run -> ../run) is mounted at /run;
// the view finds it there (CheckWriteRoot). Docker never writes into a
// tmpfs: a write under /run would succeed where the container never sees
// it. Refused before any intent; the rest of the container stays writable.
func TestWriteRefusedUnderResolvedTmpfs(t *testing.T) {
	r := newRig(t, false)
	r.mem.AddDir("/run").AddFile("/run/app.pid", []byte("1\n"))
	r.mem.Entries["/run/app.pid"] = fsview.Entry{Name: "app.pid", Type: fsview.TypeFile, Mode: 0o644}
	view := rootView{MemFS: r.mem, points: []string{"/run"}}
	r.w.Open = func(context.Context, string) (fsview.FS, error) { return view, nil }
	c := ctr
	c.Tmpfs = []string{"/var/run"}
	if _, err := r.w.Edit(ctx, c, "/run/app.pid", []byte("2\n"), ETag([]byte("1\n"))); !errors.Is(err, fsview.ErrForbiddenPath) || !strings.Contains(err.Error(), "/run") {
		t.Fatalf("édition sous le tmpfs résolu : %v, attendu ErrForbiddenPath nommant /run", err)
	}
	r.nothingWritten(t)
	for _, dir := range []string{"/run", "/run/sub"} {
		if _, err := r.w.Upload(ctx, c, dir, "x", []byte("x"), "*"); !errors.Is(err, fsview.ErrForbiddenPath) {
			t.Fatalf("envoi dans %s : %v, attendu ErrForbiddenPath", dir, err)
		}
	}
	r.nothingWritten(t)
	if _, err := r.w.Upload(ctx, c, "/", "run2", []byte("x"), "*"); err != nil {
		t.Fatalf("/run2 n'est pas sous /run : %v", err)
	}
	if _, err := r.w.Edit(ctx, c, "/etc/app.conf", []byte("x"), confTag); err != nil {
		t.Fatalf("le reste du conteneur reste inscriptible : %v", err)
	}
}
