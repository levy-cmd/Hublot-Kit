import { afterEach, describe, expect, it, vi } from 'vitest';
import { ApiError } from './api.ts';
import {
	containersText, filterImages, getImage, imageHex, imageHref, imageName, imageShortId, layerCounts, platform,
	sortImages, type ImageRow
} from './images.ts';

const hex = (c: string) => c.repeat(64);
const row = (id: string, tags: string[], size: number, created: string, names: string[] = []): ImageRow => ({
	id: `sha256:${id}`,
	tags,
	size,
	created,
	containers: names.map((n, i) => ({ id: `${i}`, name: n, state: 'running' }))
});

// Spec v0.2 §5.4: an image is named by its full ID only.
describe('images', () => {
	afterEach(() => vi.unstubAllGlobals());

	it('accepts a full ID only', () => {
		expect(imageHex(hex('a'))).toBe(hex('a'));
		expect(imageHex(`sha256:${hex('a')}`)).toBe(hex('a'));
		for (const bad of ['nginx:latest', 'nginx', hex('a').slice(1), hex('a').slice(0, 12), `${hex('a')}0`, hex('A'), `SHA256:${hex('a')}`, `sha256:sha256:${hex('a')}`, hex('g'), '']) {
			expect(imageHex(bad), bad).toBeNull();
		}
	});

	it('never sends a name or a short ID to the agent', async () => {
		const fetch = vi.fn(async () => new Response('{}', { status: 200 }));
		vi.stubGlobal('fetch', fetch);
		await expect(getImage('nginx:latest')).rejects.toEqual(new ApiError(404, 'not_found'));
		await expect(getImage(hex('a').slice(0, 12))).rejects.toBeInstanceOf(ApiError);
		expect(fetch).not.toHaveBeenCalled();
		await getImage(`sha256:${hex('b')}`);
		expect(fetch).toHaveBeenCalledTimes(1);
		expect(String((fetch.mock.calls[0] as unknown[])[0])).toBe(`/api/images/${hex('b')}`);
	});

	it('links by the full ID, names by tag or short ID', () => {
		expect(imageHref(`sha256:${hex('c')}`)).toBe(`/images/${hex('c')}`);
		expect(imageShortId(`sha256:${hex('c')}`)).toBe('cccccccccccc');
		expect(imageName({ id: `sha256:${hex('c')}`, tags: ['app:1', 'app:latest'] })).toBe('app:1');
		// An untagged image stays named, by its short ID (Review Focus 5).
		expect(imageName({ id: `sha256:${hex('c')}`, tags: [] })).toBe('cccccccccccc');
	});

	it('sorts by date, size or name, then by ID', () => {
		const rows = [
			row(hex('1'), ['b:1'], 10, '2026-10-01T00:00:00Z'),
			row(hex('2'), [], 30, '2026-10-03T00:00:00Z'),
			row(hex('3'), ['a:1'], 30, '2026-10-02T00:00:00Z')
		];
		const ids = (rs: ImageRow[]) => rs.map((r) => r.id.slice(7, 8));
		expect(ids(sortImages(rows, 'created'))).toEqual(['2', '3', '1']);
		expect(ids(sortImages(rows, 'size'))).toEqual(['2', '3', '1']);
		expect(ids(sortImages(rows, 'name'))).toEqual(['2', '3', '1']); // "222222222222" (short ID), "a:1", "b:1"
		expect(ids(rows)).toEqual(['1', '2', '3']); // a copy
	});

	it('filters by tag, short ID prefix or container name', () => {
		const rows = [row(hex('a'), ['shop/app:1'], 1, ''), row(hex('b'), [], 1, '', ['redis-cache'])];
		expect(filterImages(rows, ' SHOP ').map((r) => r.id)).toEqual([rows[0].id]);
		expect(filterImages(rows, 'bbbb').map((r) => r.id)).toEqual([rows[1].id]);
		expect(filterImages(rows, 'cache').map((r) => r.id)).toEqual([rows[1].id]);
		expect(filterImages(rows, '')).toHaveLength(2);
		expect(filterImages(rows, 'zzz')).toHaveLength(0);
	});

	it('counts the real layers apart from the empty entries', () => {
		const l = (empty: boolean) => ({ createdBy: '', created: '', size: empty ? 0 : 5, empty });
		expect(layerCounts([l(true), l(false), l(true), l(false), l(false)])).toEqual({ real: 3, empty: 2 });
		expect(layerCounts([])).toEqual({ real: 0, empty: 0 });
	});

	it('names the platform and the containers', () => {
		expect(platform({ os: 'linux', architecture: 'arm64', variant: 'v8' })).toBe('linux/arm64/v8');
		expect(platform({ os: 'linux', architecture: 'amd64', variant: '' })).toBe('linux/amd64');
		expect(containersText(0)).not.toContain('{n}');
		expect(containersText(3)).toContain('3');
	});
});
