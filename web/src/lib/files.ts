import { api, ApiError, previewAt, type Entry, type EntryType, type Listing, type Preview } from './api.ts';
import { errorMessage, t } from './i18n/index.ts';
import type { FileSearch, PaletteItem } from './palette.ts';

// Container paths are handled as text here, for display and navigation only:
// the agent resolves them inside the container root.

/** Absolute, without ".", "..", empty or trailing components ("/" stays "/"). */
export function normalize(p: string): string {
	const out: string[] = [];
	for (const c of p.split('/')) {
		if (c === '' || c === '.') continue;
		if (c === '..') out.pop();
		else out.push(c);
	}
	return '/' + out.join('/');
}

export const joinPath = (dir: string, name: string) => normalize(`${dir}/${name}`);
export const parentPath = (p: string) => normalize(`${p}/..`);
export const baseName = (p: string) => normalize(p).split('/').pop() || '/';

/** A link's target as a path to open: absolute as is, relative from the link's directory. */
export function resolveLinkTarget(dir: string, target: string): string {
	return target.startsWith('/') ? normalize(target) : joinPath(dir, target);
}

export function breadcrumbs(p: string): { name: string; path: string }[] {
	const crumbs = [{ name: '/', path: '/' }];
	let acc = '';
	for (const c of normalize(p).split('/').filter(Boolean)) {
		acc += '/' + c;
		crumbs.push({ name: c, path: acc });
	}
	return crumbs;
}

/** Directories first, then by name (code-point order, like the agent). */
export function sortEntries(entries: Entry[]): Entry[] {
	return [...entries].sort(
		(a, b) => Number(b.type === 'dir') - Number(a.type === 'dir') || (a.name < b.name ? -1 : a.name > b.name ? 1 : 0)
	);
}

const TYPE_CHAR: Record<EntryType, string> = { dir: 'd', symlink: 'l', file: '-', other: '?' };

/** ls-style permissions, e.g. "drwxr-sr-x"; s/S and t/T for setuid, setgid, sticky. */
export function formatMode(type: EntryType, mode: number): string {
	const bit = (m: number, c: string) => (mode & m ? c : '-');
	const exec = (m: number, special: number, c: string) =>
		mode & special ? (mode & m ? c : c.toUpperCase()) : bit(m, 'x');
	return (
		TYPE_CHAR[type] +
		bit(0o400, 'r') + bit(0o200, 'w') + exec(0o100, 0o4000, 's') +
		bit(0o040, 'r') + bit(0o020, 'w') + exec(0o010, 0o2000, 's') +
		bit(0o004, 'r') + bit(0o002, 'w') + exec(0o001, 0o1000, 't')
	);
}

/** Size in binary units: "0 o", "1.5 Kio" (fr) / "1.5 KiB" (en). */
export function formatSize(n: number): string {
	const units = [t('files.unit.b'), t('files.unit.kib'), t('files.unit.mib'), t('files.unit.gib'), t('files.unit.tib')];
	let v = n;
	let i = 0;
	while (v >= 1024 && i < units.length - 1) {
		v /= 1024;
		i++;
	}
	return i === 0 ? `${v} ${units[0]}` : `${v.toFixed(1)} ${units[i]}`;
}

/**
 * Message for a refused directory download. forbidden_path there comes from the
 * mount check (spec §5.4): its detail names the mount, shown as is (escaped by
 * Svelte like any container content).
 */
export function downloadErrorMessage(code: string, detail?: string): string {
	if (code === 'link_in_path') return t('files.linkDirHint');
	if (code === 'forbidden_path') return detail ? `${t('files.archiveForbidden')} (${detail})` : t('files.archiveForbidden');
	return errorMessage(code);
}

/** Percent shown while a stopped container is indexed: never 100 before the end. */
export function indexingPercent(progress?: number): number {
	return Math.min(99, Math.max(0, Math.floor((progress ?? 0) * 100)));
}

/**
 * Message for a failed listing or preview. A stopped container whose mounts
 * expose the host is refused whole (spec §5.2 step 7): the detail names the
 * mount, shown as text (escaped by Svelte).
 */
export function fsErrorMessage(code: string, detail: string | undefined, stopped: boolean): string {
	if (code === 'forbidden_path') {
		if (!stopped) return t('files.inaccessible');
		return detail ? `${t('files.stoppedUnavailable')} (${detail})` : t('files.stoppedUnavailable');
	}
	if (code === 'too_large' && stopped) return t('files.stoppedTooLarge');
	return errorMessage(code);
}

/**
 * Delay between two requests of a file of an image being extracted (spec
 * v0.2 §5.5): always well under the 15 s after which the agent cancels an
 * extraction that no request asks for.
 */
export const pollDelay = 2000;

/**
 * Asks ask() again every delay ms while the agent answers 202 indexing,
 * telling on.indexing the progress; then on.result or on.error, once. The
 * returned function stops it (another file chosen, the panel closed): no
 * request after it, no callback.
 */
export function pollIndexing<T>(
	ask: () => Promise<T>,
	on: { indexing: (progress?: number) => void; result: (value: T) => void; error: (e: unknown) => void },
	delay = pollDelay
): () => void {
	let stopped = false;
	let timer: ReturnType<typeof setTimeout> | undefined;
	const run = () => {
		ask().then(
			(v) => {
				if (!stopped) on.result(v);
			},
			(e) => {
				if (stopped) return;
				if (e instanceof ApiError && e.code === 'indexing') {
					on.indexing(e.progress);
					timer = setTimeout(run, delay);
				} else on.error(e);
			}
		);
	};
	run();
	return () => {
		stopped = true;
		clearTimeout(timer);
	};
}

// The files of an image (spec v0.2 §5.5), read only: by its full ID, the 64
// hex characters (images.ts imageHex), never a name.
const imageFsPath = (hex: string, route: string, path: string) =>
	`/images/${encodeURIComponent(hex)}/fs${route}?path=${encodeURIComponent(path)}`;

export const listImageDir = (hex: string, path: string) => api<Listing>(imageFsPath(hex, '', path));
export const previewImageFile = (hex: string, path: string): Promise<Preview> =>
	previewAt(`/api${imageFsPath(hex, '/file', path)}`);

/** POST /api/images/{id}/fs/search: the index already built only, the query in the body. */
export const searchImageFiles = (hex: string, q: string, signal: AbortSignal) =>
	api<FileSearch>(`/images/${encodeURIComponent(hex)}/fs/search`, {
		method: 'POST',
		body: JSON.stringify({ q: q.trim() }),
		signal
	});

/** The Files tab of image hex, at dir and file if given. */
export function imageExplorerHref(hex: string, dir?: string, file?: string): string {
	const q = new URLSearchParams({ tab: 'files' });
	if (dir) q.set('path', dir);
	if (file) q.set('file', file);
	return `/images/${encodeURIComponent(hex)}?${q}`;
}

/** The palette entries of a file search in image hex; without an index, a way to its Files tab. */
export function imageFileItems(res: FileSearch, hex: string): PaletteItem[] {
	if (!res.indexed) {
		return [{ kind: 'page', label: t('palette.openExplorer'), detail: t('imagefs.noIndex'), href: imageExplorerHref(hex) }];
	}
	return res.results.map((r) => ({
		kind: 'file',
		label: r.path,
		href: r.type === 'dir' ? imageExplorerHref(hex, r.path) : imageExplorerHref(hex, parentPath(r.path), r.path)
	}));
}

/** Message for a failed listing (index) or preview (extraction) of an image. */
export function imageFsErrorMessage(code: string, preview: boolean): string {
	if (code === 'too_large') return preview ? t('imagefs.fileTooLarge') : t('imagefs.tooLarge');
	if (code === 'unsupported_image') return t('imagefs.unsupported');
	return errorMessage(code);
}
