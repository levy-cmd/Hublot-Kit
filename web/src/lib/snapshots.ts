// The Snapshots page (spec v0.2 §5.7): a photo of the files of a container,
// metadata and a keyed fingerprint of each file kept by the agent, never the
// content. Neither a fingerprint nor the key id ever reaches the page.
import { api, ApiError, type Figures } from './api.ts';
import { t, tf } from './i18n/index.ts';

export type SnapshotSource = 'procfs' | 'archive' | 'index';

/** One snapshot of GET /api/snapshots. */
export type SnapshotRow = {
	id: string;
	containerId: string;
	containerName: string;
	project?: string;
	image?: string;
	imageId?: string;
	createdAt: string;
	finishedAt: string;
	state: 'running' | 'stopped';
	source: SnapshotSource;
	entries: number;
	hashedBytes: number;
	partial: boolean;
	/** Absent when Docker did not answer. */
	containerGone?: boolean;
};

export type SnapshotJob = {
	kind: 'snapshot' | 'count';
	snapshot?: string;
	containerId?: string;
	containerName?: string;
	progress: number;
	entries: number;
};

export type SnapshotOutcome = { snapshot: string; containerId: string; result: string; detail?: string; at: string };

export type SnapshotList = { snapshots: SnapshotRow[]; current: SnapshotJob | null; last: SnapshotOutcome | null };

export const listSnapshots = (signal?: AbortSignal) => api<SnapshotList>('/snapshots', { signal });

/**
 * Starts a snapshot of the container (full ID); the agent answers 202 with
 * the snapshot's ID, which `api` turns into an ApiError "indexing": this
 * returns that ID. index: the photo of the metadata.
 */
export async function takeSnapshot(id: string, index: boolean): Promise<string> {
	const q = index ? '?source=index' : '';
	const res = await fetch(`/api/containers/${encodeURIComponent(id)}/snapshots${q}`, {
		method: 'POST',
		credentials: 'same-origin'
	}).catch(() => {
		throw new ApiError(0, 'network');
	});
	const body = await res.json().catch(() => null);
	if (res.status === 202 && typeof body?.snapshot === 'string') return body.snapshot;
	throw new ApiError(res.status, body?.error ?? 'unknown', body?.detail);
}

export const deleteSnapshot = (sid: string) => api<void>(`/snapshots/${encodeURIComponent(sid)}`, { method: 'DELETE' });

export type FilesCount = NonNullable<Figures['files']>;

/** The bounded count; throws ApiError "indexing" (with progress) while it runs. */
export const countFiles = (id: string, signal?: AbortSignal) =>
	api<FilesCount>(`/containers/${encodeURIComponent(id)}/count`, { signal });

export type SnapshotGroup = { containerId: string; name: string; gone: boolean; rows: SnapshotRow[] };

/**
 * The snapshots by container, the groups by name, the rows the most recent
 * first. A group keeps its container ID: two containers of the same name
 * (recreated by Compose) are two groups.
 */
export function groupSnapshots(rows: SnapshotRow[]): SnapshotGroup[] {
	const by = new Map<string, SnapshotGroup>();
	for (const r of rows) {
		let g = by.get(r.containerId);
		if (!g) {
			g = { containerId: r.containerId, name: r.containerName, gone: false, rows: [] };
			by.set(r.containerId, g);
		}
		g.gone ||= r.containerGone === true;
		g.rows.push(r);
	}
	const out = [...by.values()];
	for (const g of out) g.rows.sort((a, b) => b.createdAt.localeCompare(a.createdAt));
	return out.sort((a, b) => a.name.localeCompare(b.name) || a.containerId.localeCompare(b.containerId));
}

/** What a capture will cost: the text of the confirmation (spec v0.2 §0.2, Q57). */
export type SnapshotPlan = 'running' | 'stopped' | 'index';

/**
 * The plans offered for a container, the first one proposed first: the
 * photo of the metadata when the index is already built (Q57), then the
 * full photo.
 */
export function snapshotPlans(running: boolean, files: Figures['files'] | undefined): SnapshotPlan[] {
	if (running) return ['running'];
	return files?.source === 'index' ? ['index', 'stopped'] : ['stopped'];
}

export function planCost(plan: SnapshotPlan, sizeRootFs: number | null | undefined, fmt: (n: number) => string): string[] {
	const size = sizeRootFs != null ? tf('snapshots.cost.size', { size: fmt(sizeRootFs) }) : '';
	switch (plan) {
		case 'running':
			return [t('snapshots.cost.running'), size].filter(Boolean);
		case 'stopped':
			return [t('snapshots.cost.stopped'), t('snapshots.cost.memory'), size].filter(Boolean);
		case 'index':
			return [t('snapshots.cost.index')];
	}
}

export function sourceLabel(s: SnapshotSource): string {
	return t(`snapshots.source.${s}`);
}

/** The progress of a job, in whole percents. */
export function percent(p: number): number {
	return Math.max(0, Math.min(99, Math.floor(p * 100)));
}
