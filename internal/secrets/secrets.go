// Package secrets masks the values the radiography shows (F7, spec §7): an
// environment variable whose name says it is a secret, or whose value is a
// URL with user information, never leaves the agent unless revealed.
package secrets

import (
	"net/url"
	"path"
	"regexp"
	"slices"
	"strings"
	"unicode"
)

// maxLen: a text longer than this is masked whole (fail closed). It bounds
// the work done per value: the image or the creator of a container chooses
// the size of Env and Cmd (review, round 1, I1).
const maxLen = 8 << 10

// Mask replaces a masked part of a command argument.
const Mask = "••••••"

// words of spec §7, matched anywhere in a name, ignoring case.
var words = []string{"PASSWORD", "PASSWD", "PASS", "TOKEN", "KEY", "SECRET", "CREDENTIAL", "PRIVATE", "DSN"}

// SecretName: the name contains one of the words of spec §7, ignoring case
// (KEYBOARD and COMPASS too: the rule errs on the safe side).
func SecretName(name string) bool {
	// Fail closed on look-alikes (Codex): drop the format characters
	// (zero width…), then a non-ASCII letter makes the name suspect.
	name = strings.Map(func(r rune) rune {
		if unicode.Is(unicode.Cf, r) {
			return -1
		}
		return r
	}, name)
	for _, r := range name {
		if r > unicode.MaxASCII && unicode.IsLetter(r) {
			return true
		}
	}
	up := strings.ToUpper(name)
	for _, w := range words {
		if strings.Contains(up, w) {
			return true
		}
	}
	return false
}

// userinfo: "://", then an "@" before any blank (plan Observation, Q26:
// a password or a token alone; greedy, to the last "@" of the word).
var userinfo = regexp.MustCompile(`://\S*@`)

// URLCredentials: v holds a URL with user information.
func URLCredentials(v string) bool { return userinfo.MatchString(v) }

// Var is one variable of the radiography. Value is nil when masked (spec
// §7: null with masked: true).
type Var struct {
	Name   string  `json:"name"`
	Value  *string `json:"value"`
	Masked bool    `json:"masked"`
}

// Masked: the variable must not be shown as is (spec §7): a secret name,
// or a value in which the rules of the command find something to mask
// (plan Observation, Q36: a URL with user information, "…;Pwd=…;",
// "?password=…", "-D…Password\=…", "--requirepass …"), the value read as
// is and percent-decoded ("?pass%77ord=…").
func Masked(name, value string) bool {
	if len(value) > maxLen {
		return true
	}
	if SecretName(name) || maskText(value) != value {
		return true
	}
	d, err := url.PathUnescape(value)
	return err == nil && d != value && maskText(d) != d
}

// Env masks a Config.Env ("NAME=value"; "NAME" alone has an empty value),
// in order.
func Env(env []string) []Var {
	out := make([]Var, 0, len(env))
	for _, kv := range env {
		name, value, _ := strings.Cut(kv, "=")
		v := Var{Name: name, Masked: Masked(name, value)}
		if !v.Masked {
			v.Value = &value
		}
		out = append(out, v)
	}
	return out
}

// Lookup returns the value of variable name, exact name: the last one if
// it appears twice (Docker removes duplicates when it creates a container).
func Lookup(env []string, name string) (string, bool) {
	value, found := "", false
	for _, kv := range env {
		if n, v, _ := strings.Cut(kv, "="); n == name && name != "" {
			value, found = v, true
		}
	}
	return value, found
}

// value: a word, possibly quoted or partly quoted ("x", 'x', a"b c"d); an
// unclosed quote runs to the end (review I1: nothing after it leaks).
const value = `(?:"[^"]*"?|'[^']*'?|\\.|[^\s;&|"'()])+`

// assignment: the key of NAME=value or --name=value inside a text (a shell
// line, a connection string): the leftmost run of name characters is
// preceded by anything that cannot be part of a name (review 2, Minor 5:
// ":", "/", "{", "[" too); "\=" (an escaped "=", Java properties) counts.
var assignment = regexp.MustCompile(`(-{0,2}[A-Za-z0-9_.-]+)(\\?=)`)

// valueAt: the value that starts a text.
var valueAt = regexp.MustCompile(`^` + value)

// bearer: the credential of an Authorization header (round 1, Minor).
var bearer = regexp.MustCompile(`(?i)\b((?:bearer|authorization\s*[:=]\s*basic)\s+)[^\s'"]+`)

// jsonKey: what follows a word of §7 that is a JSON key ("password":"x").
var jsonKey = regexp.MustCompile(`^"\s*:\s*(?:["\d\[{]|true|false|null)`)

// pwd: Pwd=, the password key of ODBC and .NET connection strings. Only
// as an assignment inside a text: the variable PWD is the working
// directory, not a secret.
var pwd = regexp.MustCompile(`(?i)pwd`)

// secretKey: the name of an assignment whose value is a secret.
func secretKey(name string) bool { return SecretName(name) || pwd.MatchString(name) }

// flagValue: a flag, blanks, then a word that is not a flag, inside a shell
// line (sh -c "… --requirepass x …").
var flagValue = regexp.MustCompile(`(^|\s)(-{1,2}[A-Za-z0-9_.-]+)(\s+)((?:"[^"]*"?|'[^']*'?|\\.|[^\s;&|"'()-])(?:"[^"]*"?|'[^']*'?|\\.|[^\s;&|"'()])*)`)

// flag: an argument that is only a flag ("--password", or "--password="
// whose value is the next argument).
var flag = regexp.MustCompile(`^-{1,2}[A-Za-z0-9_.-]+=?$`)

// shells and shellFlag: "sh -c", "bash -lc", "ash -ec": the next argument is
// a shell line (review 2, Minor 1: "postgres -c name=value" is not one).
var (
	shells    = []string{"sh", "bash", "ash", "dash", "zsh", "ksh"}
	shellFlag = regexp.MustCompile(`^-[A-Za-z]*c$`)
)

// whole: an argument NAME=value or --name=value, outside a shell line: the
// value is the whole rest of the argument, blanks included.
var whole = regexp.MustCompile(`(?s)^(-{0,2}[A-Za-z0-9_.-]+)=(.+)$`)

// word: a word of spec §7, anywhere, ignoring case.
var word = regexp.MustCompile(`(?i)` + strings.Join(words, "|"))

// maskText masks a text: user information of URLs, the credential of an
// Authorization header, then the value of each secret assignment (a key
// inside the value of another is found as the scan goes on: --env=DB_PASSWORD=x;
// one pass, linear), then the word after a secret flag.
func maskText(s string) string {
	s = userinfo.ReplaceAllString(s, "://"+Mask+"@")
	s = bearer.ReplaceAllString(s, "${1}"+Mask)
	var b strings.Builder
	last, pos := 0, 0
	for pos < len(s) {
		loc := assignment.FindStringSubmatchIndex(s[pos:])
		if loc == nil {
			break
		}
		start, end := pos, pos+loc[1]
		pos = end
		if !secretKey(s[start+loc[2] : start+loc[3]]) {
			continue
		}
		if v := valueAt.FindStringIndex(s[end:]); v != nil {
			b.WriteString(s[last:end])
			b.WriteString(Mask)
			last, pos = end+v[1], end+v[1]
		}
	}
	b.WriteString(s[last:])
	s = b.String()
	return flagValue.ReplaceAllStringFunc(s, func(m string) string {
		sub := flagValue.FindStringSubmatch(m)
		if !SecretName(sub[2]) || sub[4] == Mask {
			return m
		}
		return sub[1] + sub[2] + sub[3] + Mask
	})
}

// blank: what ends a token for leaks.
const blank = " \t\r\n\"'"

// alnum: a letter or a digit.
var alnum = regexp.MustCompile(`[A-Za-z0-9]`)

// leaks: a word of §7 whose own value is not masked (review 2: the link is
// local, a mask elsewhere in the text does not cover it). In its token (a
// run without blank or quote), a word is
//   - covered when the token ends with a mask after it (only punctuation
//     may follow: "--password=••••••;"), or when the token is a flag and
//     the next token is a mask ("--requirepass ••••••");
//   - harmless in a path ("/" before it in the token, no "=" or ":" after
//     it) or in a plain word ("ssh-keyscan", "reset your password");
//   - a leak when the token is a flag or holds "=" or ":" after the word.
//
// An argument that is only a flag holds no value: never a leak.
func leaks(s string) bool {
	if flag.MatchString(s) {
		return false
	}
	// One pass: the token (a run without blank or quote) is found once, and
	// its first "/" and the next "=" or ":" are tracked as the words go by
	// (round 1, I1: linear).
	var st, slash, sep int
	tokEn := 0
	for _, loc := range word.FindAllStringIndex(s, -1) {
		if loc[0] >= tokEn {
			st = tokEn + strings.LastIndexAny(s[tokEn:loc[0]], blank) + 1
			tokEn = len(s)
			if i := strings.IndexAny(s[loc[1]:], blank); i >= 0 {
				tokEn = loc[1] + i
			}
			slash, sep = -1, -1
			if i := strings.IndexByte(s[st:tokEn], '/'); i >= 0 {
				slash = st + i
			}
		}
		if sep < loc[1] {
			sep = tokEn // none
			if i := strings.IndexAny(s[loc[1]:tokEn], "=:"); i >= 0 {
				sep = loc[1] + i
			}
		}
		bound := sep < tokEn
		if bound && strings.HasPrefix(s[sep+1:tokEn], Mask) && !alnum.MatchString(s[sep+1+len(Mask):tokEn]) {
			continue
		}
		isFlag := s[st] == '-'
		if isFlag && !bound && strings.HasPrefix(strings.TrimLeft(s[tokEn:], " \t"), Mask) {
			continue
		}
		if tokEn < len(s) && s[tokEn] == '"' && jsonKey.MatchString(s[tokEn:min(len(s), tokEn+64)]) {
			bound = true // "password":"x" (JSON): the separator is after the quote
		}
		if slash >= 0 && slash < loc[0] && !bound {
			continue
		}
		if isFlag || bound {
			return true
		}
	}
	return false
}

// Line masks one text (a message of Docker, a shell line): maskText, then,
// if a word of §7 is left with nothing masked after it, the whole text.
func Line(s string) string {
	if len(s) > maxLen {
		return Mask
	}
	if m := maskText(s); !leaks(m) {
		return m
	}
	return Mask
}

// Args masks a command or an entrypoint (plan Observation, Q27), in a copy.
// The argument after a secret flag is masked whole: always after
// "--password=", unless it is a flag itself after "--requirepass". The
// argument after the "-c" of a shell (sh -c, bash -lc…) is a shell line:
// Line. Any other argument NAME=value or --name=value keeps its name and
// masks its whole value if the name is secret (blanks included), else the
// value goes through the rules of Line. masked tells whether anything was
// hidden.
func Args(args []string) (out []string, masked bool) {
	if args == nil {
		return nil, false
	}
	out = make([]string, len(args))
	for i, a := range args {
		prev := ""
		if i > 0 {
			prev = args[i-1]
		}
		if len(a) > maxLen {
			out[i], masked = Mask, true
			continue
		}
		switch w := whole.FindStringSubmatch(a); {
		case flag.MatchString(prev) && SecretName(prev) && (strings.HasSuffix(prev, "=") || !strings.HasPrefix(a, "-")):
			out[i] = Mask // "--password=" then any value; "--password" then a non-flag
		case w == nil, i > 0 && shellFlag.MatchString(prev) && slices.Contains(shells, path.Base(args[0])):
			out[i] = Line(a) // not an assignment, or the line of "sh -c"
		case secretKey(w[1]):
			out[i] = w[1] + "=" + Mask
		default:
			out[i] = w[1] + "=" + maskText(w[2])
			if leaks(out[i]) {
				out[i] = Mask
			}
		}
		masked = masked || out[i] != a
	}
	return out, masked
}
