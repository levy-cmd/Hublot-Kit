<script lang="ts">
	import { goto } from '$app/navigation';
	import { ApiError, checkDownload, downloadUrl, previewFile, saveFile, type Entry, type Preview } from '#lib/api.ts';
	import { baseName, downloadErrorMessage, formatMode, formatSize, fsErrorMessage, indexingPercent } from '#lib/files.ts';
	import { t } from '#lib/i18n/index.ts';
	import { latest, lock } from '#lib/guard.ts';
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
</script>

<header>
	<h2>{baseName(path)}</h2>
	{#if entry}
		<p class="meta">
			{formatSize(entry.size)} · <code>{formatMode(entry.type, entry.mode)}</code> · {entry.uid}:{entry.gid} ·
			{new Date(entry.modTime).toLocaleString()}
		</p>
	{/if}
	<button type="button" onclick={download}>{t('files.download')}</button>
	{#if !readOnly && editable && !editing}
		<button
			type="button"
			onclick={() => {
				editing = true;
				saved = false;
			}}>{t('write.edit')}</button
		>
	{/if}
	{#if downloadError}<p role="alert">{downloadError}</p>{/if}
	{#if saved}<p role="status">{t('write.saved')}</p>{/if}
</header>

{#if indexing !== null}
	<p role="status">{t('files.indexing')} : {indexing} %</p>
	<progress max="100" value={indexing}></progress>
	<p class="note">{t('files.indexingHint')}</p>
{:else if loading}
	<p>{t('files.loading')}</p>
{:else if error}
	<p role="alert">{error}</p>
	<button type="button" onclick={() => reloads++}>{t('files.reload')}</button>
{:else if preview?.kind === 'text' && editing}
	<FileEditor bind:this={editor} {path} text={preview.text} />
	<div class="actions">
		<button type="button" onclick={askSave} disabled={saving}>{saving ? t('write.saving') : t('write.save')}</button>
		<button
			type="button"
			disabled={saving}
			onclick={() => {
				editing = false;
				writeError = '';
			}}>{t('write.cancel')}</button
		>
		{#if writeError}<button type="button" disabled={saving} onclick={() => reloads++}>{t('files.reload')}</button>{/if}
	</div>
	{#if writeError}<p role="alert">{writeError}</p>{/if}
{:else if preview?.kind === 'text'}
	<pre>{preview.text}</pre>
	{#if !readOnly && !editable}<p class="note">{t('write.notEditable')}</p>{/if}
{:else if preview?.kind === 'image' && imageUrl}
	<img src={imageUrl} alt={baseName(path)} />
{:else if preview?.kind === 'binary'}
	<p>{t('files.binary')}</p>
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
	header { display: flex; flex-wrap: wrap; gap: 0.5rem 1rem; align-items: baseline; }
	h2 { font-size: 1.1rem; margin: 0; word-break: break-all; }
	.meta { margin: 0; opacity: 0.8; font-size: 0.9rem; }
	pre { overflow: auto; max-height: 70vh; padding: 0.75rem; background: #f6f6f6; border-radius: 0.25rem; white-space: pre; }
	img { max-width: 100%; height: auto; }
	.note { font-style: italic; }
	.actions { display: flex; flex-wrap: wrap; gap: 0.5rem; margin-top: 0.5rem; }
	[role='alert'] { color: #b00020; }
</style>
