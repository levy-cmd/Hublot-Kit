<script lang="ts">
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import { ApiError, checkDownload, downloadUrl, listDir, type Entry, type Listing } from '#lib/api.ts';
	import {
		breadcrumbs,
		downloadErrorMessage,
		formatMode,
		formatSize,
		joinPath,
		parentPath,
		resolveLinkTarget,
		sortEntries
	} from '#lib/files.ts';
	import { errorMessage, t } from '#lib/i18n/index.ts';
	import FilePreview from '#lib/components/FilePreview.svelte';

	let { id }: { id: string } = $props();

	// The URL carries the state (?path=dir&file=selected): back button and links work.
	let dir = $derived(page.url.searchParams.get('path') || '/');
	let file = $derived(page.url.searchParams.get('file'));
	let listing = $state<Listing | null>(null);
	let error = $state('');
	let loading = $state(false);
	let downloadError = $state('');
	let reloads = $state(0); // bumped by the Reload button: same URL, new request
	let entries = $derived(listing ? sortEntries(listing.entries) : []);
	let selected = $derived(file ? entries.find((e) => joinPath(dir, e.name) === file) : undefined);

	const href = (path: string, selectedFile?: string) =>
		'?' + new URLSearchParams(selectedFile ? { path, file: selectedFile } : { path });

	function go(path: string, selectedFile?: string, replace = false) {
		goto(href(path, selectedFile), { replace, reset: false }).catch(() => {});
	}

	$effect(() => {
		const wanted = dir;
		void reloads; // dependency: the Reload button re-runs this effect
		let current = true;
		loading = true;
		error = '';
		downloadError = '';
		listDir(id, wanted)
			.then((l) => {
				if (!current) return;
				// A link to a file, or a deep link: show its folder and preview it.
				if (l.type !== 'dir') go(parentPath(l.path), l.path, true);
				else listing = l;
			})
			.catch((e) => {
				if (!current) return;
				if (e instanceof ApiError && e.status === 401) {
					goto('/login').catch(() => {});
					return;
				}
				listing = null;
				const code = e instanceof ApiError ? e.code : 'unknown';
				error =
						e instanceof ApiError && e.status === 501
							? t('container.stoppedSoon')
							: code === 'forbidden_path'
								? t('files.inaccessible')
								: errorMessage(code);
			})
			.finally(() => {
				if (current) loading = false;
			});
		return () => {
			current = false;
		};
	});

	function open(e: Entry) {
		const p = joinPath(dir, e.name);
		if (e.type === 'dir') go(p);
		else if (e.type === 'symlink' && e.linkTarget !== undefined) go(resolveLinkTarget(dir, e.linkTarget));
		else go(dir, p);
	}

	async function downloadDir() {
		downloadError = '';
		const url = downloadUrl(id, dir);
		try {
			await checkDownload(url);
			location.assign(url);
		} catch (e) {
			downloadError =
				e instanceof ApiError ? downloadErrorMessage(e.code, e.detail) : downloadErrorMessage('unknown');
		}
	}
</script>

<div class="browser">
	<section class="listing">
		<nav class="crumbs" aria-label={t('files.path')}>
			{#each breadcrumbs(dir) as c, i (c.path)}
				{#if i > 1}<span aria-hidden="true">/</span>{/if}
				<a href={href(c.path)}>{c.name}</a>
			{/each}
		</nav>
		<div class="tools">
			<button type="button" onclick={downloadDir}>{t('files.downloadDir')}</button>
		</div>
		{#if downloadError}<p role="alert">{downloadError}</p>{/if}

		{#if error}
			<p role="alert">{error}</p>
			<button type="button" onclick={() => reloads++}>{t('files.reload')}</button>
		{:else if loading && !listing}
			<p>{t('files.loading')}</p>
		{:else if listing}
			{#if listing.truncated}<p class="note">{t('files.truncated')}</p>{/if}
			<div class="scroll">
				<table>
					<thead>
						<tr>
							<th scope="col">{t('files.col.name')}</th>
							<th scope="col">{t('files.col.size')}</th>
							<th scope="col">{t('files.col.mode')}</th>
							<th scope="col">{t('files.col.owner')}</th>
							<th scope="col">{t('files.col.modified')}</th>
						</tr>
					</thead>
					<tbody>
						{#if dir !== '/'}
							<tr>
								<td colspan="5"><a href={href(parentPath(dir))}>.. ({t('files.parent')})</a></td>
							</tr>
						{/if}
						{#each entries as e, i (i)}
							<tr class:selected={selected === e}>
								<td>
									<button type="button" class="name" onclick={() => open(e)}>
										{e.name}{e.type === 'dir' ? '/' : ''}
									</button>
									{#if e.type === 'symlink'}<span class="target">→ {e.linkTarget}</span>{/if}
								</td>
								<td>{e.type === 'dir' ? '' : formatSize(e.size)}</td>
								<td><code>{formatMode(e.type, e.mode)}</code></td>
								<td>{e.uid}:{e.gid}</td>
								<td>{new Date(e.modTime).toLocaleString()}</td>
							</tr>
						{/each}
					</tbody>
				</table>
			</div>
			{#if entries.length === 0}<p>{t('files.empty')}</p>{/if}
		{/if}
	</section>

	<section class="preview">
		{#if file}
			<FilePreview {id} path={file} entry={selected} />
		{:else}
			<p>{t('files.selectFile')}</p>
		{/if}
	</section>
</div>

<style>
	.browser { display: grid; gap: 1rem; grid-template-columns: minmax(0, 1fr); }
	@media (min-width: 60rem) {
		.browser { grid-template-columns: minmax(0, 3fr) minmax(0, 2fr); }
	}
	.crumbs { display: flex; flex-wrap: wrap; gap: 0.25rem; word-break: break-all; }
	.tools { margin: 0.5rem 0; }
	.scroll { overflow-x: auto; }
	table { border-collapse: collapse; width: 100%; }
	th, td { text-align: left; padding: 0.3rem 0.5rem; border-bottom: 1px solid #eee; white-space: nowrap; }
	button.name { background: none; border: none; padding: 0; color: inherit; text-decoration: underline; cursor: pointer; font: inherit; }
	.target { opacity: 0.7; margin-left: 0.4rem; }
	tr.selected { background: #eef4ff; }
	.note { font-style: italic; }
	[role='alert'] { color: #b00020; }
</style>
