import type { EditorState, Extension } from '@codemirror/state';
import type { Language } from './write.ts';

// Each language is its own chunk, loaded when a file of that kind is edited
// (spec §3: languages on demand).
const loaders: Record<Language, () => Promise<Extension>> = {
	json: async () => (await import('@codemirror/lang-json')).json(),
	yaml: async () => (await import('@codemirror/lang-yaml')).yaml(),
	javascript: async () => (await import('@codemirror/lang-javascript')).javascript(),
	python: async () => (await import('@codemirror/lang-python')).python(),
	shell: async () => {
		const [{ StreamLanguage }, { shell }] = await Promise.all([
			import('@codemirror/language'),
			import('@codemirror/legacy-modes/mode/shell')
		]);
		return StreamLanguage.define(shell);
	},
	text: async () => []
};

export const loadLanguage = (l: Language): Promise<Extension> => loaders[l]();

/**
 * The edited text with the state's line separator (EditorState.lineSeparator).
 * Not state.doc.toString(): it always joins lines with LF, and a CRLF file
 * would be saved in LF (Q16).
 */
export const docText = (state: EditorState): string => state.sliceDoc();
