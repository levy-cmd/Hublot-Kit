import type { Chunk } from '@codemirror/merge';
import type { Text } from '@codemirror/state';
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

/** Draws d in parent; the returned function removes it. */
export async function mountDiff(parent: HTMLElement, d: DiffInput, label: string): Promise<() => void> {
	const [cm, st, mg] = await Promise.all([import('@codemirror/view'), import('@codemirror/state'), import('@codemirror/merge')]);
	const { EditorView, GutterMarker, gutter, lineNumbers } = cm;
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
	const dark = document.documentElement.dataset.theme !== 'light';
	const common = [
		st.EditorState.readOnly.of(true),
		EditorView.contentAttributes.of({ 'aria-label': label, 'aria-readonly': 'true' }),
		chrome(EditorView, dark)
	];
	let state;
	if (d.kind !== 'modified') {
		state = st.EditorState.create({
			doc: d.text,
			extensions: [...common, lineNumbers(), EditorView.editorAttributes.of({ class: `hk-${d.kind}` })]
		});
	} else {
		const doc = st.Text.of(d.modified.split(/\r\n?|\n/));
		const original = st.Text.of(d.original.split(/\r\n?|\n/));
		const lc = lineChunks(mg.Chunk.build(original, doc), original, doc);
		const lineAt = (v: Editor, pos: number) => v.state.doc.lineAt(pos).number;
		const removed = (v: Editor, pos: number) => deletedAt(lc, lineAt(v, pos));
		state = st.EditorState.create({
			doc,
			extensions: [
				...common,
				mg.unifiedMergeView({ original, mergeControls: false, gutter: false, syntaxHighlightDeletions: false }),
				// Image side: the old number, "+" for a line the container added.
				gutter({
					class: 'hk-num',
					lineMarker: (v, line) => new Nums([String(aLineOf(lc, lineAt(v, line.from)) ?? '+')]),
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
	const view = new EditorView({ parent, state });
	return () => view.destroy();
}
