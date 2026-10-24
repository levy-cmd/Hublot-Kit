import type { Chunk } from '@codemirror/merge';
import type { EditorState, Text } from '@codemirror/state';
import { ApiError, type Change, type Preview } from './api.ts';
import { t } from './i18n/index.ts';
import { withParams } from './url.ts';

// The content diff of a path of F5 (spec v0.2 §5.6): the image's version
// on the left, the container's on the right, computed in the browser from
// the two previews (no new route). Each side shows exactly what its preview
// shows (F2 for the container, v0.2 §5.5 for the image): nothing more.

/** One side as loaded: a preview, or the reason the agent gave for none; null when not asked. */
export type Side = Preview | { kind: 'too_large' } | { kind: 'not_regular' } | { kind: 'missing' } | null;

export type Reason = 'binary' | 'too_large' | 'not_regular' | 'missing';
export type NoDiff = { reason: Reason; side: 'image' | 'container' };

export type DiffInput =
	| { kind: 'added'; text: string }
	| { kind: 'deleted'; text: string }
	// equal: the same text ("bytes"), or the same once line endings are set aside ("eol").
	| { kind: 'modified'; original: string; modified: string; equal: 'bytes' | 'eol' | null };

/** The sides a kind shows: an added file never extracts the image, a deleted one never reads the container. */
export const needs = (kind: Change['kind']): { image: boolean; container: boolean } => ({
	image: kind !== 'added',
	container: kind !== 'deleted'
});

/** The text of a side, or why there is no diff with it. */
function textOf(s: Side, side: NoDiff['side']): string | NoDiff {
	if (s === null || s.kind === 'missing') return { reason: 'missing', side };
	if (s.kind === 'text') return s.text;
	if (s.kind === 'binary' || s.kind === 'image') return { reason: 'binary', side };
	return { reason: s.kind, side };
}

const eol = (s: string) => s.replace(/\r\n?/g, '\n');

/** What to show for change: one side (added, deleted), both (modified), or the reason for none. */
export function sides(change: Change, imageText: Side, containerText: Side): DiffInput | NoDiff {
	const want = needs(change.kind);
	const a = want.image ? textOf(imageText, 'image') : '';
	if (typeof a !== 'string') return a;
	const b = want.container ? textOf(containerText, 'container') : '';
	if (typeof b !== 'string') return b;
	if (change.kind === 'added') return { kind: 'added', text: b };
	if (change.kind === 'deleted') return { kind: 'deleted', text: a };
	return { kind: 'modified', original: a, modified: b, equal: a === b ? 'bytes' : eol(a) === eol(b) ? 'eol' : null };
}

/** The side of an agent error that is a reason for no diff; null for a real failure, told as such. */
export function sideOf(e: unknown): Side {
	if (!(e instanceof ApiError)) return null;
	if (e.code === 'too_large') return { kind: 'too_large' };
	if (e.code === 'not_regular') return { kind: 'not_regular' };
	if (e.code === 'not_found') return { kind: 'missing' };
	return null;
}

export const reasonMessage = (r: NoDiff): string => t(`diff.reason.${r.reason}.${r.side}`);

const KINDS: readonly Change['kind'][] = ['added', 'modified', 'deleted'];

/** The link that opens the diff of change in the content panel; the file preview gives way to it. */
export const diffHref = (current: { toString(): string }, change: Change): string =>
	withParams(current, { panel: 'diff', diff: change.path, change: change.kind, file: null });

/** The change whose diff the URL asks for, or null. */
export function diffChange(q: { get(key: string): string | null }): Change | null {
	const path = q.get('diff');
	const kind = q.get('change') as Change['kind'] | null;
	if (!path || !path.startsWith('/') || !kind || !KINDS.includes(kind)) return null;
	return { path, kind };
}

// Line numbers of each side. The unified view shows the container's text
// (B) as the document, and the image's removed lines (A) as a block above
// the line they precede: the B gutter numbers the document, the A gutter
// maps each line back, and numbers the removed blocks.

/** A chunk in lines, 1-based: aCount lines from aFrom in A replaced by bCount from bFrom in B. */
export type LineChunk = { aFrom: number; aCount: number; bFrom: number; bCount: number };

const lineOf = (doc: Text, pos: number) => doc.lineAt(Math.min(pos, doc.length)).number;
// A chunk's end is one past its last line's break (to = end + 1): "end" is
// on the last line covered, when the chunk covers any.
const count = (doc: Text, from: number, to: number) => (from < to ? lineOf(doc, Math.max(from, to - 1)) - lineOf(doc, from) + 1 : 0);

export function lineChunks(chunks: readonly Chunk[], a: Text, b: Text): LineChunk[] {
	return chunks.map((c) => ({
		aFrom: lineOf(a, c.fromA),
		aCount: count(a, c.fromA, c.toA),
		bFrom: lineOf(b, c.fromB),
		bCount: count(b, c.fromB, c.toB)
	}));
}

/** The A line shown beside line bLine of B; null for a line added in B. */
export function aLineOf(chunks: readonly LineChunk[], bLine: number): number | null {
	let offset = 0;
	for (const c of chunks) {
		if (bLine >= c.bFrom && bLine < c.bFrom + c.bCount) return null;
		if (c.bFrom + c.bCount <= bLine) offset += c.aCount - c.bCount;
	}
	return bLine + offset;
}

/** The A lines removed just above line bLine of B (bLine = last + 1: after the end). */
export function deletedAt(chunks: readonly LineChunk[], bLine: number): number[] {
	const c = chunks.find((c) => c.bFrom === bLine && c.aCount > 0);
	return c ? Array.from({ length: c.aCount }, (_, i) => c.aFrom + i) : [];
}

// The view itself: CodeMirror, read only (EditorState.readOnly), with
// @codemirror/merge for a modified file. Loaded with the first diff, apart
// from the initial bundle. The texts go in as documents: CodeMirror draws
// them as text nodes (the removed lines too, createTextNode), never as HTML.

type View = (typeof import('@codemirror/view'))['EditorView'];

const chrome = (EditorView: View, dark: boolean) =>
	EditorView.theme(
		{
			'&': { backgroundColor: 'var(--releve)', color: 'var(--texte)', fontSize: 'var(--t-xs)' },
			'.cm-content': { fontFamily: 'var(--mono)' },
			'.cm-gutters': { backgroundColor: 'var(--surface)', color: 'var(--texte-doux)', borderRight: '1px solid var(--trait)' },
			'.hk-num .cm-gutterElement': { padding: '0 0.4rem', textAlign: 'right', fontFamily: 'var(--mono)' },
			'&.cm-focused': { outline: '2px solid var(--focus)' },
			'&.cm-focused .cm-selectionBackground, .cm-selectionBackground, ::selection': { backgroundColor: 'var(--selection)' },
			'&.cm-merge-b .cm-changedLine, &.hk-added .cm-line': { backgroundColor: 'var(--diff-ajout)' },
			'.cm-deletedChunk, &.hk-deleted .cm-line': { backgroundColor: 'var(--diff-suppr)' },
			'&.cm-merge-b .cm-changedText': {
				background: 'linear-gradient(var(--emeraude-texte), var(--emeraude-texte)) bottom/100% 2px no-repeat'
			},
			'&.cm-merge-b .cm-deletedText, .cm-deletedChunk .cm-deletedText': {
				background: 'linear-gradient(var(--rouge-texte), var(--rouge-texte)) bottom/100% 2px no-repeat'
			}
		},
		{ dark }
	);

// The diff of the view, bounded for any input (the texts come from the
// image's author). @codemirror/merge's own diff is not: scanLimit and
// timeout leave findMatch unbounded, and a repetitive line of 300 KB moved
// held the tab 12 to 20 s, a line of 2 MiB minutes. So the view gets this
// diff (diffConfig.override):
// - by lines, Myers with at most maxD edits, maxD such that lines × edits
//   stays under LINE_DIFF_BUDGET (and 1000 at most, for the memory of the
//   path); past it, the lines between the common start and end are one
//   replaced block: less precise, never slow;
// - each changed block of at most REFINE_MAX characters a side is refined
//   to the characters by the package's diff, REFINE_BUDGET characters in
//   all: a cost bounded by the size of the blocks, whatever they hold.

/** A change in character offsets: a[fromA, toA) replaced by b[fromB, toB). */
export type Span = { fromA: number; toA: number; fromB: number; toB: number };

export const LINE_DIFF_BUDGET = 10_000_000;
export const MAX_EDITS = 1000;
export const REFINE_MAX = 4096;
export const REFINE_BUDGET = 65_536;

/**
 * The edits the line diff may look for among size lines: its work is at
 * most lines × edits, and the memory of its path edits².
 */
export const editLimit = (size: number): number => Math.min(MAX_EDITS, Math.floor(LINE_DIFF_BUDGET / Math.max(1, size)));

/** Line blocks [i0, i1) of A replaced by [j0, j1) of B, or null past maxD edits. */
function myers(A: Int32Array, B: Int32Array, maxD: number): [number, number, number, number][] | null {
	const n = A.length;
	const m = B.length;
	const off = maxD + 1;
	const v = new Int32Array(2 * maxD + 3);
	const trace: Int32Array[] = [];
	let found = -1;
	for (let d = 0; d <= maxD && found < 0; d++) {
		trace.push(v.slice(off - d - 1, off + d + 2)); // v[k] for k in [-d-1, d+1]
		for (let k = -d; k <= d; k += 2) {
			let x = k === -d || (k !== d && v[off + k - 1] < v[off + k + 1]) ? v[off + k + 1] : v[off + k - 1] + 1;
			let y = x - k;
			while (x < n && y < m && A[x] === B[y]) {
				x++;
				y++;
			}
			v[off + k] = x;
			if (x >= n && y >= m) {
				found = d;
				break;
			}
		}
	}
	if (found < 0) return null;
	// Back from (n, m): one edit per d, gathered into blocks.
	const blocks: [number, number, number, number][] = [];
	let x = n;
	let y = m;
	for (let d = found; d > 0; d--) {
		const t = trace[d]; // v before step d: t[k + d + 1] = v[k]
		const at = (k: number) => t[k + d + 1];
		const k = x - y;
		const pk = k === -d || (k !== d && at(k - 1) < at(k + 1)) ? k + 1 : k - 1;
		const px = at(pk);
		const py = px - pk;
		// One step from (px, py), down (a line of B) or right (a line of A),
		// to (sx, sy); then equal lines up to (x, y).
		const sx = pk === k + 1 ? px : px + 1;
		const sy = sx - k;
		const last = blocks[blocks.length - 1];
		if (last && last[0] === sx && last[2] === sy) {
			last[0] = px;
			last[2] = py;
		} else blocks.push([px, sx, py, sy]);
		x = px;
		y = py;
	}
	return blocks.reverse();
}

/**
 * The changes from a to b, bounded (see above); refine: the character diff
 * of a small block; coarse: called when the diff falls back to one block.
 */
export function lineDiff(a: string, b: string, refine?: (a: string, b: string) => readonly Span[], coarse?: () => void): Span[] {
	// A sentinel line end: every line ends with "\n", the blocks are clean
	// slices; offsets past the real ends are brought back below.
	const la = (a + '\n').split('\n');
	const lb = (b + '\n').split('\n');
	la.pop();
	lb.pop();
	const ids = new Map<string, number>();
	const id = (s: string) => {
		let i = ids.get(s);
		if (i === undefined) ids.set(s, (i = ids.size));
		return i;
	};
	const A = Int32Array.from(la, id);
	const B = Int32Array.from(lb, id);
	let p = 0;
	while (p < A.length && p < B.length && A[p] === B[p]) p++;
	let s = 0;
	while (s < A.length - p && s < B.length - p && A[A.length - 1 - s] === B[B.length - 1 - s]) s++;
	const midA = A.subarray(p, A.length - s);
	const midB = B.subarray(p, B.length - s);
	const size = midA.length + midB.length;
	if (size === 0) return [];
	const maxD = editLimit(size);
	const found = myers(midA, midB, maxD);
	if (!found) coarse?.();
	const blocks = found?.map(([i0, i1, j0, j1]) => [i0 + p, i1 + p, j0 + p, j1 + p]) ?? [[p, A.length - s, p, B.length - s]];
	const starts = (lines: string[]) => {
		const o = new Int32Array(lines.length + 1);
		for (let i = 0; i < lines.length; i++) o[i + 1] = o[i] + lines[i].length + 1;
		return o;
	};
	const sa = starts(la);
	const sb = starts(lb);
	const out: Span[] = [];
	let refined = 0;
	for (const [i0, i1, j0, j1] of blocks) {
		let span = { fromA: sa[i0], toA: sa[i1], fromB: sb[j0], toB: sb[j1] };
		// A block that reaches the end: it holds the sentinel; the line end
		// before it is taken instead (both sides reach the end together).
		if (span.toA > a.length || span.toB > b.length) {
			if (span.fromA > 0 && span.fromB > 0) span = { fromA: span.fromA - 1, toA: span.toA - 1, fromB: span.fromB - 1, toB: span.toB - 1 };
			span = { ...span, toA: Math.min(span.toA, a.length), toB: Math.min(span.toB, b.length) };
		}
		const lenA = span.toA - span.fromA;
		const lenB = span.toB - span.fromB;
		if (refine && lenA > 0 && lenB > 0 && lenA <= REFINE_MAX && lenB <= REFINE_MAX && refined + lenA + lenB <= REFINE_BUDGET) {
			refined += lenA + lenB;
			for (const c of refine(a.slice(span.fromA, span.toA), b.slice(span.fromB, span.toB)))
				out.push({ fromA: c.fromA + span.fromA, toA: c.toA + span.fromA, fromB: c.fromB + span.fromB, toB: c.toB + span.fromB });
		} else out.push(span);
	}
	return out;
}

type MergeMod = typeof import('@codemirror/merge');

const numbered = new WeakMap<readonly Chunk[], LineChunk[]>();

/** The chunks the view shows, in lines: the numbers of the gutters come from them only. */
export function numbering(mg: MergeMod, state: EditorState): LineChunk[] {
	const c = mg.getChunks(state);
	if (!c) return [];
	let lc = numbered.get(c.chunks);
	if (!lc) numbered.set(c.chunks, (lc = lineChunks(c.chunks, mg.getOriginalDoc(state), state.doc)));
	return lc;
}

/** The read-only state that shows d (dark: the page's theme; coarse: see lineDiff). */
export async function diffState(d: DiffInput, label: string, dark: boolean, coarse?: () => void): Promise<EditorState> {
	const [cm, st, mg] = await Promise.all([import('@codemirror/view'), import('@codemirror/state'), import('@codemirror/merge')]);
	const { EditorView, GutterMarker, gutter, highlightSpecialChars, lineNumbers } = cm;
	type Editor = InstanceType<View>;
	// A number, or a column of them for a block of removed lines (one per
	// line of the block, at the editor's line height).
	class Nums extends GutterMarker {
		readonly texts: string[];
		constructor(texts: string[]) {
			super();
			this.texts = texts;
		}
		eq(o: InstanceType<typeof GutterMarker>) {
			return o instanceof Nums && o.texts.join() === this.texts.join();
		}
		toDOM(v: Editor) {
			const box = document.createElement('div');
			for (const s of this.texts) {
				const line = box.appendChild(document.createElement('div'));
				line.style.height = `${v.defaultLineHeight}px`;
				line.textContent = s;
			}
			return box;
		}
	}
	const common = [
		st.EditorState.readOnly.of(true),
		// Control and bidi characters (U+202E…) shown as such, as in the editor.
		highlightSpecialChars(),
		EditorView.contentAttributes.of({ 'aria-label': label, 'aria-readonly': 'true' }),
		chrome(EditorView, dark)
	];
	if (d.kind !== 'modified')
		return st.EditorState.create({
			doc: d.text,
			extensions: [...common, lineNumbers(), EditorView.editorAttributes.of({ class: `hk-${d.kind}` })]
		});
	const doc = st.Text.of(d.modified.split(/\r\n?|\n/));
	const original = st.Text.of(d.original.split(/\r\n?|\n/));
	const lineAt = (v: Editor, pos: number) => v.state.doc.lineAt(pos).number;
	const removed = (v: Editor, pos: number) => deletedAt(numbering(mg, v.state), lineAt(v, pos));
	return st.EditorState.create({
		doc,
		extensions: [
			...common,
			mg.unifiedMergeView({
				original,
				mergeControls: false,
				gutter: false,
				syntaxHighlightDeletions: false,
				diffConfig: {
					override: (x, y) =>
						lineDiff(x, y, (p, q) => mg.diff(p, q, { scanLimit: 500 }), coarse).map(
							(c) => new mg.Change(c.fromA, c.toA, c.fromB, c.toB)
						)
				}
			}),
			// Image side: the old number, "+" for a line the container added.
			gutter({
				class: 'hk-num',
				lineMarker: (v, line) => new Nums([String(aLineOf(numbering(mg, v.state), lineAt(v, line.from)) ?? '+')]),
				widgetMarker: (v, _w, block) => {
					const n = removed(v, block.from);
					return n.length ? new Nums(n.map(String)) : null;
				},
				initialSpacer: () => new Nums([String(original.lines)])
			}),
			// Container side: the new number, "−" for a removed line.
			gutter({
				class: 'hk-num',
				lineMarker: (v, line) => new Nums([String(lineAt(v, line.from))]),
				widgetMarker: (v, _w, block) => {
					const n = removed(v, block.from);
					return n.length ? new Nums(n.map(() => '−')) : null;
				},
				initialSpacer: () => new Nums([String(doc.lines)])
			})
		]
	});
}

/** Draws d in parent: remove takes it away; coarse, the diff fell back to one block (lineDiff). */
export async function mountDiff(parent: HTMLElement, d: DiffInput, label: string): Promise<{ remove: () => void; coarse: boolean }> {
	const { EditorView } = await import('@codemirror/view');
	let coarse = false;
	const state = await diffState(d, label, document.documentElement.dataset.theme !== 'light', () => (coarse = true));
	const view = new EditorView({ parent, state });
	return { remove: () => view.destroy(), coarse };
}
