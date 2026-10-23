import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { ApiError, type Entry } from './api.ts';
import {
	baseName,
	imageExplorerHref,
	imageFileItems,
	imageFsErrorMessage,
	pollDelay,
	pollIndexing,
	breadcrumbs,
	downloadErrorMessage,
	formatMode,
	fsErrorMessage,
	indexingPercent,
	formatSize,
	joinPath,
	normalize,
	parentPath,
	resolveLinkTarget,
	sortEntries
} from './files.ts';
import { errorMessage, t } from './i18n/index.ts';

const e = (name: string, type: Entry['type']): Entry => ({
	name, type, size: 0, mode: 0o644, uid: 0, gid: 0, modTime: '2026-10-08T09:00:00Z'
});

describe('paths', () => {
	it('normalizes', () => {
		expect(normalize('')).toBe('/');
		expect(normalize('etc//x/./y/')).toBe('/etc/x/y');
		expect(normalize('/../../etc')).toBe('/etc');
	});
	it('joins, finds the parent and the base name', () => {
		expect(joinPath('/', 'etc')).toBe('/etc');
		expect(joinPath('/etc', 'hosts')).toBe('/etc/hosts');
		expect(parentPath('/usr/lib')).toBe('/usr');
		expect(parentPath('/')).toBe('/');
		expect(baseName('/etc/hosts')).toBe('hosts');
		expect(baseName('/')).toBe('/');
	});
	it('resolves link targets as text', () => {
		expect(resolveLinkTarget('/etc', '../usr/lib/os-release')).toBe('/usr/lib/os-release');
		expect(resolveLinkTarget('/usr/bin', '/bin/busybox')).toBe('/bin/busybox');
		expect(resolveLinkTarget('/', '../../x')).toBe('/x');
		expect(resolveLinkTarget('/a', 'b/./c/')).toBe('/a/b/c');
	});
	it('builds breadcrumbs', () => {
		expect(breadcrumbs('/')).toEqual([{ name: '/', path: '/' }]);
		expect(breadcrumbs('/usr/lib')).toEqual([
			{ name: '/', path: '/' },
			{ name: 'usr', path: '/usr' },
			{ name: 'lib', path: '/usr/lib' }
		]);
	});
});

describe('sortEntries', () => {
	it('puts directories first, then sorts by name', () => {
		const sorted = sortEntries([e('b.txt', 'file'), e('z', 'dir'), e('a', 'symlink'), e('c', 'dir')]);
		expect(sorted.map((x) => x.name)).toEqual(['c', 'z', 'a', 'b.txt']);
	});
});

describe('formatMode', () => {
	it('formats like ls', () => {
		expect(formatMode('dir', 0o755)).toBe('drwxr-xr-x');
		expect(formatMode('file', 0o644)).toBe('-rw-r--r--');
		expect(formatMode('symlink', 0o777)).toBe('lrwxrwxrwx');
		expect(formatMode('other', 0o600)).toBe('?rw-------');
	});
	it('shows setuid, setgid and sticky', () => {
		expect(formatMode('file', 0o4755)).toBe('-rwsr-xr-x');
		expect(formatMode('file', 0o4644)).toBe('-rwSr--r--');
		expect(formatMode('file', 0o2755)).toBe('-rwxr-sr-x');
		expect(formatMode('dir', 0o1777)).toBe('drwxrwxrwt');
		expect(formatMode('dir', 0o1776)).toBe('drwxrwxrwT');
	});
});

describe('formatSize', () => {
	it('uses binary units', () => {
		expect(formatSize(0)).toBe(`0 ${t('files.unit.b')}`);
		expect(formatSize(1023)).toBe(`1023 ${t('files.unit.b')}`);
		expect(formatSize(1536)).toBe(`1.5 ${t('files.unit.kib')}`);
		expect(formatSize(2 * 1024 * 1024)).toBe(`2.0 ${t('files.unit.mib')}`);
	});
});

describe('downloadErrorMessage', () => {
	it('explains a refused directory download and names the mount', () => {
		expect(downloadErrorMessage('forbidden_path', 'chemin interdit : bind /proc -> /host/proc')).toBe(
			`${t('files.archiveForbidden')} (chemin interdit : bind /proc -> /host/proc)`
		);
		expect(downloadErrorMessage('forbidden_path')).toBe(t('files.archiveForbidden'));
		expect(downloadErrorMessage('link_in_path', 'x')).toBe(t('files.linkDirHint'));
		expect(downloadErrorMessage('docker_unavailable', 'dial')).toBe(errorMessage('docker_unavailable'));
	});
});

describe('indexingPercent', () => {
	it('never shows 100 before the end', () => {
		expect([undefined, 0, 0.42, 0.99, 1, 7, -1].map(indexingPercent)).toEqual([0, 0, 42, 99, 99, 99, 0]);
	});
});

describe('fsErrorMessage', () => {
	it('names the mount of a stopped container that cannot be explored', () => {
		expect(fsErrorMessage('forbidden_path', 'bind /proc -> /host/proc', true)).toBe(
			`${t('files.stoppedUnavailable')} (bind /proc -> /host/proc)`
		);
	});
	it('keeps the pseudo filesystem message of a running container', () => {
		expect(fsErrorMessage('forbidden_path', undefined, false)).toBe(t('files.inaccessible'));
	});
	it('explains the index cap of a stopped container', () => {
		expect(fsErrorMessage('too_large', undefined, true)).toBe(t('files.stoppedTooLarge'));
		expect(fsErrorMessage('not_found', undefined, true)).toBe(errorMessage('not_found'));
	});
});

describe('pollIndexing (image files, spec v0.2 §5.5)', () => {
	beforeEach(() => {
		vi.useFakeTimers();
	});
	afterEach(() => {
		vi.useRealTimers();
	});
	const indexing = () => Promise.reject(new ApiError(202, 'indexing', undefined, 0.5));

	it('asks again every 2 s, always under the 15 s Idle of the agent', () => {
		expect(pollDelay).toBe(2000);
		expect(pollDelay).toBeLessThan(15_000 / 2);
	});

	it('polls while 202, then gives the result once', async () => {
		const ask = vi.fn<() => Promise<string>>().mockImplementationOnce(indexing).mockImplementationOnce(indexing).mockResolvedValue('ok');
		const seen: unknown[] = [];
		pollIndexing(ask, { indexing: (p) => seen.push(p), result: (v) => seen.push(v), error: (e) => seen.push(e) });
		await vi.advanceTimersByTimeAsync(0);
		expect(ask).toHaveBeenCalledTimes(1);
		await vi.advanceTimersByTimeAsync(1999);
		expect(ask).toHaveBeenCalledTimes(1);
		await vi.advanceTimersByTimeAsync(1);
		expect(ask).toHaveBeenCalledTimes(2);
		await vi.advanceTimersByTimeAsync(2000);
		expect(ask).toHaveBeenCalledTimes(3);
		await vi.advanceTimersByTimeAsync(10_000);
		expect(ask).toHaveBeenCalledTimes(3);
		expect(seen).toEqual([0.5, 0.5, 'ok']);
	});

	it('stops when another file is chosen: no request, no callback after', async () => {
		const ask = vi.fn<() => Promise<string>>().mockImplementation(indexing);
		const on = { indexing: vi.fn(), result: vi.fn(), error: vi.fn() };
		const stop = pollIndexing(ask, on);
		await vi.advanceTimersByTimeAsync(2000);
		expect(ask).toHaveBeenCalledTimes(2);
		stop();
		await vi.advanceTimersByTimeAsync(20_000);
		expect(ask).toHaveBeenCalledTimes(2);
		expect(on.result).not.toHaveBeenCalled();
		expect(on.error).not.toHaveBeenCalled();
	});

	it('drops an answer that arrives after the stop', async () => {
		let resolve: (v: string) => void = () => {};
		const ask = vi.fn(() => new Promise<string>((r) => (resolve = r)));
		const on = { indexing: vi.fn(), result: vi.fn(), error: vi.fn() };
		const stop = pollIndexing(ask, on);
		stop();
		resolve('late');
		await vi.advanceTimersByTimeAsync(0);
		expect(on.result).not.toHaveBeenCalled();
	});

	it('gives any other error at once, without asking again', async () => {
		const ask = vi.fn<() => Promise<string>>().mockRejectedValue(new ApiError(422, 'unsupported_image'));
		const on = { indexing: vi.fn(), result: vi.fn(), error: vi.fn() };
		pollIndexing(ask, on);
		await vi.advanceTimersByTimeAsync(10_000);
		expect(ask).toHaveBeenCalledTimes(1);
		expect(on.error).toHaveBeenCalledTimes(1);
	});
});

describe('image files', () => {
	const hex = 'ab'.repeat(32);
	it('links the Files tab of the image', () => {
		expect(imageExplorerHref(hex)).toBe(`/images/${hex}?tab=files`);
		expect(imageExplorerHref(hex, '/etc', '/etc/os-release')).toBe(`/images/${hex}?tab=files&path=%2Fetc&file=%2Fetc%2Fos-release`);
	});
	it('turns a search into palette entries, or a way to the Files tab', () => {
		expect(imageFileItems({ indexed: false, results: [], truncated: false }, hex)).toEqual([
			{ kind: 'page', label: t('palette.openExplorer'), detail: t('imagefs.noIndex'), href: `/images/${hex}?tab=files` }
		]);
		const items = imageFileItems(
			{ indexed: true, results: [{ path: '/etc/nginx', type: 'dir' }, { path: '/etc/nginx/nginx.conf', type: 'file' }], truncated: false },
			hex
		);
		expect(items.map((i) => i.href)).toEqual([
			imageExplorerHref(hex, '/etc/nginx'),
			imageExplorerHref(hex, '/etc/nginx', '/etc/nginx/nginx.conf')
		]);
	});
	it('explains the refusals of an image', () => {
		expect(imageFsErrorMessage('too_large', false)).toBe(t('imagefs.tooLarge'));
		expect(imageFsErrorMessage('too_large', true)).toBe(t('imagefs.fileTooLarge'));
		expect(imageFsErrorMessage('unsupported_image', false)).toBe(t('imagefs.unsupported'));
		expect(imageFsErrorMessage('not_regular', true)).toBe(errorMessage('not_regular'));
	});
});
