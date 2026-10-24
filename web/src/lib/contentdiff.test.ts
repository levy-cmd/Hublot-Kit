import { describe, expect, it } from 'vitest';
import * as merge from '@codemirror/merge';
import { Chunk } from '@codemirror/merge';
import { Text } from '@codemirror/state';
import { ApiError, type Change, type Preview } from './api.ts';
import {
	aLineOf,
	deletedAt,
	diffChange,
	diffHref,
	diffState,
	lineChunks,
	numbering,
	needs,
	reasonMessage,
	sideOf,
	editLimit,
	LINE_DIFF_BUDGET,
	lineDiff,
	MAX_EDITS,
	REFINE_MAX,
	sides,
	type Side,
	type Span
} from './contentdiff.ts';

const text = (s: string, utf8 = true): Preview => ({ kind: 'text', text: s, etag: 'e', utf8 });
const binary: Preview = { kind: 'binary' };
const png: Preview = { kind: 'image', blob: new Blob(['x']), etag: 'e' };
const change = (kind: Change['kind'], path = '/etc/motd'): Change => ({ path, kind });

describe('sides: the three kinds of F5 (spec v0.2 §5.6)', () => {
	it('modified: the image on the left, the container on the right', () => {
		expect(sides(change('modified'), text('a\n'), text('a\nb\n'))).toEqual({
			kind: 'modified',
			original: 'a\n',
			modified: 'a\nb\n',
			equal: null
		});
	});
	it('added: the container alone, the image side never read', () => {
		expect(sides(change('added'), null, text('new\n'))).toEqual({ kind: 'added', text: 'new\n' });
		// Even an image side given by mistake is not shown.
		expect(sides(change('added'), text('old'), text('new\n'))).toEqual({ kind: 'added', text: 'new\n' });
	});
	it('deleted: the image alone', () => {
		expect(sides(change('deleted'), text('old\n'), null)).toEqual({ kind: 'deleted', text: 'old\n' });
		expect(sides(change('deleted'), text('old\n'), text('x'))).toEqual({ kind: 'deleted', text: 'old\n' });
	});
	it('modified with the same text: said, with line endings apart', () => {
		expect(sides(change('modified'), text('a\n'), text('a\n'))).toMatchObject({ kind: 'modified', equal: 'bytes' });
		expect(sides(change('modified'), text('a\r\nb\r\n'), text('a\nb\n'))).toMatchObject({ kind: 'modified', equal: 'eol' });
		expect(sides(change('modified'), text('a\rb'), text('a\nb'))).toMatchObject({ kind: 'modified', equal: 'eol' });
	});
});

describe('sides: no diff, a reason (spec v0.2 §5.6)', () => {
	const cases: [string, Side, Side, string, 'image' | 'container'][] = [
		['binary in the image', binary, text('a'), 'binary', 'image'],
		['binary in the container', text('a'), binary, 'binary', 'container'],
		['a picture is binary', png, text('a'), 'binary', 'image'],
		['a picture in the container', text('a'), png, 'binary', 'container'],
		['too large in the image', { kind: 'too_large' }, text('a'), 'too_large', 'image'],
		['too large in the container', text('a'), { kind: 'too_large' }, 'too_large', 'container'],
		['not regular in the image', { kind: 'not_regular' }, text('a'), 'not_regular', 'image'],
		['not regular in the container', text('a'), { kind: 'not_regular' }, 'not_regular', 'container'],
		['missing from the image', { kind: 'missing' }, text('a'), 'missing', 'image'],
		['missing from the container', text('a'), { kind: 'missing' }, 'missing', 'container'],
		['image side not loaded', null, text('a'), 'missing', 'image'],
		['container side not loaded', text('a'), null, 'missing', 'container']
	];
	for (const [name, img, ctr, reason, side] of cases) {
		it(`modified, ${name}`, () => {
			expect(sides(change('modified'), img, ctr)).toEqual({ reason, side });
		});
	}
	it('the image side is told first when both have a reason', () => {
		expect(sides(change('modified'), { kind: 'too_large' }, binary)).toEqual({ reason: 'too_large', side: 'image' });
	});
	it('added: the reason of the container side only', () => {
		expect(sides(change('added'), { kind: 'too_large' }, binary)).toEqual({ reason: 'binary', side: 'container' });
		expect(sides(change('added'), null, { kind: 'too_large' })).toEqual({ reason: 'too_large', side: 'container' });
		expect(sides(change('added'), null, { kind: 'not_regular' })).toEqual({ reason: 'not_regular', side: 'container' });
		expect(sides(change('added'), null, null)).toEqual({ reason: 'missing', side: 'container' });
	});
	it('deleted: the reason of the image side only', () => {
		expect(sides(change('deleted'), binary, { kind: 'too_large' })).toEqual({ reason: 'binary', side: 'image' });
		expect(sides(change('deleted'), { kind: 'missing' }, null)).toEqual({ reason: 'missing', side: 'image' });
		expect(sides(change('deleted'), { kind: 'not_regular' }, null)).toEqual({ reason: 'not_regular', side: 'image' });
		expect(sides(change('deleted'), { kind: 'too_large' }, text('x'))).toEqual({ reason: 'too_large', side: 'image' });
	});
	it('text that is not valid UTF-8 is compared as the preview shows it', () => {
		expect(sides(change('modified'), text('a�', false), text('b'))).toMatchObject({ original: 'a�' });
	});
	it('every reason has a message for each side', () => {
		for (const r of ['binary', 'too_large', 'not_regular', 'missing'] as const)
			for (const s of ['image', 'container'] as const) {
				const m = reasonMessage({ reason: r, side: s });
				expect(m).not.toBe('');
				expect(m).not.toMatch(/^diff\./); // a key, not a text
			}
		expect(reasonMessage({ reason: 'missing', side: 'image' })).not.toBe(reasonMessage({ reason: 'missing', side: 'container' }));
	});
});

describe('sideOf: the agent errors that are a reason, not a failure', () => {
	it('maps too_large, not_regular and not_found', () => {
		expect(sideOf(new ApiError(413, 'too_large'))).toEqual({ kind: 'too_large' });
		expect(sideOf(new ApiError(422, 'not_regular'))).toEqual({ kind: 'not_regular' });
		expect(sideOf(new ApiError(404, 'not_found'))).toEqual({ kind: 'missing' });
	});
	it('leaves the others to the error message', () => {
		for (const code of ['indexing', 'forbidden_path', 'link_in_path', 'internal', 'network', 'unsupported_image', 'container_changed'])
			expect(sideOf(new ApiError(500, code))).toBeNull();
		expect(sideOf(new Error('x'))).toBeNull();
	});
});

describe('needs: only the sides a kind shows are asked for', () => {
	it('never extracts the image for an added file, never reads the container for a deleted one', () => {
		expect(needs('added')).toEqual({ image: false, container: true });
		expect(needs('deleted')).toEqual({ image: true, container: false });
		expect(needs('modified')).toEqual({ image: true, container: true });
	});
});

describe('diff in the URL', () => {
	it('opens the diff of a path, the file preview closed', () => {
		const href = diffHref(new URLSearchParams('c=abc&panel=files&path=%2Fetc&file=%2Fetc%2Fhosts'), change('modified', '/etc/motd'));
		const q = new URLSearchParams(href.slice(1));
		expect(q.get('c')).toBe('abc');
		expect(q.get('panel')).toBe('diff');
		expect(q.get('diff')).toBe('/etc/motd');
		expect(q.get('change')).toBe('modified');
		expect(q.has('file')).toBe(false);
	});
	it('reads it back, and refuses an unknown kind or a relative path', () => {
		expect(diffChange(new URLSearchParams('diff=%2Fetc%2Fmotd&change=deleted'))).toEqual(change('deleted'));
		expect(diffChange(new URLSearchParams('diff=%2Fetc%2Fmotd&change=renamed'))).toBeNull();
		expect(diffChange(new URLSearchParams('diff=%2Fetc%2Fmotd'))).toBeNull();
		expect(diffChange(new URLSearchParams('diff=etc%2Fmotd&change=added'))).toBeNull();
		expect(diffChange(new URLSearchParams('change=added'))).toBeNull();
	});
});

describe('line numbers of each side in the unified view', () => {
	const T = (s: string) => Text.of(s.split('\n'));
	const numbers = (a: string, b: string) => {
		const A = T(a);
		const B = T(b);
		const lc = lineChunks(Chunk.build(A, B), A, B);
		const rows: string[] = [];
		for (let l = 1; l <= B.lines + 1; l++) {
			for (const n of deletedAt(lc, l)) rows.push(`${n} -`);
			if (l <= B.lines) rows.push(`${aLineOf(lc, l) ?? '+'} ${l}`);
		}
		return rows;
	};
	it('an added line: no old number, the next ones shifted', () => {
		expect(numbers('a\nb\nc', 'a\nX\nb\nc')).toEqual(['1 1', '+ 2', '2 3', '3 4']);
	});
	it('a removed line: its old number alone, above the next line', () => {
		expect(numbers('x\na\nb', 'a\nb')).toEqual(['1 -', '2 1', '3 2']);
	});
	it('a changed line: the old one, then the new one', () => {
		expect(numbers('a\nb\nc', 'a\nX\nc')).toEqual(['1 1', '2 -', '+ 2', '3 3']);
	});
	it('several chunks keep their offsets', () => {
		expect(numbers('1\n2\n3\n4\n5\n6', '1\nN\n2\n3\n5\n6\nM')).toEqual([
			'1 1', '+ 2', '2 3', '3 4', '4 -', '5 5', '6 -', '+ 6', '+ 7'
		]);
	});
	it('from an empty file', () => {
		expect(numbers('', 'a\nb')).toEqual(['1 -', '+ 1', '+ 2']);
	});
});

describe('the unified view of two large files (review of tranche 7)', () => {
	// About 224 KiB each, nearly every line different: without the scan
	// limit of the view, the diff took 54 s and found other chunks than the
	// ones shown.
	const big = (seed: number) => {
		let x = seed;
		const word = () => ((x = (x * 1103515245 + 12345) % 2147483648), (x % 100000).toString(36));
		return Array.from({ length: 8500 }, () => Array.from({ length: 6 }, word).join(' ')).join('\n') + '\n';
	};
	// Bounded (1.5 s, the view built), and the numbers are those of the
	// chunks the view shows: every line of each side numbered once, in order.
	const bounded = async (original: string, modified: string) => {
		await diffState({ kind: 'added', text: 'warm-up' }, 'diff', true); // modules loaded before timing
		const t0 = performance.now();
		const state = await diffState({ kind: 'modified', original, modified, equal: null }, 'diff', true);
		const shown = merge.getChunks(state)!.chunks;
		const nums = numbering(merge, state);
		expect(performance.now() - t0).toBeLessThan(1500);
		expect(nums).toEqual(lineChunks(shown, merge.getOriginalDoc(state), state.doc));
		// Walked in order: unchanged lines, then each chunk's removed lines
		// (deletedAt) and added ones (no old number). aLineOf is checked on
		// lines spread over the file.
		const b = state.doc.lines;
		const step = Math.max(1, Math.floor(b / 1000));
		let a = 1;
		let l = 1;
		const unchanged = (to: number) => {
			for (; l < to; l++, a++) if (l % step === 0) expect(aLineOf(nums, l)).toBe(a);
		};
		for (const c of nums) {
			expect(c.bFrom).toBeGreaterThanOrEqual(l);
			unchanged(c.bFrom);
			if (c.aCount > 0) {
				expect(deletedAt(nums, c.bFrom)[0]).toBe(a);
				a += c.aCount;
			}
			if (c.bCount > 0) expect(aLineOf(nums, c.bFrom)).toBeNull();
			l += c.bCount;
		}
		unchanged(b + 1);
		expect(a - 1).toBe(merge.getOriginalDoc(state).lines);
		return shown;
	};
	it('is bounded in time, and numbers the chunks the view shows', async () => {
		const original = big(1);
		expect(original.length).toBeGreaterThan(200_000);
		expect((await bounded(original, big(2))).length).toBeGreaterThan(0);
	});
	// Second review: findMatch of @codemirror/merge is bounded by neither
	// scanLimit nor timeout. A repetitive line of 300 KB moved held the tab
	// 12 to 20 s.
	it('a very long line moved', async () => {
		const m = 'a'.repeat(300_000);
		await bounded(`x\n${m}\ny\n`, `y\n${m}\nx\n`);
	});
	it('a repetitive block moved, short lines', async () => {
		const m = 'ab\n'.repeat(100_000);
		await bounded(`x\n${m}y\n`, `y\n${m}x\n`);
	});
	it('many identical lines', async () => {
		await bounded('x\n'.repeat(500_000), 'x\n'.repeat(250_000) + 'y\n' + 'x\n'.repeat(250_000));
		await bounded('x\n'.repeat(500_000), 'y\n'.repeat(500_000));
	});
	it('two files of 100 000 lines with nothing in common', async () => {
		const lines = (p: string) => Array.from({ length: 100_000 }, (_, i) => `${p}${i}`).join('\n');
		await bounded(lines('a'), lines('b'));
	});
	it('a single line of 2 MiB', async () => {
		const m = 'ab'.repeat(1 << 20);
		await bounded(m, m.slice(0, 1 << 20) + 'Z' + m.slice((1 << 20) + 1));
		await bounded(m, 'ba'.repeat(1 << 20));
	});
	it('line endings in CR alone', async () => {
		const ls = Array.from({ length: 50_000 }, (_, i) => `line ${i % 7}`);
		await bounded(ls.join('\r'), [...ls].reverse().join('\r'));
	});
	it('many small blocks, each costly to refine', async () => {
		// Each changed line is small enough to be refined to the characters,
		// and costly to refine (a repetitive run whose ends moved): the
		// total refined is what bounds them.
		const a: string[] = [];
		const b: string[] = [];
		for (let i = 0; i < 500; i++) {
			const run = 'a'.repeat(3990);
			a.push(`k${i}`, `x${run}y${i}`);
			b.push(`k${i}`, `y${i}${run}x`);
		}
		await bounded(a.join('\n'), b.join('\n'));
	});
	it('long runs of the same line, edits at both ends and inside', async () => {
		// Every diagonal of the line diff slides along the run: the work is
		// lines × edits, which the budget bounds.
		await bounded(`a\n${'x\n'.repeat(999_000)}a\n`, `b\n${'x\n'.repeat(500_000)}${'c\n'.repeat(1500)}${'x\n'.repeat(497_500)}b\n`);
	});
});

describe('lineDiff: the bounded diff behind the view', () => {
	// The spans, applied to a, give b; between them a and b are equal.
	const check = (a: string, b: string, spans: Span[]) => {
		let ia = 0;
		let ib = 0;
		let out = '';
		for (const s of spans) {
			expect(s.fromA).toBeGreaterThanOrEqual(ia);
			expect(s.fromA - ia).toBe(s.fromB - ib);
			expect(a.slice(ia, s.fromA)).toBe(b.slice(ib, s.fromB));
			out += a.slice(ia, s.fromA) + b.slice(s.fromB, s.toB);
			ia = s.toA;
			ib = s.toB;
		}
		expect(a.slice(ia)).toBe(b.slice(ib));
		expect(out + a.slice(ia)).toBe(b);
	};
	const cases: [string, string][] = [
		['', ''], ['', 'a'], ['a', ''], ['x', 'x\ny'], ['x\ny', 'x'], ['x\ny', 'x\nz'], ['a\nb\nc', 'a\nX\nc'],
		['x\na\nb', 'a\nb'], ['a\nb\n', 'a\nb'], ['a\nb', 'a\nb\n'], ['1\n2\n3\n4\n5\n6', '1\nN\n2\n3\n5\n6\nM'],
		['same\n', 'same\n'], ['\n\n\n', '\n']
	];
	for (const [a, b] of cases) it(`${JSON.stringify(a)} → ${JSON.stringify(b)}`, () => check(a, b, lineDiff(a, b)));
	it('random edits, with and without refinement', () => {
		let x = 3;
		const r = (n: number) => ((x = (x * 1103515245 + 12345) % 2147483648), x % n);
		for (let t = 0; t < 300; t++) {
			const a = Array.from({ length: r(12) }, () => 'abc'[r(3)].repeat(1 + r(3))).join('\n');
			const ls = a.split('\n');
			for (let e = r(4); e > 0; e--) ls.splice(r(ls.length + 1), r(2), 'abcz'[r(4)]);
			const b = ls.join('\n') + (r(3) ? '' : '\n');
			check(a, b, lineDiff(a, b));
			check(a, b, lineDiff(a, b, (p, q) => merge.diff(p, q)));
		}
	});
	it('bounds the work of the line diff: lines × edits, and edits² for its memory', () => {
		for (const n of [1, 10, 3000, 10_000, 100_000, 2_000_000]) {
			expect(editLimit(n) * n).toBeLessThanOrEqual(LINE_DIFF_BUDGET);
			expect(editLimit(n)).toBeLessThanOrEqual(MAX_EDITS);
		}
		expect(MAX_EDITS).toBeLessThanOrEqual(1000);
		expect(editLimit(2_000_000)).toBe(5);
	});
	it('past the edits allowed, one replaced block between the common ends', () => {
		const a = Array.from({ length: 3000 }, (_, i) => `a${i}`).join('\n');
		const b = Array.from({ length: 3000 }, (_, i) => `b${i}`).join('\n');
		expect(lineDiff(`same\n${a}\nend`, `same\n${b}\nend`)).toEqual([{ fromA: 5, toA: 5 + a.length + 1, fromB: 5, toB: 5 + b.length + 1 }]);		// 600 lines changed out of 1200: 1200 edits, past the 1000 allowed.
		const c = Array.from({ length: 1200 }, (_, i) => `l${i}`);
		const d = c.map((l, i) => (i % 2 ? l : `${l}!`));
		expect(lineDiff(c.join('\n'), d.join('\n'))).toHaveLength(1);
		expect(lineDiff(c.slice(0, 800).join('\n'), d.slice(0, 800).join('\n'))).toHaveLength(400);
	});
	it('only lines that changed', () => {
		expect(lineDiff('a\nb\nc\n', 'a\nB\nc\n')).toEqual([{ fromA: 2, toA: 4, fromB: 2, toB: 4 }]); // the line and its end
	});
	it('refines a small region to the characters, never a large one', () => {
		const calls: number[] = [];
		const refine = (p: string, q: string) => (calls.push(p.length + q.length), merge.diff(p, q));
		expect(lineDiff('a\nhello world\nc', 'a\nhello there\nc', refine).every((s) => s.toA - s.fromA < 11)).toBe(true);
		const big = 'w'.repeat(REFINE_MAX + 1);
		lineDiff(`${big}\n`, `${big}x\n`, refine);
		expect(Math.max(...calls)).toBeLessThanOrEqual(2 * REFINE_MAX);
	});
});
