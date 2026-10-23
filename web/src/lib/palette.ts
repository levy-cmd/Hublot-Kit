// The global search (spec v0.2 §5.8): containers, Compose projects and pages
// are searched here, in the lists the browser already has; files only in the
// index already built of the container open (Q61), through the agent.
// Nothing is kept: no history, no storage, never the query in the page's URL.
import { api, type Container, type EntryType } from './api.ts';
import { t } from './i18n/index.ts';

export type PaletteKind = 'container' | 'image' | 'project' | 'page' | 'file';

export type PaletteItem = {
	kind: PaletteKind;
	label: string;
	detail?: string;
	href: string;
	/** Other names it answers to (short ID, image, project, service): names only, never a value. */
	terms?: string[];
};

/** The order of the sections of the palette. */
export const KINDS: PaletteKind[] = ['container', 'image', 'project', 'page', 'file'];

// Same bounds as the agent (fs_search.go): 2 to 200 characters.
export const MIN_QUERY = 2;
export const MAX_QUERY = 200;

/** True for a query of 2 to 200 characters, spaces around it ignored. */
export function validQuery(q: string): boolean {
	const n = [...q.trim()].length;
	return n >= MIN_QUERY && n <= MAX_QUERY;
}

// Quality of a match, lower is better: the label exactly, at its start,
// inside it, then another name of the item.
function quality(it: PaletteItem, q: string): number | null {
	const label = it.label.toLowerCase();
	if (label === q) return 0;
	if (label.startsWith(q)) return 1;
	if (label.includes(q)) return 2;
	if (it.detail?.toLowerCase().includes(q) || it.terms?.some((s) => s.toLowerCase().includes(q))) return 3;
	return null;
}

/**
 * The items that match q (case ignored), best first. A container or an
 * image always comes before a project, a page or a file; within that, an
 * exact name, then a prefix, a substring, another name; then by kind, then
 * alphabetically.
 */
export function rank(items: PaletteItem[], q: string): PaletteItem[] {
	if (!validQuery(q)) return [];
	const lq = q.trim().toLowerCase();
	const scored: { it: PaletteItem; s: number }[] = [];
	for (const it of items) {
		const m = quality(it, lq);
		if (m === null) continue;
		const primary = it.kind === 'container' || it.kind === 'image';
		scored.push({ it, s: (primary ? 0 : 4) + m });
	}
	return scored
		.sort((a, b) => a.s - b.s || KINDS.indexOf(a.it.kind) - KINDS.indexOf(b.it.kind) || a.it.label.localeCompare(b.it.label))
		.map((x) => x.it);
}

/** The items grouped by kind, in the order of KINDS, empty groups left out. */
export function sections(items: PaletteItem[]): { kind: PaletteKind; items: PaletteItem[] }[] {
	return KINDS.map((kind) => ({ kind, items: items.filter((i) => i.kind === kind) })).filter((s) => s.items.length > 0);
}

export type FileSearch = { indexed: boolean; results: { path: string; type: EntryType }[]; truncated: boolean };

/**
 * POST /api/containers/{id}/fs/search, never builds an index (Q61). The
 * query goes in the body, never in a URL that a reverse proxy would log
 * (review of tranche 3); id is the full ID (fullId).
 */
export const searchFiles = (id: string, q: string, signal: AbortSignal) =>
	api<FileSearch>(`/containers/${encodeURIComponent(id)}/fs/search`, {
		method: 'POST',
		body: JSON.stringify({ q: q.trim() }),
		signal
	});

/**
 * The full ID, in the list the agent gave, of ref: a full ID, a name, or a
 * unique prefix of 12 hex digits or more (a /c/<short ID> link). null
 * otherwise. The agent knows a build in progress by the full ID only (no
 * prefix, v0.1.1): with a short ID, a request during an export would wait
 * for Docker's lock (review of tranche 3).
 */
export function fullId(list: Container[] | null, ref: string | null): string | null {
	if (!list || !ref) return null;
	const exact = list.find((c) => c.id === ref || c.name === ref);
	if (exact) return exact.id;
	if (!/^[0-9a-f]{12,63}$/.test(ref)) return null;
	const found = list.filter((c) => c.id.startsWith(ref));
	return found.length === 1 ? found[0].id : null;
}

/** The explorer of container id, at dir and file if given, on the list or on the full page. */
function explorerHref(id: string, fullPage: boolean, dir?: string, file?: string): string {
	const q = new URLSearchParams(fullPage ? { tab: 'files' } : { c: id, panel: 'files' });
	if (dir) q.set('path', dir);
	if (file) q.set('file', file);
	return fullPage ? `/c/${encodeURIComponent(id)}?${q}` : `/?${q}`;
}

const parent = (p: string) => p.slice(0, p.lastIndexOf('/')) || '/';

/**
 * The palette entries of a file search in container id. Without an index
 * (container running, or never opened in the explorer), no file at all:
 * a way to the explorer instead.
 */
export function fileItems(res: FileSearch, id: string, fullPage: boolean): PaletteItem[] {
	if (!res.indexed) {
		return [{ kind: 'page', label: t('palette.openExplorer'), detail: t('palette.noIndex'), href: explorerHref(id, fullPage) }];
	}
	return res.results.map((r) => ({
		kind: 'file',
		label: r.path,
		href: r.type === 'dir' ? explorerHref(id, fullPage, r.path) : explorerHref(id, fullPage, parent(r.path), r.path)
	}));
}

/** Opens the palette from anywhere (the side bar entry "Search"). */
export const PALETTE_EVENT = 'hublot:palette';
export function openPalette(): void {
	window.dispatchEvent(new CustomEvent(PALETTE_EVENT));
}

/** Ctrl+K, or Cmd+K on macOS. */
export const isPaletteKey = (e: Pick<KeyboardEvent, 'key' | 'ctrlKey' | 'metaKey' | 'altKey' | 'shiftKey'>) =>
	e.key.toLowerCase() === 'k' && (e.ctrlKey || e.metaKey) && !e.altKey && !e.shiftKey;
