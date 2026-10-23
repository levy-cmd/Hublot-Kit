<script lang="ts">
	import { goto } from '$app/navigation';
	import { ApiError, checkDownload, downloadUrl, previewFile, saveFile, type Entry, type Preview } from '#lib/api.ts';
	import { baseName, downloadErrorMessage, formatMode, formatSize, fsErrorMessage, indexingPercent } from '#lib/files.ts';
	import { lang, t } from '#lib/i18n/index.ts';
	import { latest, lock } from '#lib/guard.ts';
	import Icon from '#lib/icons/Icon.svelte';
	import { EDIT_MAX, lineSeparatorOf, writeErrorMessage } from '#lib/write.ts';
	import ConfirmDialog from '#lib/components/ConfirmDialog.svelte';
	import FileEditor from '#lib/components/FileEditor.svelte';

	let {
		id,
		name,
		path,
		entry,
		stopped = false,
		readOnly = true
	}: { id: string; name: string; path: string; entry?: Entry; stopped?: boolean; readOnly?: boolean } = $props();

	let preview = $state<Preview | null>(null);
	let imageUrl = $state('');
	let error = $state('');
	let indexing = $state<number | null>(null); // percent while the index is built
	let loading = $state(false);
	let downloadError = $state('');
	let reloads = $state(0); // bumped by the Reload button: same path, new request
	let editing = $state(false);
	let editor = $state<FileEditor>();
	// What the dialog names is exactly what save() sends: frozen when it opens.
	let pending = $state<{ id: string; path: string; etag: string; text: string } | null>(null);
	const writes = latest(); // stale-response guard: navigation cancels a write in flight
	const saveLock = lock();
	let saving = $state(false);
	let writeError = $state('');
	let saved = $state(false);
	// Editable: valid UTF-8, one kind of line ending (saving would change the bytes otherwise)
	// and an ETag to condition the write on (never an empty If-Match).
	let editable = $derived(
		preview?.kind === 'text' && preview.utf8 && preview.etag !== '' && lineSeparatorOf(preview.text) !== null
	);

	$effect(() => {
		const wanted = path;
		void id;
		void reloads; // dependency: the Reload button re-runs this effect
		let current = true;
		let url = '';
		let retry: ReturnType<typeof setTimeout> | undefined;
		preview = null;
		imageUrl = '';
		error = '';
		downloadError = '';
		editing = false;
		pending = null;
		writes.cancel();
		writeError = '';
		saved = false;
		loading = true;
		previewFile(id, wanted)
			.then((p) => {
				if (!current) return;
				indexing = null;
				preview = p;
				if (p.kind === 'image') imageUrl = url = URL.createObjectURL(p.blob);
			})
			.catch((e) => {
				if (!current) return;
				if (e instanceof ApiError && e.status === 401) {
					goto('/login').catch(() => {});
					return;
				}
				if (e instanceof ApiError && e.code === 'indexing') {
					indexing = indexingPercent(e.progress);
					retry = setTimeout(() => reloads++, 1000); // ask again until the index is ready
					return;
				}
				indexing = null;
				const code = e instanceof ApiError ? e.code : 'unknown';
				error =
					code === 'too_large'
						? t('files.tooLarge')
						: fsErrorMessage(code, e instanceof ApiError ? e.detail : undefined, stopped);
			})
			.finally(() => {
				if (current) loading = false;
			});
		return () => {
			current = false;
			clearTimeout(retry);
			if (url) URL.revokeObjectURL(url);
		};
	});

	function askSave() {
		const text = editor?.content() ?? null; // null while the editor loads: nothing to confirm
		writeError = '';
		if (text !== null && new TextEncoder().encode(text).length > EDIT_MAX) {
			writeError = t('write.editTooLarge'); // the agent would refuse it: no confirmation
			return;
		}
		pending = text !== null && preview?.kind === 'text' ? { id, path, etag: preview.etag, text } : null;
	}

	function save() {
		const p = pending;
		pending = null;
		if (!p || preview?.kind !== 'text') return;
		const current = preview;
		void saveLock.run(async () => {
			const token = writes.begin();
			saving = true;
			try {
				const etag = await saveFile(p.id, p.path, p.text, p.etag);
				if (!writes.isCurrent(token)) return;
				preview = { ...current, text: p.text, etag };
				editing = false;
				saved = true;
			} catch (e) {
				if (!writes.isCurrent(token)) return;
				if (e instanceof ApiError && e.status === 401) {
					goto('/login').catch(() => {});
					return;
				}
				writeError =
					e instanceof ApiError ? writeErrorMessage('edit', e.code, e.detail, e.status) : writeErrorMessage('edit', 'unknown');
			} finally {
				saving = false;
			}
		});
	}

	async function download() {
		downloadError = '';
		const url = downloadUrl(id, path);
		try {
			await checkDownload(url);
			location.assign(url); // Content-Disposition: attachment, the page stays
		} catch (e) {
			downloadError =
				e instanceof ApiError ? downloadErrorMessage(e.code, e.detail) : downloadErrorMessage('unknown');
		}
	}

	// "1\n2\n…": the gutter of the preview, one number per line of the text.
	let lineNumbers = $derived.by(() => {
		if (preview?.kind !== 'text') return '';
		const text = preview.text;
		let n = 1;
		for (let i = 0; i < text.length; i++) if (text.charCodeAt(i) === 10) n++;
		if (text.endsWith('\n')) n--;
		return Array.from({ length: Math.max(n, 1) }, (_, i) => i + 1).join('\n');
	});

	const when = (iso: string) => new Date(iso).toLocaleString(lang, { dateStyle: 'medium', timeStyle: 'short' });
</script>

<header>
	<span class="ficon"><Icon name="file" /></span>
	<div class="title">
		<h3>{baseName(path)}</h3>
		<p class="path mono">{path}</p>
	</div>
	<div class="actions">
		{#if !readOnly && editable && !editing}
			<button
				type="button"
				class="btn small"
				onclick={() => {
					editing = true;
					saved = false;
				}}><Icon name="edit" /> {t('write.edit')}</button
			>
		{/if}
		<button type="button" class="icon-btn small" onclick={download} aria-label={t('files.download')} title={t('files.download')}>
			<Icon name="download" />
		</button>
	</div>
</header>
{#if entry}
	<dl class="meta">
		<div><dt class="sr-only">{t('files.col.size')}</dt><dd class="mono">{formatSize(entry.size)}</dd></div>
		<div><dt class="sr-only">{t('files.col.mode')}</dt><dd class="mono">{formatMode(entry.type, entry.mode)}</dd></div>
		<div><dt class="sr-only">{t('files.col.owner')}</dt><dd class="mono">{entry.uid}:{entry.gid}</dd></div>
		<div><dt class="sr-only">{t('files.col.modified')}</dt><dd>{when(entry.modTime)}</dd></div>
	</dl>
{/if}
{#if downloadError}<p role="alert">{downloadError}</p>{/if}
{#if saved}<p role="status" class="saved">{t('write.saved')}</p>{/if}

{#if indexing !== null}
	<p role="status">{t('files.indexing')} : {indexing} %</p>
	<progress max="100" value={indexing}></progress>
	<p class="note">{t('files.indexingHint')}</p>
{:else if loading}
	<p class="muted">{t('files.loading')}</p>
{:else if error}
	<p role="alert">{error}</p>
	<button type="button" class="btn small" onclick={() => reloads++}>{t('files.reload')}</button>
{:else if preview?.kind === 'text' && editing}
	<FileEditor bind:this={editor} {path} text={preview.text} />
	<div class="edit-actions">
		<button type="button" class="btn primary small" onclick={askSave} disabled={saving}>{saving ? t('write.saving') : t('write.save')}</button>
		<button
			type="button"
			class="btn small"
			disabled={saving}
			onclick={() => {
				editing = false;
				writeError = '';
			}}>{t('write.cancel')}</button
		>
		{#if writeError}<button type="button" class="btn small" disabled={saving} onclick={() => reloads++}>{t('files.reload')}</button>{/if}
	</div>
	{#if writeError}<p role="alert">{writeError}</p>{/if}
{:else if preview?.kind === 'text'}
	<div class="code">
		<pre class="gutter" aria-hidden="true">{lineNumbers}</pre>
		<pre class="text">{preview.text}</pre>
	</div>
	{#if !readOnly && !editable}<p class="note">{t('write.notEditable')}</p>{/if}
{:else if preview?.kind === 'image' && imageUrl}
	<img src={imageUrl} alt={baseName(path)} />
{:else if preview?.kind === 'binary'}
	<p class="muted">{t('files.binary')}</p>
{/if}

{#if pending !== null}
	<ConfirmDialog
		title={t('confirm.edit')}
		container={`${name} (${pending.id.slice(0, 12)})`}
		path={pending.path}
		details={stopped ? [t('confirm.stoppedNote')] : []}
		onconfirm={save}
		oncancel={() => (pending = null)}
	/>
{/if}

<style>
	header {
		display: flex;
		align-items: flex-start;
		gap: 0.6rem;
		min-width: 0;
	}
	.ficon {
		color: var(--texte-doux);
		padding-top: 0.15rem;
	}
	.title {
		flex: 1;
		min-width: 0;
	}
	h3 {
		font-size: var(--t-m);
		overflow-wrap: anywhere;
	}
	.path {
		color: var(--texte-doux);
		font-size: var(--t-xs);
		overflow-wrap: anywhere;
	}
	.actions {
		display: flex;
		gap: 0.25rem;
		align-items: center;
	}
	.meta {
		display: flex;
		flex-wrap: wrap;
		gap: 0.25rem 1rem;
		margin: 0.5rem 0;
		font-size: var(--t-xs);
		color: var(--texte-doux);
	}
	.meta dd {
		margin: 0;
	}
	.saved {
		color: var(--emeraude-texte);
	}
	.code {
		display: flex;
		max-height: 70vh;
		overflow: auto;
		margin-top: 0.5rem;
		background: var(--releve);
		border-radius: var(--rayon);
		font-size: var(--t-xs);
		line-height: 1.6;
	}
	pre {
		margin: 0;
		padding: 0.6rem 0.75rem;
		font-size: inherit;
		white-space: pre;
	}
	.gutter {
		position: sticky;
		left: 0;
		flex: none;
		text-align: right;
		color: var(--texte-doux);
		background: var(--releve);
		border-right: 1px solid var(--trait);
		user-select: none;
	}
	.text {
		flex: 1;
	}
	img {
		max-width: 100%;
		height: auto;
		margin-top: 0.5rem;
		border-radius: var(--rayon);
	}
	.edit-actions {
		display: flex;
		flex-wrap: wrap;
		gap: 0.5rem;
		margin-top: 0.5rem;
	}
</style>
