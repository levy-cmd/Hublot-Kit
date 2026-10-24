package secrets

// Tests of History (spec v0.2 §5.4), written before the code: the history of
// an image holds its build commands, with the build arguments (ARG) as a
// prefix "|N NAME=value … /bin/sh -c …" (classic builder) or "RUN |N …"
// (BuildKit). Their names hold no word of §7 (GH_PAT, NPM_AUTH): every value
// after "|N" is masked whatever the name, up to the shell; without a shell,
// the whole rest (fail closed). Then the line goes through Line.

import (
	"strings"
	"testing"
	"time"
)

func TestHistoryBuildArgs(t *testing.T) {
	cases := map[string]string{
		"|2 GH_PAT=s3cr3t-a NPM_AUTH=s3cr3t-b /bin/sh -c npm ci":     "|2 GH_PAT=•••••• NPM_AUTH=•••••• /bin/sh -c npm ci",
		"RUN |2 GH_PAT=s3cr3t-a NPM_AUTH=s3cr3t-b /bin/sh -c npm ci": "RUN |2 GH_PAT=•••••• NPM_AUTH=•••••• /bin/sh -c npm ci",
		"|1 NOTE=a b c /bin/bash -c make":                            "|1 NOTE=•••••• /bin/bash -c make", // a value with blanks: masked up to the shell
		"|1 X=s3cr3t-c make":                                         "|1 ••••••",                        // no shell: the whole rest
		`/bin/sh -c curl -H "Authorization: Bearer s3cr3t-d" x`:      `/bin/sh -c curl -H "Authorization: Bearer ••••••" x`,
		// The number bounds the names shown: "B=s3cr3t-e" is part of the value of NOTE.
		"|1 NOTE=a B=s3cr3t-e /bin/sh -c make": "|1 NOTE=•••••• /bin/sh -c make",
		// A first word that is not NAME=: the whole part before the shell.
		"|1 =s3cr3t-f /bin/sh -c make":     "|1 •••••• /bin/sh -c make",
		"|1 s3cr3t-g X=1 /bin/sh -c make":  "|1 •••••• /bin/sh -c make",
		"RUN |1 X=s3cr3t-h":                "RUN |1 ••••••",
		"|0 /bin/sh -c apk add curl":       "|0 /bin/sh -c apk add curl",
		"|0 X=s3cr3t-n /bin/sh -c y":       "|0 •••••• /bin/sh -c y",
		"RUN |0 /bin/sh -c apk add curl":   "RUN |0 /bin/sh -c apk add curl",
		"|99999999999999999999 X=s3cr3t-i": "|99999999999999999999 ••••••",
		// The first shell, sh or bash, ends the arguments; the rest is a shell line.
		"|1 A=s3cr3t-j /bin/bash -c x /bin/sh -c y": "|1 ••••••", // two shells: the cut is unsure (second review)
		// A name that is not ASCII is not a name: part of the value.
		"|2 X=a ÉCHO=s3cr3t-k /bin/sh -c y": "|2 •••••• /bin/sh -c y",
		// Then Line, on the whole line: a secret of §7 in the command.
		"|1 X=1 /bin/sh -c mysql --password=s3cr3t-l": "|1 X=•••••• /bin/sh -c mysql --password=••••••",
		"ENV DB_PASSWORD=s3cr3t-env":                  "ENV DB_PASSWORD=••••••",
		"/bin/sh -c #(nop)  ENV API_TOKEN=s3cr3t-m":   "/bin/sh -c #(nop)  ENV API_TOKEN=••••••",
		// Not a prefix: "|2" inside a line, or without its space.
		"/bin/sh -c echo |2 x":                 "/bin/sh -c echo |2 x",
		"|2":                                   "|2",
		"":                                     "",
		"/bin/sh -c #(nop) ADD file:abc in / ": "/bin/sh -c #(nop) ADD file:abc in / ",
		"RUN /bin/sh -c apk add --no-cache curl # buildkit": "RUN /bin/sh -c apk add --no-cache curl # buildkit",
	}
	check(t, cases)
}

// Measured on Docker 29.7 (BuildKit, tranche 5): the history also keeps
// "ARG NAME=value" with the value given by --build-arg, and "LABEL k=v …"
// with every label, quotes removed ("a b=c d" for "a b"="c d"). A build
// argument is masked whatever its name, as after "|N"; a label leaves the
// agent through secrets.Labels only (spec v0.2 §7), so a LABEL line keeps
// none of its text.
func TestHistoryArgAndLabel(t *testing.T) {
	check(t, map[string]string{
		"ARG GH_PAT=s3cr3t-o":                    "ARG GH_PAT=••••••",
		"ARG GH_PAT":                             "ARG GH_PAT",
		"ARG A B=s3cr3t-p":                       "ARG A B=••••••",
		"ARG A=s3cr3t-q B=s3cr3t-r":              "ARG ••••••",
		"ARG NOTE=a s3cr3t-s c":                  "ARG ••••••",
		"ARG =s3cr3t-t":                          "ARG ••••••",
		"ARG s3cr3t-u!":                          "ARG ••••••",
		"arg GH_PAT=s3cr3t-v":                    "arg GH_PAT=••••••",
		"/bin/sh -c #(nop)  ARG GH_PAT=s3cr3t-w": "/bin/sh -c #(nop)  ARG GH_PAT=••••••",
		"LABEL traefik.http.middlewares.a.basicauth.users=admin:s3cr3t-x org.opencontainers.image.title=x": "LABEL ••••••",
		"LABEL a b=c s3cr3t-y":                   "LABEL ••••••",
		"label org.opencontainers.image.title=x": "label ••••••",
		"/bin/sh -c #(nop)  LABEL a=s3cr3t-z":    "/bin/sh -c #(nop)  LABEL ••••••",
		// Only the instruction itself: a RUN that says "ARG" or "LABEL" is a shell line.
		"RUN /bin/sh -c echo ARG X=1 # buildkit": "RUN /bin/sh -c echo ARG X=1 # buildkit",
		"ARGS=1":                                 "ARGS=1",
		"LABELS x":                               "LABELS x",
	})
}

// Review of tranche 5 (relecteur, critical): a value with a blank made the
// next word of the form NAME= pass for a name ("ARG NPM_AUTH=Basic
// dXNl…==" let the base64 out). Fail closed: with "|N" for N ≥ 2, every
// word must be NAME=… (NAME of [A-Za-z_][A-Za-z0-9_]*), and there must be
// exactly N of them (a blank in a value always adds a word); with "|1", the
// single value runs to the shell. An ARG has no count: "dXNl…==" is a
// NAME= word too, so at most one assignment, its last word, after names
// alone; else the whole part is masked.
// Second review of tranche 5 (Codex): the cut was at the FIRST shell; a value
// that holds a shell itself fooled it ("|1 X=p /bin/sh -c token /bin/sh -c
// cmd": "token" went through Line only). After "|N", more than one shell,
// spaced in any way, masks the whole rest.
func TestHistoryTwoShells(t *testing.T) {
	check(t, map[string]string{
		"|1 X=préfixe /bin/sh -c s3cr3t-a /bin/sh -c vraie-commande": "|1 ••••••",
		"RUN |1 X=p /bin/bash -c s3cr3t-b /bin/sh -c cmd # buildkit": "RUN |1 ••••••",
		"|1 X=p /bin/sh -c s3cr3t-c /bin/bash -c cmd":                "|1 ••••••",
		"|2 A=1 B=x/bin/sh -c s3cr3t-d /bin/sh -c cmd":               "|2 ••••••",
		"|1 X=/bin/sh -c s3cr3t-e /bin/bash -c cmd":                  "|1 ••••••",
		"|1 X=p /bin/sh  -c s3cr3t-f /bin/sh -c cmd":                 "|1 ••••••",
		"|1 X=p\t/bin/bash\t-c s3cr3t-g /bin/sh -c cmd":              "|1 ••••••",
		"RUN\t|2 A=1 B=p /bin/sh -c s3cr3t-h /bin/sh -c x":           "RUN\t|2 ••••••",
		"|1 X=s3cr3t-i /bin/sh -c echo 'nested /bin/sh -c call'":     "|1 ••••••",
		// One shell: the cut stays.
		"|1 X=s3cr3t-j /bin/sh -c sh -c 'make'": "|1 X=•••••• /bin/sh -c sh -c 'make'",
	})
}

func TestHistoryValueWithBlanks(t *testing.T) {
	check(t, map[string]string{
		"ARG NPM_AUTH=Basic s3cr3tbase64==":                              "ARG ••••••",
		"RUN |2 NPM_AUTH=Basic s3cr3tbase64== OTHER=1 /bin/sh -c npm ci": "RUN |2 •••••• /bin/sh -c npm ci",
		"|2 NPM_AUTH=Basic s3cr3tbase64== OTHER=1 /bin/sh -c npm ci":     "|2 •••••• /bin/sh -c npm ci",
		"|2 A=x B=y C=s3cr3t-a /bin/sh -c x":                             "|2 •••••• /bin/sh -c x",
		"|3 A=x B=s3cr3t-b /bin/sh -c x":                                 "|3 •••••• /bin/sh -c x",
		"|2 A.b=x C=s3cr3t-c /bin/sh -c x":                               "|2 •••••• /bin/sh -c x",
		"|2 A-b=x C=s3cr3t-d /bin/sh -c x":                               "|2 •••••• /bin/sh -c x",
		"|2 A=s3cr3t-e B=s3cr3t-f /bin/sh -c x":                          "|2 A=•••••• B=•••••• /bin/sh -c x",
		"|1 NPM_AUTH=Basic s3cr3tbase64== /bin/sh -c npm ci":             "|1 NPM_AUTH=•••••• /bin/sh -c npm ci",
		"ARG A=1 B=s3cr3t-g":                                             "ARG ••••••", // two assignments: "A=1 B=…" may be one value
		"ARG A=1 s3cr3t-h":                                               "ARG ••••••",
		"ARG A.b=s3cr3t-i":                                               "ARG ••••••",
		"ARG A B":                                                        "ARG A B",
		// Minor: a tab after ARG or LABEL, and ONBUILD.
		"ARG\tGH_PAT=s3cr3t-j":                        "ARG GH_PAT=••••••",
		"LABEL\ta=s3cr3t-k":                           "LABEL ••••••",
		"ONBUILD ARG GH_PAT=s3cr3t-l":                 "ONBUILD ARG GH_PAT=••••••",
		"ONBUILD LABEL a=s3cr3t-m":                    "ONBUILD LABEL ••••••",
		"onbuild\tlabel a=s3cr3t-n":                   "onbuild\tlabel ••••••",
		"/bin/sh -c #(nop)  ONBUILD ARG X=s3cr3t-o":   "/bin/sh -c #(nop)  ONBUILD ARG X=••••••",
		"/bin/sh -c #(nop)  ONBUILD LABEL a=s3cr3t-p": "/bin/sh -c #(nop)  ONBUILD LABEL ••••••",
		"RUN\t|1 X=s3cr3t-q /bin/sh -c y":             "RUN\t|1 X=•••••• /bin/sh -c y",
	})
}

func check(t *testing.T, cases map[string]string) {
	t.Helper()
	for in, want := range cases {
		got := History(in)
		if got != want {
			t.Errorf("History(%q)\n obtenu  %q\n attendu %q", in, got, want)
		}
		if strings.Contains(got, "s3cr3t") {
			t.Errorf("History(%q) laisse passer un secret : %q", in, got)
		}
	}
}

// A line longer than the bound of Line is masked whole, as by Line.
func TestHistoryLongLine(t *testing.T) {
	in := "|1 X=1 /bin/sh -c echo " + strings.Repeat("a", maxLen)
	if got := History(in); got != Mask {
		t.Errorf("ligne longue : %q", got[:min(len(got), 80)])
	}
}

// The cost stays linear: an attacker chooses the history of an image it
// publishes (as Env and Cmd, review of the Observation phase, I1).
func TestHistoryCost(t *testing.T) {
	within(t, "History", time.Second, func() {
		for range 200 {
			History("|9999 " + strings.Repeat("A=b ", 2000))
			History("RUN |1 " + strings.Repeat("x ", 4000) + " /bin/sh -c y")
		}
	})
}
