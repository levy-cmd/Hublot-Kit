import { afterEach, describe, expect, it, vi } from 'vitest';
import { ApiError } from './api.ts';
import { t } from './i18n/index.ts';
import { groupSnapshots, percent, planCost, snapshotPlans, takeSnapshot, type SnapshotRow } from './snapshots.ts';

const row = (id: string, containerId: string, name: string, createdAt: string, gone?: boolean): SnapshotRow => ({
	id,
	containerId,
	containerName: name,
	createdAt,
	finishedAt: createdAt,
	state: 'running',
	source: 'procfs',
	entries: 3,
	hashedBytes: 10,
	partial: false,
	containerGone: gone
});

describe('snapshots', () => {
	afterEach(() => vi.unstubAllGlobals());

	it('groups by container, the most recent first', () => {
		const g = groupSnapshots([
			row('1', 'b', 'web', '2026-10-01T10:00:00Z'),
			row('2', 'a', 'db', '2026-10-02T10:00:00Z', true),
			row('3', 'b', 'web', '2026-10-03T10:00:00Z'),
			// Recreated by Compose: same name, another ID, another group.
			row('4', 'c', 'web', '2026-10-04T10:00:00Z', false)
		]);
		expect(g.map((x) => [x.name, x.containerId])).toEqual([['db', 'a'], ['web', 'b'], ['web', 'c']]);
		expect(g[1].rows.map((r) => r.id)).toEqual(['3', '1']);
		expect(g[0].gone).toBe(true);
		expect(g[1].gone).toBe(false);
	});

	// Q57: the photo of the metadata first, when the index is built.
	it('offers the photo of the metadata first when possible', () => {
		const at = '2026-10-01T10:00:00Z';
		expect(snapshotPlans(true, { count: 3, source: 'snapshot', at })).toEqual(['running']);
		expect(snapshotPlans(false, null)).toEqual(['stopped']);
		expect(snapshotPlans(false, { count: 3, source: 'count', at })).toEqual(['stopped']);
		expect(snapshotPlans(false, { count: 3, source: 'index', at })).toEqual(['index', 'stopped']);
	});

	// Spec v0.2 §0.2: the confirmation says the cost.
	it('says the cost of each plan', () => {
		const fmt = (n: number) => `${n} o`;
		expect(planCost('running', 42, fmt)).toEqual([t('snapshots.cost.running'), t('snapshots.cost.memory'), expect.stringContaining('42 o')]);
		expect(planCost('stopped', null, fmt)).toEqual([t('snapshots.cost.stopped'), t('snapshots.cost.memory')]);
		expect(planCost('index', 42, fmt)).toEqual([t('snapshots.cost.index')]);
		for (const k of ['snapshots.cost.running', 'snapshots.cost.stopped', 'snapshots.cost.index']) expect(t(k)).not.toBe(k);
	});

	it('returns the ID of the 202 and posts to the full ID', async () => {
		const id = 'a'.repeat(64);
		const sid = '0b7d5c1e-8f3a-4c2b-9d1e-123456789abc';
		const fetch = vi.fn(async () => new Response(JSON.stringify({ error: 'indexing', progress: 0, snapshot: sid }), { status: 202 }));
		vi.stubGlobal('fetch', fetch);
		await expect(takeSnapshot(id, true)).resolves.toBe(sid);
		expect(fetch).toHaveBeenCalledWith(`/api/containers/${id}/snapshots?source=index`, expect.objectContaining({ method: 'POST' }));
	});

	it('throws the code of a refusal', async () => {
		vi.stubGlobal('fetch', vi.fn(async () => new Response(JSON.stringify({ error: 'too_large', detail: '20' }), { status: 413 })));
		await expect(takeSnapshot('x', false)).rejects.toEqual(new ApiError(413, 'too_large', '20'));
		vi.stubGlobal('fetch', vi.fn(async () => new Response(JSON.stringify({ error: 'conflict' }), { status: 409 })));
		await expect(takeSnapshot('x', false)).rejects.toMatchObject({ code: 'conflict' });
	});

	it('shows whole percents below 100', () => {
		expect(percent(0)).toBe(0);
		expect(percent(0.456)).toBe(45);
		expect(percent(0.999)).toBe(99);
		expect(percent(2)).toBe(99);
		expect(percent(-1)).toBe(0);
	});
});
