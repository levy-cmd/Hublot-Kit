package secrets

import (
	"regexp"
	"strconv"
	"strings"
)

// buildArgs: the prefix of a history line run with build arguments, "|N"
// (classic builder) or "RUN |N" (BuildKit), then a blank.
var buildArgs = regexp.MustCompile(`^(?:RUN )?\|([0-9]+) `)

// argName: a word that starts an assignment of the prefix (NAME=).
var argName = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_.-]*=`)

// shellMarks end the build arguments: the first one found starts the command.
var shellMarks = []string{" /bin/sh -c ", " /bin/bash -c "}

// instruction: a line of the ARG or LABEL instruction, as BuildKit records
// it ("ARG GH_PAT=value", "LABEL k=v k2=v2") or the classic builder
// ("/bin/sh -c #(nop)  ARG …"), in any case.
var instruction = regexp.MustCompile(`^((?:/bin/sh -c #\(nop\) +)?(?i:ARG|LABEL)) (?s:(.*))$`)

// bareName: a build argument without a value ("ARG GH_PAT").
var bareName = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_.-]*$`)

// History masks one line of an image's history (spec v0.2 §5.4). The build
// arguments of "|N NAME=value … /bin/sh -c …" are secrets whatever their
// name (GH_PAT, NPM_AUTH hold no word of §7): every value is masked up to
// the first " /bin/sh -c " or " /bin/bash -c ", the names stay (at most N of
// them: a value may hold blanks and "=", the count does not find its end).
// Without a shell, or if the first word is not NAME=, the whole part is
// masked (fail closed). Then the line goes through Line.
//
// BuildKit also keeps "ARG NAME=value" with the value given by --build-arg,
// and "LABEL k=v …" without quotes (measured, Docker 29.7, tranche 5): the
// values of an ARG are masked as after "|N"; a LABEL line keeps none of its
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
	end := -1
	for _, s := range shellMarks {
		if i := strings.Index(rest, s); i >= 0 && (end < 0 || i < end) {
			end = i
		}
	}
	if end < 0 {
		return Line(pre + " " + Mask)
	}
	args, cmd := strings.TrimPrefix(rest[:end], " "), rest[end:]
	if args == "" {
		return Line(pre + cmd)
	}
	return Line(pre + " " + maskArgs(args, n) + cmd)
}

// maskArgs: "NAME=••••••" for each of the first n words that start an
// assignment, the other words being part of a value; Mask alone if the
// first word does not start one (or n is 0: values without a name).
func maskArgs(args string, n int) string {
	words := strings.Split(args, " ")
	if n == 0 || !argName.MatchString(words[0]) {
		return Mask
	}
	var names []string
	for _, w := range words {
		if len(names) == n {
			break
		}
		if name := argName.FindString(w); name != "" {
			names = append(names, name+Mask)
		}
	}
	return strings.Join(names, " ")
}

// maskArgInstruction masks the words of an ARG instruction: a name alone
// stays until the first NAME=value; then each NAME= keeps its name and
// masks its value, the other words being part of a value. Mask alone if the
// first word is neither.
func maskArgInstruction(args string) string {
	words := strings.Fields(args)
	var out []string
	inValue := false
	for i, w := range words {
		switch name := argName.FindString(w); {
		case name != "":
			out, inValue = append(out, name+Mask), true
		case !inValue && bareName.MatchString(w):
			out = append(out, w)
		case i == 0:
			return Mask
		}
	}
	if len(out) == 0 {
		return Mask
	}
	return strings.Join(out, " ")
}
