import { readFileSync } from 'node:fs';
import { describe, expect, it } from 'vitest';
import { NO_PROJECT } from './containers.ts';
import {
	addInvReveal, argText, askInv, dropInvContainer, filterRows, forgetInv, invRevealed, noInvReveals, projectsOfRows,
	removeInvReveal, secretCount, sortRows, uniqueNames, type SecretRow
} from './inventory.ts';

// Spec v0.2 §5.9: the Secrets page, names only, sorted by number of secrets,
// filtered by project; a revealed value lives in the page's state only.

const row = (name: string, o: Partial<SecretRow> = {}): SecretRow => ({
	id: `${name}-id`,
	name,
	project: '',
	service: '',
	state: 'running',
	env: [],
	args: [],
	labels: [],
	unavailable: false,
	...o
});

describe('inventory', () => {
	it('counts the variables and arguments masked as secrets, not the masked labels', () => {
		expect(secretCount(row('a', { env: ['A', 'B'], args: [{ part: 'cmd', index: 1, kind: 'flag', name: '--password' }], labels: ['traefik.x'] }))).toBe(3);
		expect(secretCount(row('b', { labels: ['x', 'y'] }))).toBe(0);
	});

	it('sorts by number of secrets, then masked labels, then name', () => {
		const rows = [
			row('zeta', { labels: ['x'] }),
			row('beta', { env: ['A'] }),
			row('alpha'),
			row('gamma', { env: ['A', 'B'] }),
			row('delta', { args: [{ part: 'cmd', index: 1, kind: 'flag', name: '--password' }] }),
			row('omega', { unavailable: true })
		];
		expect(sortRows(rows).map((r) => r.name)).toEqual(['gamma', 'beta', 'delta', 'zeta', 'alpha', 'omega']);
		expect(rows[0].name).toBe('zeta'); // a copy, the list is not changed
	});

	it('puts an unavailable container after those it could inspect', () => {
		const rows = [row('b', { unavailable: true }), row('a', { env: ['X'] }), row('c')];
		expect(sortRows(rows).map((r) => r.name)).toEqual(['a', 'c', 'b']);
	});

	it('filters by Compose project, as the containers list', () => {
		const rows = [row('a', { project: 'shop' }), row('b'), row('c', { project: 'blog' })];
		expect(projectsOfRows(rows)).toEqual(['blog', 'shop']);
		expect(filterRows(rows, '').map((r) => r.name)).toEqual(['a', 'b', 'c']);
		expect(filterRows(rows, 'shop').map((r) => r.name)).toEqual(['a']);
		expect(filterRows(rows, NO_PROJECT).map((r) => r.name)).toEqual(['b']);
	});

	// Review of tranche 4 (relecteur): Docker may keep two variables of the
	// same name; a keyed {#each} would break on them.
	it('lists a variable name once', () => {
		expect(uniqueNames(['API_TOKEN', 'DB_PASSWORD', 'API_TOKEN'])).toEqual(['API_TOKEN', 'DB_PASSWORD']);
	});

	// Review of tranche 4 (Codex): an argument is its position and the name
	// detected, never its text.
	it('names a masked argument by its position and what was detected', () => {
		const flag = argText({ part: 'cmd', index: 2, kind: 'flag', name: '--requirepass' });
		expect(flag).toContain('--requirepass');
		expect(flag).toContain('3');
		expect(argText({ part: 'entrypoint', index: 0, kind: 'assignment', name: 'PGPASSWORD' })).toContain('PGPASSWORD=');
		expect(argText({ part: 'cmd', index: 2, kind: 'shell', name: 'sh -c' })).toContain('sh -c');
		const unknown = argText({ part: 'cmd', index: 1, kind: 'future', name: 'x' });
		expect(unknown).toBe(argText({ part: 'cmd', index: 1, kind: 'other' }));
	});

	it('keeps a revealed value per container and per variable', () => {
		let r = noInvReveals;
		r = addInvReveal(r, askInv(r, 'c1'), 'DB_PASSWORD', 'v1');
		r = addInvReveal(r, askInv(r, 'c2'), 'DB_PASSWORD', 'v2');
		expect(invRevealed(r, 'c1', 'DB_PASSWORD')).toBe('v1');
		expect(invRevealed(r, 'c2', 'DB_PASSWORD')).toBe('v2');
		expect(invRevealed(r, 'c1', 'OTHER')).toBeNull();
		r = removeInvReveal(r, 'c1', 'DB_PASSWORD');
		expect(invRevealed(r, 'c1', 'DB_PASSWORD')).toBeNull();
		expect(invRevealed(r, 'c2', 'DB_PASSWORD')).toBe('v2');
	});

	it('does not take a name of Object.prototype for a revealed value', () => {
		const r = addInvReveal(noInvReveals, askInv(noInvReveals, 'c1'), '__proto__', 'v');
		expect(invRevealed(r, 'c1', '__proto__')).toBe('v');
		expect(invRevealed(r, 'c1', 'constructor')).toBeNull();
		expect(invRevealed(r, 'constructor', 'toString')).toBeNull();
	});

	it('forgets the values of a folded container only', () => {
		let r = addInvReveal(noInvReveals, askInv(noInvReveals, 'c1'), 'A', 'v1');
		r = addInvReveal(r, askInv(r, 'c2'), 'A', 'v2');
		r = dropInvContainer(r, 'c1');
		expect(invRevealed(r, 'c1', 'A')).toBeNull();
		expect(invRevealed(r, 'c2', 'A')).toBe('v2');
	});

	it('forgets every value, and drops a reveal asked before (pagehide, leaving the page)', () => {
		let r = addInvReveal(noInvReveals, askInv(noInvReveals, 'c1'), 'A', 'v');
		const pending = askInv(r, 'c2');
		r = forgetInv(r);
		expect(invRevealed(r, 'c1', 'A')).toBeNull();
		r = addInvReveal(r, pending, 'B', 'late');
		expect(invRevealed(r, 'c2', 'B')).toBeNull();
	});

	// Spec v0.2 §5.9, §7: nothing of the page goes to a browser store.
	it('never writes a browser store', () => {
		for (const f of ['./inventory.ts', '../routes/secrets/+page.svelte']) {
			const src = readFileSync(new URL(f, import.meta.url), 'utf8');
			expect(src).not.toMatch(/localStorage|sessionStorage|indexedDB|caches\.|document\.cookie|console\./);
		}
	});
});
