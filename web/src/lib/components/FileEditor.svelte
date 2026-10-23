<script lang="ts">
	import type { EditorView } from '@codemirror/view';
	import { docText, loadLanguage } from '#lib/editor.ts';
	import { t } from '#lib/i18n/index.ts';
	import { languageOf, lineSeparatorOf } from '#lib/write.ts';

	// CodeMirror and the language are loaded when the editor opens: none of
	// it is in the initial bundle (spec §3).
	let { path, text }: { path: string; text: string } = $props();

	let host = $state<HTMLDivElement>();
	let view: EditorView | undefined;
	let ready = $state(false);
	let failed = $state(false);

	$effect(() => {
		const target = host;
		const doc = text;
		const lang = languageOf(path);
		if (!target) return;
		let current = true;
		ready = false;
		failed = false;
		Promise.all([import('codemirror'), import('@codemirror/state'), import('@codemirror/language'), loadLanguage(lang)])
			.then(([{ EditorView, basicSetup }, { EditorState }, { HighlightStyle, defaultHighlightStyle, syntaxHighlighting }, language]) => {
				if (!current) return;
				// The file's own line separator: saving never rewrites line endings.
				const sep = lineSeparatorOf(doc) ?? '\n';
				// The tokens of the page; on the dark theme, the default syntax
				// colours keep their hue and are lightened to stay readable.
				const dark = document.documentElement.dataset.theme !== 'light';
				const colours = dark
					? [syntaxHighlighting(HighlightStyle.define(defaultHighlightStyle.specs.map((s) => ({ ...s, color: lighten(s.color) }))))]
					: [];
				view = new EditorView({
					doc,
					parent: target,
					extensions: [basicSetup, language, EditorState.lineSeparator.of(sep), ...colours, chrome(EditorView, dark)]
				});
				ready = true;
			})
			.catch(() => {
				if (current) failed = true;
			});
		return () => {
			current = false;
			view?.destroy();
			view = undefined;
		};
	});

	type View = typeof import('@codemirror/view').EditorView;

	const chrome = (EditorView: View, dark: boolean) =>
		EditorView.theme(
			{
				'&': { backgroundColor: 'var(--releve)', color: 'var(--texte)', fontSize: 'var(--t-s)' },
				'.cm-content': { fontFamily: 'var(--mono)', caretColor: 'var(--texte)' },
				'.cm-cursor, .cm-dropCursor': { borderLeftColor: 'var(--texte)' },
				'.cm-gutters': { backgroundColor: 'var(--surface)', color: 'var(--texte-doux)', borderRight: '1px solid var(--trait)' },
				'.cm-activeLine, .cm-activeLineGutter': { backgroundColor: 'var(--surface)' },
				'&.cm-focused .cm-selectionBackground, .cm-selectionBackground, ::selection': { backgroundColor: 'var(--selection)' },
				'&.cm-focused': { outline: '2px solid var(--focus)' }
			},
			{ dark }
		);

	/** A #rgb or #rrggbb colour with its hue, lightness raised to 75 %. */
	function lighten(c: string | undefined): string | undefined {
		if (!c || !/^#([0-9a-f]{3}|[0-9a-f]{6})$/i.test(c)) return c;
		const h = c.length === 4 ? c.replace(/^#(.)(.)(.)$/, '#$1$1$2$2$3$3') : c;
		const [r, g, b] = [1, 3, 5].map((i) => parseInt(h.slice(i, i + 2), 16) / 255);
		const max = Math.max(r, g, b);
		const min = Math.min(r, g, b);
		const d = max - min;
		let hue = 0;
		if (d) hue = max === r ? ((g - b) / d) % 6 : max === g ? (b - r) / d + 2 : (r - g) / d + 4;
		const sat = d ? Math.min(1, d / (1 - Math.abs(max + min - 1)) || 0) : 0;
		return `hsl(${Math.round(hue * 60 + 360) % 360} ${Math.round(Math.min(sat, 0.7) * 100)}% 75%)`;
	}

	/** The edited text, with the file's line separator; null while the editor loads. */
	export function content(): string | null {
		return view ? docText(view.state) : null;
	}
</script>

{#if failed}
	<p role="alert">{t('write.editorFailed')}</p>
{:else if !ready}
	<p>{t('write.editorLoading')}</p>
{/if}
<div class="editor" bind:this={host}></div>

<style>
	.editor { border: 1px solid var(--trait-fort); border-radius: var(--rayon); max-height: 70vh; overflow: auto; margin-top: 0.5rem; }
	.editor :global(.cm-editor) { min-height: 12rem; }
</style>
