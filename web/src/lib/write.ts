import { errorMessage, t } from './i18n/index.ts';

/** Caps of the agent (package files, Q14): an edit never exceeds the preview. */
export const EDIT_MAX = 2 * 1024 * 1024;
export const UPLOAD_MAX = 32 * 1024 * 1024;

/**
 * Strict UTF-8, BOM kept. null when the bytes are not valid UTF-8: the
 * editor would save U+FFFD in place of each invalid byte.
 */
export function decodeUtf8(bytes: Uint8Array): string | null {
	try {
		return new TextDecoder('utf-8', { fatal: true, ignoreBOM: true }).decode(bytes);
	} catch {
		return null;
	}
}

/** The line separator of a text; null when CRLF and lone LF are mixed (saving would rewrite them all). */
export function lineSeparatorOf(text: string): '\n' | '\r\n' | null {
	const crlf = text.split('\r\n').length - 1;
	const lf = text.split('\n').length - 1;
	if (crlf === 0) return '\n';
	return crlf === lf ? '\r\n' : null;
}

/** Languages of spec §3; each one is a chunk loaded on demand (editor.ts). */
export type Language = 'json' | 'yaml' | 'javascript' | 'python' | 'shell' | 'text';

const BY_EXT: Record<string, Language> = {
	json: 'json',
	yml: 'yaml',
	yaml: 'yaml',
	js: 'javascript',
	mjs: 'javascript',
	cjs: 'javascript',
	py: 'python',
	sh: 'shell',
	bash: 'shell',
	zsh: 'shell'
};
const SHELL_NAMES = ['.bashrc', '.bash_profile', '.profile', '.zshrc'];

export function languageOf(path: string): Language {
	const name = (path.split('/').pop() ?? '').toLowerCase();
	if (SHELL_NAMES.includes(name)) return 'shell';
	const dot = name.lastIndexOf('.');
	const ext = name.slice(dot + 1);
	// hasOwn: "x.constructor" must not read Object.prototype.
	return dot > 0 && Object.hasOwn(BY_EXT, ext) ? BY_EXT[ext] : 'text';
}

/**
 * Message for a refused edit or upload. A read_only from Docker and a
 * forbidden_path name their cause in the detail (shown as text). A 504 means
 * the guard gave up while the agent may still write (Q17); so does a lost connection.
 */
export function writeErrorMessage(kind: 'edit' | 'upload', code: string, detail?: string, status?: number): string {
	// A network error is the same case: the request may have reached the agent.
	if (status === 504) return t('write.maybeWritten');
	if (code === 'network') return t('write.maybeWrittenNetwork');
	switch (code) {
		case 'conflict':
			return t(kind === 'edit' ? 'write.conflictEdit' : 'write.conflictUpload');
		case 'read_only':
			return detail ? `${t('write.readOnly')} (${detail})` : t('write.readOnly');
		case 'link_in_path':
			return t('write.linkInPath');
		case 'too_large':
			return t(kind === 'edit' ? 'write.editTooLarge' : 'write.uploadTooLarge');
		case 'forbidden_path':
			return detail ? `${t('write.forbidden')} (${detail})` : t('write.forbidden');
		default:
			return errorMessage(code);
	}
}

/**
 * False for a name the browser rewrites in the multipart filename (`"`, CR,
 * LF become %22, %0D, %0A): the agent would write another name than the one
 * the confirmation shows.
 */
export function uploadNameOk(name: string): boolean {
	return !/["\r\n]/.test(name);
}
