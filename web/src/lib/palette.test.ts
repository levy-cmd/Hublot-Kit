import { readFileSync } from 'node:fs';
import { afterEach, describe, expect, it, vi } from 'vitest';
import type { Container } from './api.ts';
import { containerList } from './containerList.svelte.ts';
import {
	fileItems, fullId, MAX_QUERY, rank, searchFiles, sections, validQuery, type FileSearch, type PaletteItem
} from './palette.ts';
import { paletteItems } from './palette/index.ts';

// Spec v0.2 §5.8: the global search (Ctrl+K).

const item = (kind: PaletteItem['kind'], label: string, detail?: string): PaletteItem => ({
	kind, label, detail, href: `/${kind}/${label}`
});

const ctr = (name: string, extra: Partial<Container> = {}): Container => ({
	id: `${name}0123456789abcdef`, name, image: 'nginx:1.27', state: 'running', status: 'Up 2 hours',
	created: '2026-10-08T09:00:00Z', ports: [], ...extra
});

afterEach(() => {
	vi.unstubAllGlobals();
	containerList.clear();
});

describe('rank', () => {
	it('puts an exact name before a substring, and a substring before a project', () => {
		const items = [
			item('project', 'shop'),
			item('container', 'shop-web'),
			item('container', 'shop'),
			item('page', 'Paramètres')
		];
		expect(rank(items, 'SHOP').map((i) => `${i.kind}:${i.label}`)).toEqual([
			'container:shop',
			'container:shop-web',
			'project:shop'
		]);
	});

	it('finds a container by its detail (image, short ID, project, service)', () => {
		const items = [item('container', 'web', 'nginx:1.27 · shop/front'), item('container', 'db', 'postgres:17')];
		expect(rank(items, 'shop/front').map((i) => i.label)).toEqual(['web']);
		expect(rank(items, 'postgres').map((i) => i.label)).toEqual(['db']);
	});

	it('answers nothing to a query too short or too long', () => {
		const items = [item('container', 'ab')];
		expect(rank(items, 'a')).toEqual([]);
		expect(rank(items, 'ab'.repeat(101))).toEqual([]);
	});
});

describe('validQuery', () => {
	it('accepts 2 to 200 characters, as the agent does', () => {
		expect(validQuery('a')).toBe(false);
		expect(validQuery(' a ')).toBe(false);
		expect(validQuery('ab')).toBe(true);
		expect(validQuery('éé')).toBe(true);
		expect(validQuery('x'.repeat(MAX_QUERY))).toBe(true);
		expect(validQuery('x'.repeat(MAX_QUERY + 1))).toBe(false);
		expect(validQuery('é'.repeat(MAX_QUERY))).toBe(true);
	});
});

describe('fileItems', () => {
	const none: FileSearch = { indexed: false, results: [], truncated: false };

	it('has no file entry without an index, only a way to the explorer', () => {
		const out = fileItems(none, 'abc', false);
		expect(out.some((i) => i.kind === 'file')).toBe(false);
		expect(out.map((i) => i.href)).toEqual(['/?c=abc&panel=files']);
		expect(fileItems(none, 'abc', true).map((i) => i.href)).toEqual(['/c/abc?tab=files']);
	});

	it('opens a file in the explorer of its container', () => {
		const res: FileSearch = {
			indexed: true,
			truncated: false,
			results: [
				{ path: '/etc/nginx/nginx.conf', type: 'file' },
				{ path: '/etc/nginx', type: 'dir' }
			]
		};
		expect(fileItems(res, 'abc', false).map((i) => [i.kind, i.label, i.href])).toEqual([
			['file', '/etc/nginx/nginx.conf', '/?c=abc&panel=files&path=%2Fetc%2Fnginx&file=%2Fetc%2Fnginx%2Fnginx.conf'],
			['file', '/etc/nginx', '/?c=abc&panel=files&path=%2Fetc%2Fnginx']
		]);
		expect(fileItems(res, 'abc', true)[0].href).toBe(
			'/c/abc?tab=files&path=%2Fetc%2Fnginx&file=%2Fetc%2Fnginx%2Fnginx.conf'
		);
	});
});

// Review of tranche 3 (Codex): the agent recognises a build in progress by
// the full ID only (no prefix, v0.1.1); the palette never sends anything else.
describe('fullId', () => {
	const full = 'ab'.repeat(32);
	const list = [ctr('web', { id: full }), ctr('db', { id: 'cd'.repeat(32) })];
	it('turns a name, a short or a full ID into the full ID of the list', () => {
		expect(fullId(list, full)).toBe(full);
		expect(fullId(list, 'web')).toBe(full);
		expect(fullId(list, full.slice(0, 12))).toBe(full);
	});
	it('gives nothing for an unknown, too short or ambiguous reference', () => {
		expect(fullId(list, 'ef'.repeat(6))).toBeNull();
		expect(fullId(list, 'abab')).toBeNull();
		expect(fullId([...list, ctr('x', { id: 'ab'.repeat(6) + 'ef'.repeat(26) })], full.slice(0, 12))).toBeNull();
		expect(fullId(null, full)).toBeNull();
		expect(fullId(list, null)).toBeNull();
	});
});

describe('sections', () => {
	it('groups by kind, in a fixed order', () => {
		const out = sections([item('page', 'p'), item('container', 'a'), item('project', 'x'), item('container', 'b')]);
		expect(out.map((s) => [s.kind, s.items.map((i) => i.label)])).toEqual([
			['container', ['a', 'b']],
			['project', ['x']],
			['page', ['p']]
		]);
	});
});

describe('paletteItems', () => {
	it('lists the containers, their Compose projects and the pages, never a value', () => {
		containerList.items = [
			ctr('web', { project: 'shop', service: 'front' }),
			ctr('db', { project: 'shop', service: 'db', image: 'postgres:17' }),
			ctr('solo')
		];
		const items = paletteItems();
		expect(items.filter((i) => i.kind === 'container').map((i) => i.label)).toEqual(['web', 'db', 'solo']);
		expect(items.filter((i) => i.kind === 'project').map((i) => [i.label, i.href])).toEqual([['shop', '/?project=shop']]);
		expect(items.find((i) => i.label === 'web')?.href).toBe('/?c=web0123456789abcdef');
		expect(items.some((i) => i.kind === 'page' && i.href === '/settings')).toBe(true);
		// Panel shortcuts and the search entry itself are not pages.
		expect(items.some((i) => i.href.includes('panel=') || i.href === '')).toBe(false);
		expect(rank(items, 'web0123').map((i) => i.label)).toEqual(['web']);
		expect(rank(items, 'front').map((i) => i.label)).toEqual(['web']);
		expect(rank(items, 'postgres').map((i) => i.label)).toEqual(['db']);
	});
});

// Spec v0.2 §5.8: nothing is kept, the query never goes into the page's URL.
describe('no trace of the query', () => {
	it('asks the agent once, and stores nothing', async () => {
		const store = { setItem: vi.fn(), getItem: vi.fn(() => null), removeItem: vi.fn() };
		vi.stubGlobal('localStorage', store);
		vi.stubGlobal('sessionStorage', store);
		const fetchMock = vi.fn(
			async () =>
				new Response(JSON.stringify({ indexed: true, results: [{ path: '/srv/backup.sql', type: 'file' }], truncated: false }))
		);
		vi.stubGlobal('fetch', fetchMock);
		containerList.items = [ctr('web')];
		const res = await searchFiles('web0123456789abcdef', 'backup sql', new AbortController().signal);
		rank(paletteItems(), 'backup sql');
		fileItems(res, 'web0123456789abcdef', false);
		expect(fetchMock).toHaveBeenCalledOnce();
		// Review of tranche 3 (Codex): the query goes in the body of a POST,
		// never in a URL a reverse proxy would log.
		const [url, init] = fetchMock.mock.calls[0] as unknown as [string, RequestInit];
		expect(url).toBe('/api/containers/web0123456789abcdef/fs/search');
		expect(url).not.toMatch(/backup|\?/);
		expect(init.method).toBe('POST');
		expect(JSON.parse(String(init.body))).toEqual({ q: 'backup sql' });
		expect(store.setItem).not.toHaveBeenCalled();
	});

	it('has no storage, history or URL write in the palette', () => {
		for (const f of ['./components/CommandPalette.svelte', './palette.ts', './palette/index.ts', './palette/containers.ts']) {
			const src = readFileSync(new URL(f, import.meta.url), 'utf8');
			expect(src, f).not.toMatch(/localStorage|sessionStorage|indexedDB|pushState|replaceState|document\.cookie/);
		}
	});
});
