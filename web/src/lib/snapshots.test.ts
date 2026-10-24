import { afterEach, describe, expect, it, vi } from 'vitest';
import { ApiError } from './api.ts';
import { t } from './i18n/index.ts';
import {
	CHANGE_KINDS,
	compareCandidates,
	compareSnapshots,
	contentPlan,
	fieldLabel,
	filterResults,
	findResult,
	groupSnapshots,
	imageMatch,
	kindCounts,
	KINDS,
	percent,
	planCost,
	snapshotPlans,
	takeSnapshot,
	type CompareResult,
	type Comparison,
	type SnapshotRow
} from './snapshots.ts';

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
		expect(planCost('running', 42, fmt)).toEqual([t('snapshots.cost.running'), expect.stringContaining('42 o')]);
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

	// Spec v0.2 §5.7: the directories whose mtime alone changed are hidden by default.
	it('filters by kind, the directories whose date alone changed on demand', () => {
		const rs: CompareResult[] = [
			{ path: '/etc', kind: 'meta', type: 'dir', fields: ['mtime'] },
			{ path: '/etc/motd', kind: 'modified', type: 'file', fields: [] },
			{ path: '/etc/new', kind: 'added', type: 'file', fields: [] },
			{ path: '/var', kind: 'meta', type: 'dir', fields: ['mode', 'mtime'] },
			{ path: '/x', kind: 'meta', type: 'file', fields: ['mtime'] }
		];
		expect(filterResults(rs, new Set(KINDS), false).map((r) => r.path)).toEqual(['/etc/motd', '/etc/new', '/var', '/x']);
		expect(filterResults(rs, new Set(KINDS), true)).toHaveLength(5);
		expect(filterResults(rs, new Set(['added']), true).map((r) => r.path)).toEqual(['/etc/new']);
		expect(kindCounts(rs, false)).toEqual({ added: 1, deleted: 0, modified: 1, meta: 2, type: 0, unknown: 0 });
		expect(kindCounts(rs, true).meta).toBe(3);
	});

	// Q59: no content is kept; against the current container, its content,
	// and for a modified file the diff when image-match proves it.
	it('shows a content only against the current container', () => {
		const r = (kind: CompareResult['kind'], type = 'file'): CompareResult => ({ path: '/f', kind, type, fields: [] });
		for (const k of KINDS) expect(contentPlan(r(k), false)).toBe('none');
		expect(contentPlan(r('modified'), true)).toBe('match');
		expect(contentPlan(r('added'), true)).toBe('current');
		expect(contentPlan(r('unknown'), true)).toBe('current');
		expect(contentPlan(r('meta'), true)).toBe('current');
		expect(contentPlan(r('deleted'), true)).toBe('none');
		expect(contentPlan(r('modified', 'dir'), true)).toBe('none');
		expect(contentPlan(r('type', 'symlink'), true)).toBe('none');
	});

	it('offers to compare with the same container, then the same name', () => {
		const a = row('a', 'c1', 'web', '2026-10-02T10:00:00Z');
		const rows = [
			a,
			row('older', 'c1', 'web', '2026-10-01T10:00:00Z'),
			row('newer', 'c1', 'web', '2026-10-03T10:00:00Z'),
			row('recreated', 'c2', 'web', '2026-10-04T10:00:00Z'),
			row('other', 'c3', 'db', '2026-10-05T10:00:00Z')
		];
		expect(compareCandidates(rows, a).map((r) => r.id)).toEqual(['newer', 'older', 'recreated']);
	});

	it('asks the comparison and the match by the snapshot ID, the path encoded', async () => {
		const sid = '0b7d5c1e-8f3a-4c2b-9d1e-123456789abc';
		const fetch = vi.fn(async () => new Response(JSON.stringify({ match: true }), { status: 200 }));
		vi.stubGlobal('fetch', fetch);
		await expect(imageMatch(sid, '/etc/a b&c')).resolves.toBe(true);
		expect(fetch).toHaveBeenCalledWith(`/api/snapshots/${sid}/image-match?path=%2Fetc%2Fa%20b%26c`, expect.anything());
		// Anything but a true boolean is false.
		vi.stubGlobal('fetch', vi.fn(async () => new Response(JSON.stringify({ match: 'yes' }), { status: 200 })));
		await expect(imageMatch(sid, '/f')).resolves.toBe(false);
		vi.stubGlobal('fetch', vi.fn(async () => new Response(JSON.stringify({ error: 'indexing', progress: 0.5 }), { status: 202 })));
		await expect(compareSnapshots(sid, 'current')).rejects.toMatchObject({ code: 'indexing' });
	});

	it('names every kind and field (the i18n test checks both languages)', () => {
		for (const k of KINDS) expect(t(`snapshots.kind.${k}`)).not.toBe(`snapshots.kind.${k}`);
		for (const f of ['size', 'mode', 'uid', 'gid', 'mtime', 'link']) {
			expect(t(`snapshots.field.${f}`)).not.toBe(`snapshots.field.${f}`);
			expect(fieldLabel(f)).toBe(t(`snapshots.field.${f}`));
		}
		expect(fieldLabel('later')).toBe('later');
	});

	// Review of tranche 9: the unknown come apart, never among the filters.
	it('finds a result among the changes or the unknown', () => {
		expect(CHANGE_KINDS).not.toContain('unknown');
		expect(CHANGE_KINDS).toHaveLength(KINDS.length - 1);
		const side = { id: 'x' } as Comparison['a'];
		const c: Comparison = {
			a: side,
			b: side,
			contentCompared: false,
			truncated: false,
			results: [{ path: '/added', kind: 'added', type: 'file', fields: [] }],
			unknown: [{ path: '/same', kind: 'unknown', type: 'file', fields: [] }],
			unknownCount: 12_345
		};
		expect(findResult(c, '/added')?.kind).toBe('added');
		expect(findResult(c, '/same')?.kind).toBe('unknown');
		expect(findResult(c, '/none')).toBeNull();
	});

	it('shows whole percents below 100', () => {
		expect(percent(0)).toBe(0);
		expect(percent(0.456)).toBe(45);
		expect(percent(0.999)).toBe(99);
		expect(percent(2)).toBe(99);
		expect(percent(-1)).toBe(0);
	});
});
