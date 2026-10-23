import { readdirSync, readFileSync, statSync } from 'node:fs';
import { join } from 'node:path';
import { describe, expect, it } from 'vitest';
import { API_ERROR_CODES, detectLang, dicts, errorMessage, modules } from './index';

// The dictionaries are split by module (plan v0.2, tranche 1): <module>.fr.ts
// and <module>.en.ts, loaded by import.meta.glob. A later tranche adds its own
// pair without touching the others.
describe('i18n', () => {
	it('every module has both languages, with exactly the same keys', () => {
		expect(Object.keys(modules.fr).sort()).toEqual(Object.keys(modules.en).sort());
		for (const m of Object.keys(modules.fr)) {
			expect(Object.keys(modules.en[m]).sort(), m).toEqual(Object.keys(modules.fr[m]).sort());
		}
	});

	it('no key is defined by two modules', () => {
		const seen = new Map<string, string>();
		for (const [m, dict] of Object.entries(modules.fr)) {
			for (const k of Object.keys(dict)) {
				expect(seen.get(k), `${k} in ${m}`).toBeUndefined();
				seen.set(k, m);
			}
		}
	});

	it('every API error code of the spec has a translation', () => {
		for (const code of API_ERROR_CODES) {
			expect(dicts.fr).toHaveProperty(`error.${code}`);
			expect(dicts.en).toHaveProperty(`error.${code}`);
		}
	});

	it('knows the codes reserved for later tranches (v0.2 §6)', () => {
		expect(API_ERROR_CODES).toContain('invalid_query');
		expect(API_ERROR_CODES).toContain('unsupported_image');
	});

	it('knows the code of an unreadable log driver (F6)', () => {
		expect(API_ERROR_CODES).toContain('logs_unavailable');
	});

	it('unknown codes fall back to a generic message', () => {
		expect(errorMessage('nope')).toBe(errorMessage('unknown'));
	});

	it('detects the language', () => {
		expect(detectLang('fr-FR')).toBe('fr');
		expect(detectLang('en-US')).toBe('en');
		expect(detectLang(undefined)).toBe('en');
	});

	// Keys are plain strings since the split: this test stands for the type check.
	it('every literal key used by the code exists', () => {
		const src = new URL('../..', import.meta.url).pathname;
		const files: string[] = [];
		const walk = (d: string) => {
			for (const n of readdirSync(d)) {
				const p = join(d, n);
				if (statSync(p).isDirectory()) walk(p);
				else if (/\.(svelte|ts)$/.test(n) && !n.endsWith('.test.ts')) files.push(p);
			}
		};
		walk(src);
		const used = new Set<string>();
		for (const f of files) {
			for (const m of readFileSync(f, 'utf8').matchAll(/\bt\('([a-zA-Z0-9_.-]+)'\)/g)) used.add(m[1]);
			for (const m of readFileSync(f, 'utf8').matchAll(/\blabelKey: '([a-zA-Z0-9_.-]+)'/g)) used.add(m[1]);
		}
		expect(used.size).toBeGreaterThan(100);
		for (const k of used) expect(dicts.fr, k).toHaveProperty([k]);
	});
});
