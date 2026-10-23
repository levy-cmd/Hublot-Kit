<script lang="ts">
	import { goto } from '$app/navigation';
	import { ApiError, getContainer, previewFile, type Change } from '#lib/api.ts';
	import { kindLabel } from '#lib/changes.ts';
	import {
		mountDiff,
		needs,
		reasonMessage,
		sideOf,
		sides,
		type DiffInput,
		type NoDiff,
		type Side
	} from '#lib/contentdiff.ts';
	import { baseName, fsErrorMessage, imageFsErrorMessage, indexingPercent, pollDelay, pollIndexing, previewImageFile } from '#lib/files.ts';
	import { t } from '#lib/i18n/index.ts';
	import Icon from '#lib/icons/Icon.svelte';
	import { imageHex } from '#lib/images.ts';

	// F17 (spec v0.2 §5.6): the unified diff of a path of F5, the image's
	// version against the container's, read only. Each side is the preview
	// the explorer would show (F2, v0.2 §5.5), nothing more; its text goes
	// into CodeMirror as a document, never as HTML. CodeMirror and the merge
	// extension load with the first diff, apart from the initial bundle.
	let {
		id,
		change,
		stopped = false,
		fileHref
	}: { id: string; change: Change; stopped?: boolean; fileHref?: string } = $props();

	type Loading = { image: number | null; container: number | null };
	let result = $state<DiffInput | NoDiff | null>(null);
	let progress = $state<Loading>({ image: null, container: null });
	let error = $state('');
	let reloads = $state(0);

	$effect(() => {
		const wantedId = id;
		const c = change;
		const fromStopped = stopped;
		void reloads;
		const want = needs(c.kind);
		const got: { image: Side; container: Side } = { image: null, container: null };
		let left = Number(want.image) + Number(want.container);
		let current = true;
		const stops: (() => void)[] = [];
		result = null;
		error = '';
		progress = { image: null, container: null };

		const fail = (e: unknown, message: (code: string, detail?: string) => string) => {
			if (!current) return;
			stops.forEach((s) => s());
			if (e instanceof ApiError && e.status === 401) {
				goto('/login').catch(() => {});
				return;
			}
			error = e instanceof ApiError ? message(e.code, e.detail) : message('unknown');
		};
		const settle = (side: 'image' | 'container', s: Side) => {
			if (!current || error) return;
			got[side] = s;
			progress = { ...progress, [side]: null };
			if (--left === 0) result = sides(c, got.image, got.container);
		};
		const ask = (side: 'image' | 'container', run: () => Promise<Side>, delay: number, message: (code: string, detail?: string) => string) => {
			stops.push(
				pollIndexing(run, {
					indexing: (p) => {
						if (current) progress = { ...progress, [side]: indexingPercent(p) };
					},
					result: (s) => settle(side, s),
					error: (e) => {
						const s = sideOf(e);
						if (s) settle(side, s);
						else fail(e, message);
					}
				}, delay)
			);
		};

		if (want.container)
			ask('container', () => previewFile(wantedId, c.path), 1000, (code, detail) =>
				code === 'too_large' ? t('files.tooLarge') : fsErrorMessage(code, detail, fromStopped)
			);
		if (want.image) {
			// The image of the container by its ID (an untagged image too), never by name.
			getContainer(wantedId).then(
				(r) => {
					if (!current) return;
					const hex = imageHex(r.imageId);
					if (!hex) return fail(new ApiError(404, 'not_found'), (code) => imageFsErrorMessage(code, true));
					ask('image', () => previewImageFile(hex, c.path), pollDelay, (code) => imageFsErrorMessage(code, true));
				},
				(e) => fail(e, (code) => imageFsErrorMessage(code, true))
			);
		}
		return () => {
			current = false;
			stops.forEach((s) => s());
		};
	});

	let input = $derived(result && 'kind' in result ? result : null);
	let noDiff = $derived(result && 'reason' in result ? result : null);
	let host = $state<HTMLDivElement>();
	let failed = $state(false);

	$effect(() => {
		const target = host;
		const d = input;
		if (!target || !d || (d.kind === 'modified' && d.equal === 'bytes')) return;
		let current = true;
		let destroy = () => {};
		failed = false;
		mountDiff(target, d, `${t('diff.title')} ${change.path}`)
			.then((remove) => {
				if (current) destroy = remove;
				else remove();
			})
			.catch(() => {
				if (current) failed = true;
			});
		return () => {
			current = false;
			destroy();
		};
	});

	const loading = $derived(progress.image !== null || progress.container !== null);
</script>

<header>
	<span class="ficon {change.kind}"><Icon name="diff" /></span>
	<div class="title">
		<h3>{baseName(change.path)} <span class="badge {change.kind}">{kindLabel(change.kind)}</span></h3>
		<p class="path mono">{change.path}</p>
	</div>
	{#if fileHref && change.kind !== 'deleted'}
		<a class="icon-btn small" href={fileHref} aria-label={t('diff.openFile')} title={t('diff.openFile')}><Icon name="folder" /></a>
	{/if}
</header>

{#if error}
	<p role="alert">{error}</p>
	<button type="button" class="btn small" onclick={() => reloads++}>{t('files.reload')}</button>
{:else if loading}
	{#if progress.image !== null}
		<p role="status">{t('diff.loadingImage')} : {progress.image} %</p>
		<progress max="100" value={progress.image}></progress>
	{/if}
	{#if progress.container !== null}
		<p role="status">{t('diff.loadingContainer')} : {progress.container} %</p>
		<progress max="100" value={progress.container}></progress>
	{/if}
	<p class="note">{t('diff.loadingHint')}</p>
{:else if !result}
	<p class="muted">{t('files.loading')}</p>
{:else if noDiff}
	<p class="muted reason">{reasonMessage(noDiff)}</p>
{:else if input}
	{#if input.kind === 'modified' && input.equal}
		<p class="muted">{t(`diff.equal.${input.equal}`)}</p>
	{:else}
		<p class="note">{t(`diff.legend.${input.kind}`)}</p>
	{/if}
	{#if failed}
		<p role="alert">{t('error.unknown')}</p>
	{/if}
	{#if !(input.kind === 'modified' && input.equal === 'bytes')}
		<div class="diff" bind:this={host}></div>
	{/if}
{/if}

<style>
	header {
		display: flex;
		align-items: flex-start;
		gap: 0.6rem;
		min-width: 0;
		margin-bottom: 0.5rem;
	}
	.ficon {
		color: var(--texte-doux);
		padding-top: 0.15rem;
	}
	.ficon.added {
		color: var(--emeraude-texte);
	}
	.ficon.modified {
		color: var(--ambre-texte);
	}
	.ficon.deleted {
		color: var(--rouge-texte);
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
	.badge {
		display: inline-block;
		margin-left: 0.3rem;
		padding: 0.05rem 0.5rem;
		border-radius: 999px;
		font-size: var(--t-xs);
		font-weight: 600;
		vertical-align: middle;
	}
	.badge.added {
		background: var(--diff-ajout);
		color: var(--emeraude-texte);
	}
	.badge.modified {
		background: var(--releve);
		color: var(--ambre-texte);
	}
	.badge.deleted {
		background: var(--diff-suppr);
		color: var(--rouge-texte);
	}
	.diff {
		max-height: 70vh;
		overflow: auto;
		margin-top: 0.5rem;
		border-radius: var(--rayon);
		background: var(--releve);
	}
	.diff :global(.cm-editor) {
		min-width: 0;
	}
	progress {
		width: 100%;
	}
</style>
