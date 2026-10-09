import { describe, expect, it } from 'vitest';
import type { Entry } from './api.ts';
import {
	baseName,
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
