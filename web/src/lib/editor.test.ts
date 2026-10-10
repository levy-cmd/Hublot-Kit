import { EditorState } from '@codemirror/state';
import { describe, expect, it } from 'vitest';
import { docText, loadLanguage } from './editor.ts';

describe('loadLanguage', () => {
	it('loads every language on demand', async () => {
		for (const l of ['json', 'yaml', 'javascript', 'python', 'shell', 'text'] as const) {
			await expect(loadLanguage(l)).resolves.toBeDefined();
		}
		// A real language is a non-empty extension; plain text adds nothing.
		expect(await loadLanguage('json')).not.toEqual([]);
		expect(await loadLanguage('shell')).not.toEqual([]);
		expect(await loadLanguage('text')).toEqual([]);
	});
});

describe('docText', () => {
	it('gives back the text with the file own line separator, byte for byte', () => {
		const cases: [string, string][] = [
			['a\r\nb\r\n', '\r\n'],
			['a\nb\n', '\n'],
			['a\rb\n', '\n'] // a lone CR stays a character
		];
		for (const [doc, sep] of cases) {
			const state = EditorState.create({ doc, extensions: [EditorState.lineSeparator.of(sep)] });
			expect(docText(state)).toBe(doc);
		}
		// The trap: Text.toString() always joins with LF.
		const crlf = EditorState.create({ doc: 'a\r\nb', extensions: [EditorState.lineSeparator.of('\r\n')] });
		expect(crlf.doc.toString()).toBe('a\nb');
	});
});
