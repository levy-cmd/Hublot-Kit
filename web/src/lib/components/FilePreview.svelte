<script lang="ts">
	import { goto } from '$app/navigation';
	import { ApiError, checkDownload, downloadUrl, previewFile, type Entry, type Preview } from '#lib/api.ts';
	import { baseName, downloadErrorMessage, formatMode, formatSize } from '#lib/files.ts';
	import { errorMessage, t } from '#lib/i18n/index.ts';

	let { id, path, entry }: { id: string; path: string; entry?: Entry } = $props();

	let preview = $state<Preview | null>(null);
	let imageUrl = $state('');
	let error = $state('');
	let loading = $state(false);
	let downloadError = $state('');
	let reloads = $state(0); // bumped by the Reload button: same path, new request

	$effect(() => {
		const wanted = path;
		void reloads; // dependency: the Reload button re-runs this effect
		let current = true;
		let url = '';
		preview = null;
		imageUrl = '';
		error = '';
		downloadError = '';
		loading = true;
		previewFile(id, wanted)
			.then((p) => {
				if (!current) return;
				preview = p;
				if (p.kind === 'image') imageUrl = url = URL.createObjectURL(p.blob);
			})
			.catch((e) => {
				if (!current) return;
				if (e instanceof ApiError && e.status === 401) {
					goto('/login').catch(() => {});
					return;
				}
				const code = e instanceof ApiError ? e.code : 'unknown';
				error =
						e instanceof ApiError && e.status === 501
							? t('container.stoppedSoon')
							: code === 'too_large'
								? t('files.tooLarge')
								: errorMessage(code);
			})
			.finally(() => {
				if (current) loading = false;
			});
		return () => {
			current = false;
			if (url) URL.revokeObjectURL(url);
		};
	});

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
	{#if downloadError}<p role="alert">{downloadError}</p>{/if}
</header>

{#if loading}
	<p>{t('files.loading')}</p>
{:else if error}
	<p role="alert">{error}</p>
	<button type="button" onclick={() => reloads++}>{t('files.reload')}</button>
{:else if preview?.kind === 'text'}
	<pre>{preview.text}</pre>
{:else if preview?.kind === 'image' && imageUrl}
	<img src={imageUrl} alt={baseName(path)} />
{:else if preview?.kind === 'binary'}
	<p>{t('files.binary')}</p>
{/if}

<style>
	header { display: flex; flex-wrap: wrap; gap: 0.5rem 1rem; align-items: baseline; }
	h2 { font-size: 1.1rem; margin: 0; word-break: break-all; }
	.meta { margin: 0; opacity: 0.8; font-size: 0.9rem; }
	pre { overflow: auto; max-height: 70vh; padding: 0.75rem; background: #f6f6f6; border-radius: 0.25rem; white-space: pre; }
	img { max-width: 100%; height: auto; }
	[role='alert'] { color: #b00020; }
</style>
