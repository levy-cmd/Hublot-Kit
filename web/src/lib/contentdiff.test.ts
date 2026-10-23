import { describe, expect, it } from 'vitest';
import { Chunk } from '@codemirror/merge';
import { Text } from '@codemirror/state';
import { ApiError, type Change, type Preview } from './api.ts';
import {
	aLineOf,
	deletedAt,
	diffChange,
	diffHref,
	lineChunks,
	needs,
	reasonMessage,
	sideOf,
	sides,
	type Side
} from './contentdiff.ts';
import { t } from './i18n/index.ts';

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
