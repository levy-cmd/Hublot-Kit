// The pages keep their state in the query string (?c=, panel=, path=,
// file=, tab=, state=): links change some keys and keep the others.

/** "?…" with the keys of set changed; null removes a key. */
export function withParams(current: { toString(): string }, set: Record<string, string | null>): string {
	const q = new URLSearchParams(current.toString());
	for (const [k, v] of Object.entries(set)) {
		if (v === null) q.delete(k);
		else q.set(k, v);
	}
	const s = q.toString();
	return s ? `?${s}` : '?';
}
