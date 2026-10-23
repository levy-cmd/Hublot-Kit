package secrets

import (
	"sort"
	"strings"
)

// Label is one label of a container (spec v0.2 §5.2). Value is nil when
// masked, as for a variable (spec §7: null with masked: true).
type Label struct {
	Key    string  `json:"key"`
	Value  *string `json:"value"`
	Masked bool    `json:"masked"`
}

// safeLabelPrefixes: labels written by Compose and the OCI image labels.
// Third-party tools put secrets in labels whose keys hold no word of §7
// (traefik…basicauth.users=admin:$apr1$…), so the rule is a whitelist.
var safeLabelPrefixes = []string{"com.docker.compose.", "org.opencontainers.image."}

// SafeLabel: the value of label key may be shown, by the whitelist of spec
// v0.2 §5.2 and by §7 (Masked), which still applies under the whitelist: a
// label set by hand under com.docker.compose. may hold anything.
func SafeLabel(key, value string) bool {
	for _, p := range safeLabelPrefixes {
		if strings.HasPrefix(key, p) {
			return !Masked(key, value)
		}
	}
	return false
}

// Labels returns every label sorted by key: the value of a safe one
// (SafeLabel), the key alone for the others. There is no reveal of labels
// (Q60): their values can be read on the host with docker inspect.
func Labels(m map[string]string) []Label {
	out := make([]Label, 0, len(m))
	for k, v := range m {
		l := Label{Key: k, Masked: true}
		if SafeLabel(k, v) {
			l.Value, l.Masked = &v, false
		}
		out = append(out, l)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out
}
