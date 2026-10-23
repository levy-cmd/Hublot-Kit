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
		"|1 A=s3cr3t-j /bin/bash -c x /bin/sh -c y": "|1 A=•••••• /bin/bash -c x /bin/sh -c y",
		// A name that is not ASCII is not a name: part of the value.
		"|2 X=a ÉCHO=s3cr3t-k /bin/sh -c y": "|2 X=•••••• /bin/sh -c y",
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
