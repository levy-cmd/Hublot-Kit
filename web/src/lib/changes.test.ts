import { describe, expect, it } from 'vitest';
import type { Change } from './api';
import { changeMarks, countChanges, deletedIn, kindLetter, truncateNames } from './changes';

const cs: Change[] = [
	{ path: '/etc', kind: 'modified' },
	{ path: '/etc/motd', kind: 'deleted' },
	{ path: '/etc/new.conf', kind: 'added' },
	{ path: '/etc/ssl/old.pem', kind: 'deleted' },
	{ path: '/gone', kind: 'deleted' }
];

describe('changes', () => {
	it('marks each path with its kind', () => {
		const m = changeMarks(cs);
		expect(m.get('/etc/new.conf')).toBe('added');
		expect(m.get('/etc')).toBe('modified');
		expect(m.get('/usr')).toBeUndefined();
	});
	it('lists the deleted children of a folder, not its grandchildren', () => {
		expect(deletedIn(cs, '/etc')).toEqual(['motd']);
		expect(deletedIn(cs, '/')).toEqual(['gone']);
		expect(deletedIn(cs, '/etc/ssl')).toEqual(['old.pem']);
		expect(deletedIn(cs, '/e')).toEqual([]);
	});
	it('counts per kind', () => {
		expect(countChanges(cs)).toEqual({ added: 1, modified: 1, deleted: 3 });
	});
	it('gives a letter per kind', () => {
		expect(['added', 'modified', 'deleted'].map((k) => kindLetter(k as Change['kind']))).toEqual(['A', 'M', 'D']);
	});
	it('truncates a long list of names', () => {
		const names = Array.from({ length: 50 }, (_, i) => `f${i}`);
		const r = truncateNames(names);
		expect(r.shown).toHaveLength(20);
		expect(r.more).toBe(30);
		expect(truncateNames(['a'])).toEqual({ shown: ['a'], more: 0 });
	});
});
