<script lang="ts">
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import {
		ApiError,
		checkDownload,
		downloadUrl,
		listChanges,
		listDir,
		type Change,
		type Entry,
		type Listing,
		uploadFile
	} from '#lib/api.ts';
	import { changeMarks, deletedIn, kindLabel, kindLetter, truncateNames } from '#lib/changes.ts';
	import {
		breadcrumbs,
		downloadErrorMessage,
		formatMode,
		formatSize,
		fsErrorMessage,
		indexingPercent,
		joinPath,
		parentPath,
		resolveLinkTarget,
		sortEntries
	} from '#lib/files.ts';
	import { errorMessage, t } from '#lib/i18n/index.ts';
	import { latest, lock } from '#lib/guard.ts';
	import { UPLOAD_MAX, uploadNameOk, writeErrorMessage } from '#lib/write.ts';
	import ConfirmDialog from '#lib/components/ConfirmDialog.svelte';
	import FilePreview from '#lib/components/FilePreview.svelte';

	let {
		id,
		name,
		stopped = false,
		readOnly = true
	}: { id: string; name: string; stopped?: boolean; readOnly?: boolean } = $props();

	// The URL carries the state (?path=dir&file=selected): back button and links work.
	let dir = $derived(page.url.searchParams.get('path') || '/');
	let file = $derived(page.url.searchParams.get('file'));
	let listing = $state<Listing | null>(null);
	let error = $state('');
	let indexing = $state<number | null>(null); // percent while the index is built
	let loading = $state(false);
	let downloadError = $state('');
	let reloads = $state(0); // bumped by the Reload button: same URL, new request
	let entries = $derived(listing ? sortEntries(listing.entries) : []);
	let changes = $state<Change[]>([]);
	let marks = $derived(changeMarks(changes));
	let deleted = $derived(deletedIn(changes, dir));
	let selected = $derived(file ? entries.find((e) => joinPath(dir, e.name) === file) : undefined);

	let picker = $state<HTMLInputElement>();
	let toUpload = $state<{ id: string; file: File; dir: string } | null>(null); // waiting for the confirmation
	const uploads = latest(); // stale-response guard: another container cancels an upload in flight
	const uploadLock = lock();
	$effect(() => {
		void id;
		toUpload = null;
		return () => uploads.cancel();
	});
	let uploading = $state(false);
	let uploadError = $state('');

	const href = (path: string, selectedFile?: string) =>
		'?' + new URLSearchParams(selectedFile ? { path, file: selectedFile } : { path });

	function go(path: string, selectedFile?: string, replace = false) {
		goto(href(path, selectedFile), { replace, reset: false }).catch(() => {});
	}

	$effect(() => {
		const wanted = dir;
		void reloads; // dependency: the Reload button re-runs this effect
		let current = true;
		let retry: ReturnType<typeof setTimeout> | undefined;
		loading = true;
		error = '';
		downloadError = '';
		listDir(id, wanted)
			.then((l) => {
				if (!current) return;
				indexing = null;
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
				if (e instanceof ApiError && e.code === 'indexing') {
					indexing = indexingPercent(e.progress);
					retry = setTimeout(() => reloads++, 1000); // ask again until the index is ready
					return;
				}
				indexing = null;
				error = e instanceof ApiError ? fsErrorMessage(e.code, e.detail, stopped) : errorMessage('unknown');
			})
			.finally(() => {
				if (current) loading = false;
			});
		return () => {
			current = false;
			clearTimeout(retry);
		};
	});

	// Depends on id only: the indexing retry (every second) must not re-ask the diff.
	// If it fails, no marks and no message (plan decision).
	$effect(() => {
		const wanted = id;
		let current = true;
		changes = [];
		listChanges(wanted)
			.then((l) => {
				if (current) changes = l ?? [];
			})
			.catch(() => {});
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

	function picked(e: Event & { currentTarget: HTMLInputElement }) {
		const f = e.currentTarget.files?.[0];
		e.currentTarget.value = ''; // the same file can be picked again
		uploadError = '';
		if (!f) return;
		if (f.size > UPLOAD_MAX) {
			uploadError = t('write.uploadTooLarge');
			return;
		}
		if (!uploadNameOk(f.name)) {
			uploadError = t('write.invalidName');
			return;
		}
		toUpload = { id, file: f, dir };
	}

	function upload() {
		const u = toUpload;
		toUpload = null;
		if (!u) return;
		void uploadLock.run(async () => {
			const token = uploads.begin();
			uploading = true;
			try {
				const res = await uploadFile(u.id, u.dir, u.file);
				if (!uploads.isCurrent(token)) return;
				reloads++;
				if (dir === u.dir) go(u.dir, res.path);
			} catch (e) {
				if (!uploads.isCurrent(token)) return;
				if (e instanceof ApiError && e.status === 401) {
					goto('/login').catch(() => {});
					return;
				}
				uploadError =
					e instanceof ApiError
						? writeErrorMessage('upload', e.code, e.detail, e.status)
						: writeErrorMessage('upload', 'unknown');
				// The write may have happened (504, network): the listing is stale.
				if (e instanceof ApiError && (e.status === 504 || e.code === 'network')) reloads++;
			} finally {
				uploading = false;
			}
		});
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
			{#if !readOnly}
				<input type="file" bind:this={picker} onchange={picked} hidden />
				<button type="button" disabled={uploading} onclick={() => picker?.click()}>
					{uploading ? t('write.uploading') : t('write.upload')}
				</button>
			{/if}
		</div>
		{#if downloadError}<p role="alert">{downloadError}</p>{/if}
		{#if uploadError}
			<p role="alert">{uploadError}</p>
			<button type="button" onclick={() => reloads++}>{t('files.reload')}</button>
		{/if}

		{#if indexing !== null}
			<p role="status">{t('files.indexing')} : {indexing} %</p>
			<progress max="100" value={indexing}></progress>
			<p class="note">{t('files.indexingHint')}</p>
		{:else if error}
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
							{@const k = marks.get(joinPath(dir, e.name))}
							<tr class:selected={selected === e}>
								<td>
									<button type="button" class="name" onclick={() => open(e)}>
										{e.name}{e.type === 'dir' ? '/' : ''}
									</button>
									{#if k}<abbr class="mark {k}" title={kindLabel(k)}>{kindLetter(k)}</abbr>{/if}
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
			{#if deleted.length}
				<p class="note">
					{deleted.length} {t('changes.deletedHere')} {truncateNames(deleted).shown.join(', ')}{#if truncateNames(deleted).more}, {t('changes.andMore').replace('{n}', String(truncateNames(deleted).more))}{/if} — <a href="?tab=changes">{t('container.tab.changes')}</a>
				</p>
			{/if}
		{/if}
	</section>

	<section class="preview">
		{#if file}
			<FilePreview {id} {name} path={file} entry={selected} {stopped} {readOnly} />
		{:else}
			<p>{t('files.selectFile')}</p>
		{/if}
	</section>
</div>

{#if toUpload}
	<ConfirmDialog
		title={t('confirm.upload')}
		container={`${name} (${toUpload.id.slice(0, 12)})`}
		path={joinPath(toUpload.dir, toUpload.file.name)}
		details={[
			`${t('confirm.size')} : ${formatSize(toUpload.file.size)}`,
			...(stopped ? [t('confirm.stoppedNote')] : [])
		]}
		onconfirm={upload}
		oncancel={() => (toUpload = null)}
	/>
{/if}

<style>
	.browser { display: grid; gap: 1rem; grid-template-columns: minmax(0, 1fr); }
	@media (min-width: 60rem) {
		.browser { grid-template-columns: minmax(0, 3fr) minmax(0, 2fr); }
	}
	.crumbs { display: flex; flex-wrap: wrap; gap: 0.25rem; word-break: break-all; }
	.tools { margin: 0.5rem 0; }
	.scroll { overflow-x: auto; }
	table { border-collapse: collapse; width: 100%; }
	th, td { text-align: left; padding: 0.3rem 0.5rem; border-bottom: 1px solid var(--trait-fin); white-space: nowrap; }
	button.name { background: none; border: none; padding: 0; color: inherit; text-decoration: underline; cursor: pointer; font: inherit; }
	.target { opacity: 0.7; margin-left: 0.4rem; }
	tr.selected { background: var(--jaune); color: var(--sur-jaune); }
	tr.selected .mark { color: inherit; }
	.note { font-style: italic; overflow-wrap: anywhere; }
	.mark { font-weight: 700; text-decoration: none; margin-left: 0.4rem; }
	.added { color: var(--vert-texte); }
	.modified { color: var(--ambre-texte); }
	[role='alert'] { color: var(--rouille-texte); }
</style>
