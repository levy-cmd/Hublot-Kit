import { describe, expect, it } from 'vitest';
import { t } from './i18n/index.ts';
import { decodeUtf8, languageOf, lineSeparatorOf, writeErrorMessage } from './write.ts';

describe('decodeUtf8', () => {
	it('keeps valid UTF-8, BOM included', () => {
		expect(decodeUtf8(new TextEncoder().encode('été'))).toBe('été');
		expect(decodeUtf8(new Uint8Array([0xef, 0xbb, 0xbf, 0x61]))).toBe('﻿a');
	});
	it('refuses invalid bytes: saving would replace them by U+FFFD', () => {
		expect(decodeUtf8(new Uint8Array([0x61, 0xe9, 0x62]))).toBeNull(); // Latin-1 "aéb"
	});
});

describe('lineSeparatorOf', () => {
	it('finds LF, CRLF, or refuses a mix', () => {
		expect(lineSeparatorOf('a\nb\n')).toBe('\n');
		expect(lineSeparatorOf('sans fin de ligne')).toBe('\n');
		expect(lineSeparatorOf('a\r\nb\r\n')).toBe('\r\n');
		expect(lineSeparatorOf('a\r\nb\n')).toBeNull();
	});
});

describe('languageOf', () => {
	it('maps the languages of spec §3, anything else is plain text', () => {
		expect(languageOf('/app/package.json')).toBe('json');
		expect(languageOf('/etc/compose.YML')).toBe('yaml');
		expect(languageOf('/srv/app.yaml')).toBe('yaml');
		expect(languageOf('/app/index.mjs')).toBe('javascript');
		expect(languageOf('/app/main.py')).toBe('python');
		expect(languageOf('/entrypoint.sh')).toBe('shell');
		expect(languageOf('/root/.bashrc')).toBe('shell');
		expect(languageOf('/etc/nginx/nginx.conf')).toBe('text');
		expect(languageOf('/etc/hosts')).toBe('text');
		expect(languageOf('/json')).toBe('text');
	});
});

describe('writeErrorMessage', () => {
	it('says what to do for each refusal', () => {
		expect(writeErrorMessage('edit', 'conflict')).toBe(t('write.conflictEdit'));
		expect(writeErrorMessage('upload', 'conflict')).toBe(t('write.conflictUpload'));
		expect(writeErrorMessage('upload', 'read_only', 'container rootfs is marked read-only')).toBe(
			`${t('write.readOnly')} (container rootfs is marked read-only)`
		);
		expect(writeErrorMessage('edit', 'link_in_path')).toBe(t('write.linkInPath'));
		expect(writeErrorMessage('edit', 'too_large')).toBe(t('write.editTooLarge'));
		expect(writeErrorMessage('upload', 'too_large')).toBe(t('write.uploadTooLarge'));
		expect(writeErrorMessage('upload', 'invalid_name')).toBe(t('error.invalid_name'));
		expect(writeErrorMessage('upload', 'forbidden_path', 'tmpfs /scratch')).toBe(`${t('write.forbidden')} (tmpfs /scratch)`);
		// 504: the guard gave up, the write may still happen (Q17).
		expect(writeErrorMessage('edit', 'internal', 'délai dépassé', 504)).toBe(t('write.maybeWritten'));
		expect(writeErrorMessage('edit', 'internal', undefined, 500)).toBe(t('error.internal'));
	});
});
