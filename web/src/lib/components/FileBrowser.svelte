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
	import { errorMessage, lang, t, tf } from '#lib/i18n/index.ts';
	import { latest, lock } from '#lib/guard.ts';
	import Icon from '#lib/icons/Icon.svelte';
	import { withParams } from '#lib/url.ts';
	import { UPLOAD_MAX, uploadNameOk, writeErrorMessage } from '#lib/write.ts';
	import ConfirmDialog from '#lib/components/ConfirmDialog.svelte';
	import FilePreview from '#lib/components/FilePreview.svelte';

	// compact: the narrow bottom panel (name and date only); withPreview false:
	// the preview is shown by another panel, told of the selected entry by onselect.
	let {
		id,
		name,
		stopped = false,
		readOnly = true,
		compact = false,
		withPreview = true,
		diffHref,
		onselect
	}: {
		id: string;
		name: string;
		stopped?: boolean;
		readOnly?: boolean;
		compact?: boolean;
		withPreview?: boolean;
		diffHref: string;
		onselect?: (entry: Entry | undefined) => void;
	} = $props();

	// The URL carries the state (?path=dir&file=selected): back button and links work.
	let dir = $derived(page.url.searchParams.get('path') || '/');
	let file = $derived(page.url.searchParams.get('file'));
	let listing = $state<Listing | null>(null);
	let error = $state('');
	let indexing = $state<number | null>(null); // percent while the index is built
	let loading = $state(false);
	let downloadError = $state('');
	let reloads = $state(0); // bumped by the Reload button: same URL, new request
	// Explore: every entry; Diff: only what changed since the image (and the
	// folders that hold a change). F5 marks in both, as in v0.1.
	let mode = $state<'explore' | 'diff'>('explore');
	let changes = $state<Change[]>([]);
	let marks = $derived(changeMarks(changes));
	let changedDirs = $derived.by(() => {
		const dirs: Record<string, true> = {};
		for (const c of changes) {
			for (let p = parentPath(c.path); p !== '/' && !dirs[p]; p = parentPath(p)) dirs[p] = true;
		}
		return dirs;
	});
	let entries = $derived.by(() => {
		const all = listing ? sortEntries(listing.entries) : [];
		if (mode === 'explore') return all;
		return all.filter((e) => {
			const p = joinPath(dir, e.name);
			return marks.has(p) || changedDirs[p] === true;
		});
	});
	let deleted = $derived(deletedIn(changes, dir));
	let selected = $derived(file ? entries.find((e) => joinPath(dir, e.name) === file) : undefined);
	$effect(() => {
		onselect?.(selected);
	});

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

	// Other parameters of the page (container, panel, tab) are kept.
	const href = (path: string, selectedFile?: string) =>
		withParams(page.url.searchParams, { path, file: selectedFile ?? null });

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

	// The narrow panel takes the short form (23/10/26 11:56).
	const date = (iso: string) =>
		new Date(iso).toLocaleString(
			lang,
			compact
				? { day: '2-digit', month: '2-digit', year: '2-digit', hour: '2-digit', minute: '2-digit' }
				: { day: 'numeric', month: 'short', year: 'numeric', hour: '2-digit', minute: '2-digit' }
		);
	const iconOf = (e: Entry) => (e.type === 'dir' ? 'folder' : e.type === 'symlink' ? 'link' : 'file');
</script>

<div class="browser" class:compact class:alone={!withPreview}>
	<section class="listing">
		<div class="bar">
			<div class="seg" role="group" aria-label={t('files.mode')} title={t('files.modeHint')}>
				<button type="button" aria-pressed={mode === 'explore'} onclick={() => (mode = 'explore')}>{t('files.mode.explore')}</button>
				<button type="button" aria-pressed={mode === 'diff'} onclick={() => (mode = 'diff')}>{t('files.mode.diff')}</button>
			</div>
			<div class="tools">
				<button type="button" class="icon-btn small" onclick={downloadDir} aria-label={t('files.downloadDir')} title={t('files.downloadDir')}>
					<Icon name="download" />
				</button>
				{#if !readOnly}
					<input type="file" bind:this={picker} onchange={picked} hidden />
					<button
						type="button"
						class="icon-btn small"
						disabled={uploading}
						onclick={() => picker?.click()}
						aria-label={uploading ? t('write.uploading') : t('write.upload')}
						title={uploading ? t('write.uploading') : t('write.upload')}
					>
						<Icon name="upload" />
					</button>
				{/if}
			</div>
		</div>
		<nav class="crumbs mono" aria-label={t('files.path')}>
			{#each breadcrumbs(dir) as c, i (c.path)}
				{#if i > 1}<span aria-hidden="true">/</span>{/if}
				<a href={href(c.path)}>{c.name}</a>
			{/each}
		</nav>
		{#if downloadError}<p role="alert">{downloadError}</p>{/if}
		{#if uploadError}
			<p role="alert">{uploadError}</p>
			<button type="button" class="btn small" onclick={() => reloads++}>{t('files.reload')}</button>
		{/if}

		{#if indexing !== null}
			<div class="state">
				<p role="status">{t('files.indexing')} : {indexing} %</p>
				<progress max="100" value={indexing}></progress>
				<p class="note">{t('files.indexingHint')}</p>
			</div>
		{:else if error}
			<div class="state">
				<p role="alert">{error}</p>
				<button type="button" class="btn small" onclick={() => reloads++}>{t('files.reload')}</button>
			</div>
		{:else if loading && !listing}
			<p class="muted state">{t('files.loading')}</p>
		{:else if listing}
			{#if listing.truncated}<p class="note">{t('files.truncated')}</p>{/if}
			<div class="scroll">
				<table>
					<thead>
						<tr>
							<th scope="col">{t('files.col.name')}</th>
							{#if !compact}<th scope="col" class="wide">{t('files.col.size')}</th>
							<th scope="col" class="wide">{t('files.col.mode')}</th>
							<th scope="col" class="wide">{t('files.col.owner')}</th>{/if}
							<th scope="col">{t('files.col.modified')}</th>
						</tr>
					</thead>
					<tbody>
						{#if dir !== '/'}
							<tr>
								<td colspan={compact ? 2 : 5}><a class="up" href={href(parentPath(dir))}>.. <span class="muted">({t('files.parent')})</span></a></td>
							</tr>
						{/if}
						{#each entries as e, i (i)}
							{@const k = marks.get(joinPath(dir, e.name))}
							<tr class:selected={selected === e}>
								<td>
									<div class="name-cell">
										<span class="ficon {e.type}"><Icon name={iconOf(e)} /></span>
										<button type="button" class="name" title={e.name} onclick={() => open(e)}>
											{e.name}{e.type === 'dir' ? '/' : ''}
										</button>
										{#if k}<abbr class="mark {k}" title={kindLabel(k)}>{kindLetter(k)}</abbr>{/if}
										{#if e.type === 'symlink' && !compact}<span class="target mono">→ {e.linkTarget}</span>{/if}
									</div>
								</td>
								{#if !compact}<td class="wide mono">{e.type === 'dir' ? '' : formatSize(e.size)}</td>
								<td class="wide"><code>{formatMode(e.type, e.mode)}</code></td>
								<td class="wide mono">{e.uid}:{e.gid}</td>{/if}
								<td class="date">{date(e.modTime)}</td>
							</tr>
						{/each}
					</tbody>
				</table>
			</div>
			{#if entries.length === 0}<p class="muted">{mode === 'diff' ? t('changes.empty') : t('files.empty')}</p>{/if}
			{#if deleted.length}
				{@const d = truncateNames(deleted)}
				<p class="note">
					{deleted.length} {t('changes.deletedHere')} {d.shown.join(', ')}{#if d.more}, {tf('changes.andMore', { n: d.more })}{/if}
					<a href={diffHref}>{t('changes.seeAll')}</a>
				</p>
			{/if}
		{/if}
	</section>

	{#if withPreview}
		<section class="preview">
			{#if file}
				<FilePreview {id} {name} path={file} entry={selected} {stopped} {readOnly} />
			{:else}
				<p class="muted">{t('files.selectFile')}</p>
			{/if}
		</section>
	{/if}
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
	.browser {
		display: grid;
		gap: 1rem;
		grid-template-columns: minmax(0, 1fr);
		min-width: 0;
	}
	@media (min-width: 64rem) {
		.browser:not(.alone):not(.compact) {
			grid-template-columns: minmax(0, 3fr) minmax(0, 2fr);
		}
	}
	.listing {
		display: grid;
		gap: 0.5rem;
		align-content: start;
		min-width: 0;
	}
	.bar {
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: 0.5rem;
	}
	.seg {
		display: inline-flex;
		border: 1px solid var(--trait-fort);
		border-radius: var(--rayon);
		overflow: hidden;
	}
	.seg button {
		border: 0;
		background: none;
		color: var(--texte-doux);
		padding: 0.2rem 0.65rem;
		font-size: var(--t-s);
		font-weight: 600;
		cursor: pointer;
	}
	.seg button + button {
		border-left: 1px solid var(--trait-fort);
	}
	.seg button[aria-pressed='true'] {
		background: var(--selection);
		color: var(--emeraude-texte);
	}
	.tools {
		display: flex;
		gap: 0.15rem;
	}
	.crumbs {
		display: flex;
		flex-wrap: wrap;
		gap: 0.25rem;
		font-size: var(--t-s);
		overflow-wrap: anywhere;
		color: var(--texte-doux);
		padding: 0.3rem 0.5rem;
		background: var(--releve);
		border-radius: var(--rayon);
	}
	.crumbs a {
		color: var(--texte);
		text-decoration: none;
	}
	.crumbs a:hover {
		text-decoration: underline;
	}
	.state {
		display: grid;
		gap: 0.5rem;
		justify-items: start;
		padding: 0.5rem 0;
	}
	.scroll {
		overflow-x: auto;
	}
	table {
		border-collapse: collapse;
		width: 100%;
	}
	th {
		text-align: left;
		font-size: var(--t-xs);
		font-weight: 600;
		color: var(--texte-doux);
		padding: 0.3rem 0.5rem;
		border-bottom: 1px solid var(--trait);
		white-space: nowrap;
	}
	td {
		padding: 0.25rem 0.5rem;
		border-bottom: 1px solid var(--trait);
		white-space: nowrap;
		font-size: var(--t-s);
	}
	tr.selected {
		background: var(--selection);
	}
	tr.selected td:first-child {
		box-shadow: inset 3px 0 0 var(--focus);
	}
	.name-cell {
		display: flex;
		align-items: center;
		gap: 0.4rem;
		min-width: 0;
	}
	.name-cell .name {
		overflow: hidden;
		text-overflow: ellipsis;
		min-width: 0;
	}
	.compact table {
		table-layout: fixed;
	}
	.compact th:last-child {
		width: 7.25rem;
	}
	.compact td {
		overflow: hidden;
	}
	.ficon {
		color: var(--texte-doux);
	}
	.ficon.dir {
		color: var(--emeraude-texte);
	}
	button.name {
		background: none;
		border: none;
		padding: 0;
		color: var(--texte);
		cursor: pointer;
		font: inherit;
		text-align: left;
	}
	button.name:hover {
		text-decoration: underline;
	}
	.up {
		color: var(--texte);
		text-decoration: none;
	}
	.target {
		color: var(--texte-doux);
		font-size: var(--t-xs);
	}
	.date {
		color: var(--texte-doux);
	}
	.mark {
		display: inline-grid;
		place-items: center;
		width: 1.1rem;
		height: 1.1rem;
		border-radius: 3px;
		font-size: 0.6875rem;
		font-weight: 700;
		text-decoration: none;
	}
	.mark.added {
		background: var(--diff-ajout);
		color: var(--emeraude-texte);
	}
	.mark.modified {
		background: var(--releve);
		color: var(--ambre-texte);
	}
	.mark.deleted {
		background: var(--diff-suppr);
		color: var(--rouge-texte);
	}
	.note {
		overflow-wrap: anywhere;
	}
	.compact .wide {
		display: none;
	}
	.compact .date {
		text-align: right;
	}
	@media (max-width: 48rem) {
		.wide {
			display: none;
		}
	}
</style>
