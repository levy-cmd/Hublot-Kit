import type { Entry, EntryType } from './api.ts';
import { errorMessage, t } from './i18n/index.ts';

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
