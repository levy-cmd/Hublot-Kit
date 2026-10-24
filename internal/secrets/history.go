package secrets

import (
	"regexp"
	"strconv"
	"strings"
)

// buildArgs: the prefix of a history line run with build arguments, "|N"
// (classic builder) or "RUN |N" (BuildKit), then a blank.
var buildArgs = regexp.MustCompile(`^(?:RUN\s+)?\|([0-9]+)\s`)

// argName: a word that is an assignment of a build argument (NAME=…).
var argName = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*=`)

// shellMarks end the build arguments: the first one found starts the command.
var shellMarks = []string{" /bin/sh -c ", " /bin/bash -c "}

// anyShell: a shell mark however spaced. After "|N", more than one makes the
// cut unsure: a value may hold a mark itself, and what follows it would only
// go through Line (second review of tranche 5, Codex).
var anyShell = regexp.MustCompile(`/bin/(?:ba)?sh\s+-c`)

// instruction: a line of the ARG or LABEL instruction, as BuildKit records
// it ("ARG GH_PAT=value", "LABEL k=v k2=v2") or the classic builder
// ("/bin/sh -c #(nop)  ARG …"), ONBUILD or not, in any case, followed by
// any blank.
var instruction = regexp.MustCompile(`^((?:/bin/sh -c #\(nop\)\s+)?(?i:ONBUILD\s+)?(?i:ARG|LABEL))\s+(?s:(.*))$`)

// bareName: a build argument without a value ("ARG GH_PAT").
var bareName = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// History masks one line of an image's history (spec v0.2 §5.4). The build
// arguments of "|N NAME=value … /bin/sh -c …" are secrets whatever their
// name (GH_PAT, NPM_AUTH hold no word of §7): every value is masked up to
// the first " /bin/sh -c " or " /bin/bash -c ", the names stay (at most N of
// them). With "|1", the value runs to the shell, blanks included. With N ≥ 2,
// a value with a blank would make its next word pass for a name (review of
// tranche 5: "NPM_AUTH=Basic dXNl…==" let the base64 out): every word must
// be NAME=…, and there must be exactly N of them. Without a shell, with more
// than one, or on any other form, the whole part is masked (fail closed).
// Then the line goes through Line.
//
// BuildKit also keeps "ARG NAME=value" with the value given by --build-arg,
// and "LABEL k=v …" without quotes (measured, Docker 29.7, tranche 5): the
// value of an ARG is masked (names alone, then one NAME=… at the end, else
// the whole part); a LABEL line keeps none of its
// text, since labels leave the agent through Labels only (spec v0.2 §7) and
// "a b=c d" cannot be cut into keys and values.
func History(createdBy string) string {
	if m := instruction.FindStringSubmatch(createdBy); m != nil {
		if strings.HasSuffix(strings.ToUpper(m[1]), "LABEL") {
			return Line(m[1] + " " + Mask)
		}
		return Line(m[1] + " " + maskArgInstruction(m[2]))
	}
	m := buildArgs.FindStringSubmatchIndex(createdBy)
	if m == nil {
		return Line(createdBy)
	}
	n, err := strconv.Atoi(createdBy[m[2]:m[3]])
	if err != nil {
		n = int(^uint(0) >> 1) // more names than words: the words bound it
	}
	pre, rest := createdBy[:m[1]-1], createdBy[m[1]-1:] // rest starts with the blank
	if len(anyShell.FindAllStringIndex(rest, 2)) > 1 {
		return Line(pre + " " + Mask)
	}
	end := -1
	for _, s := range shellMarks {
		if i := strings.Index(rest, s); i >= 0 && (end < 0 || i < end) {
			end = i
		}
	}
	if end < 0 {
		return Line(pre + " " + Mask)
	}
	args, cmd := "", rest[end:]
	if end > 0 {
		args = rest[1:end] // rest[0] is the blank after |N
	}
	if args == "" {
		return Line(pre + cmd)
	}
	return Line(pre + " " + maskArgs(args, n) + cmd)
}

// maskArgs: "NAME=••••••" for the n assignments of a "|N" prefix, or Mask
// alone: the single NAME= of "|1" (its value runs to the shell), else
// exactly n words, each NAME=….
func maskArgs(args string, n int) string {
	words := strings.Fields(args)
	if n == 0 || len(words) == 0 || !argName.MatchString(words[0]) {
		return Mask
	}
	if n == 1 {
		return argName.FindString(words[0]) + Mask
	}
	if len(words) != n {
		return Mask
	}
	names := make([]string, 0, n)
	for _, w := range words {
		name := argName.FindString(w)
		if name == "" {
			return Mask
		}
		names = append(names, name+Mask)
	}
	return strings.Join(names, " ")
}

// maskArgInstruction masks the words of an ARG instruction: names alone,
// then at most one NAME=…, the last word, which keeps its name. An ARG
// gives no count of its arguments, and the rest of a value with a blank
// may look like NAME= ("Basic dXNl…=="): any other form masks the whole
// part.
func maskArgInstruction(args string) string {
	words := strings.Fields(args)
	if len(words) == 0 {
		return Mask
	}
	out := make([]string, 0, len(words))
	for i, w := range words {
		switch name := argName.FindString(w); {
		case name != "" && i == len(words)-1:
			out = append(out, name+Mask)
		case bareName.MatchString(w):
			out = append(out, w)
		default:
			return Mask
		}
	}
	return strings.Join(out, " ")
}
