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
		Promise.all([import('codemirror'), import('@codemirror/state'), loadLanguage(lang)])
			.then(([{ EditorView, basicSetup }, { EditorState }, language]) => {
				if (!current) return;
				// The file's own line separator: saving never rewrites line endings.
				const sep = lineSeparatorOf(doc) ?? '\n';
				view = new EditorView({
					doc,
					parent: target,
					extensions: [basicSetup, language, EditorState.lineSeparator.of(sep)]
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
	.editor { border: 1px solid var(--trait-fin); border-radius: var(--rayon); max-height: 70vh; overflow: auto; }
	.editor :global(.cm-editor) { min-height: 12rem; }
	[role='alert'] { color: var(--rouille-texte); }
</style>
