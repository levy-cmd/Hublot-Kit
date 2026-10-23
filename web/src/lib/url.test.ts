import { describe, expect, it } from 'vitest';
import { withParams } from './url.ts';

describe('withParams', () => {
	it('changes some keys and keeps the others', () => {
		const q = new URLSearchParams('c=abc&panel=files&path=%2Fetc&file=%2Fetc%2Fhosts');
		expect(withParams(q, { path: '/root', file: null })).toBe('?c=abc&panel=files&path=%2Froot');
		expect(withParams(q, { panel: 'diff' })).toContain('panel=diff');
		expect(q.get('panel')).toBe('files'); // the input is not changed
	});
	it('gives "?" when nothing is left', () => {
		expect(withParams(new URLSearchParams('c=1'), { c: null })).toBe('?');
	});
});
