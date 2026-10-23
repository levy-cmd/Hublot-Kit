<script lang="ts">
	import { page } from '$app/state';
	import { untrack } from 'svelte';
	import type { Entry } from '#lib/api.ts';
	import { t } from '#lib/i18n/index.ts';
	import Icon from '#lib/icons/Icon.svelte';
	import { diffChange, diffHref } from '#lib/contentdiff.ts';
	import { logsLive, mountedPanels, type Panel } from '#lib/panels.ts';
	import { withParams } from '#lib/url.ts';
	import ChangesList from './ChangesList.svelte';
	import FileBrowser from './FileBrowser.svelte';
	import FilePreview from './FilePreview.svelte';
	import LogsView from './LogsView.svelte';
	import UnifiedDiff from './UnifiedDiff.svelte';

	// The bottom panels of the chosen container (spec v0.2 §3): side by side
	// from 90 rem, tabs below (the file system tab then holds the content).
	// Below 90 rem a panel is mounted when first opened, never before: a
	// hidden file system panel would export a stopped container for nothing.
	// A logs panel opened, then hidden, stays mounted with its stream closed.

	let {
		id,
		name,
		stopped,
		readOnly,
		panel
	}: { id: string; name: string; stopped: boolean; readOnly: boolean; panel: Panel } = $props();

	// Known before the first render: mounting everything, then unmounting,
	// would already have sent the requests.
	const wideQuery = matchMedia('(min-width: 90rem)');
	let wide = $state(wideQuery.matches);
	$effect(() => {
		const on = () => (wide = wideQuery.matches);
		wideQuery.addEventListener('change', on);
		return () => wideQuery.removeEventListener('change', on);
	});
	let opened = $state<Panel[]>([]);
	$effect(() => {
		const p = panel;
		untrack(() => {
			if (!opened.includes(p)) opened = [...opened, p];
		});
	});
	let mounted = $derived(mountedPanels(new Set(opened), panel, wide));

	let file = $derived(page.url.searchParams.get('file'));
	// The content panel shows a file's preview, or the content diff of a
	// path of the diff panel (F17, spec v0.2 §5.6): the last one chosen (a
	// diff link drops the file; a file chosen in the explorer wins).
	let diff = $derived(file ? null : diffChange(page.url.searchParams));
	// Below 90 rem the content goes with its panel: the explorer's, or the diff's.
	let contentCurrent = $derived(panel === 'files' || (panel === 'diff' && diff !== null));
	let entry = $state<Entry | undefined>();
	const panelHref = (p: Panel) => withParams(page.url.searchParams, { panel: p });
	// A path of the diff opens in the file system panel, folder and file.
	const fileHref = (path: string) => withParams(page.url.searchParams, { panel: 'files', path, file: null });

	const TABS: { id: Panel; key: string; icon: 'folder' | 'diff' | 'logs' }[] = [
		{ id: 'files', key: 'panels.files', icon: 'folder' },
		{ id: 'diff', key: 'panels.diff', icon: 'diff' },
		{ id: 'logs', key: 'panels.logs', icon: 'logs' }
	];
</script>

<section class="panels" data-panel={panel} data-content={contentCurrent ? '' : undefined} aria-label={t('panels.label')}>
	<nav class="tabs switch" aria-label={t('panels.label')}>
		{#each TABS as p (p.id)}
			<a href={panelHref(p.id)} aria-current={panel === p.id ? 'page' : undefined}>
				<Icon name={p.icon} />
				{t(p.key)}
			</a>
		{/each}
	</nav>

	<div class="grid">
		{#if mounted.has('files')}
			<section class="panel p-files" class:current={panel === 'files'} aria-labelledby="p-files">
				<h2 id="p-files">{t('panels.files')}</h2>
				<div class="body">
					<FileBrowser {id} {name} {stopped} {readOnly} compact withPreview={false} diffHref={panelHref('diff')} onselect={(e) => (entry = e)} />
				</div>
			</section>
		{/if}
		{#if mounted.has('diff')}
			<section class="panel p-diff" class:current={panel === 'diff'} aria-labelledby="p-diff">
				<h2 id="p-diff">{t('panels.diff')}</h2>
				<div class="body">
					<ChangesList {id} {fileHref} diffHref={(c) => diffHref(page.url.searchParams, c)} selected={diff?.path} />
				</div>
			</section>
		{/if}
		{#if mounted.has('files') || (mounted.has('diff') && diff)}
			<section class="panel p-content preview" class:current={contentCurrent} aria-labelledby="p-content">
				<h2 id="p-content" class="sr-only">{t('panels.content')}</h2>
				<div class="body">
					{#if file}
						<FilePreview {id} {name} path={file} {entry} {stopped} {readOnly} />
					{:else if diff}
						{#key `${diff.kind}:${diff.path}`}
							<UnifiedDiff {id} change={diff} {stopped} fileHref={fileHref(diff.path)} />
						{/key}
					{:else}
						<p class="muted empty"><Icon name="file" size={20} /> {t('files.selectFile')}</p>
					{/if}
				</div>
			</section>
		{/if}
		{#if mounted.has('logs')}
			<section class="panel p-logs" class:current={panel === 'logs'} aria-labelledby="p-logs">
				<h2 id="p-logs">{t('panels.logs')}</h2>
				<div class="body logs-body"><LogsView {id} live={logsLive(panel, wide)} /></div>
			</section>
		{/if}
	</div>
</section>

<style>
	.panels {
		display: grid;
		gap: 0.75rem;
		min-width: 0;
	}
	.grid {
		display: grid;
		gap: 0.75rem;
		align-items: start;
		min-width: 0;
	}
	/* A panel takes the height of its content, up to the screen's; it
	   scrolls inside beyond that. The logs take a steady height. */
	.panel {
		display: flex;
		flex-direction: column;
		min-width: 0;
		max-height: min(72vh, 40rem);
		padding: 0.9rem 1rem;
	}
	.p-logs {
		height: min(62vh, 34rem);
	}
	h2 {
		font-size: var(--t-m);
		margin-bottom: 0.6rem;
	}
	.body {
		min-width: 0;
		min-height: 0;
		flex: 1 1 auto;
		overflow: auto;
	}
	.logs-body {
		display: flex;
		flex-direction: column;
		overflow: hidden;
	}
	.empty {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		padding: 0.5rem 0;
	}

	/* Under 90 rem: one panel at a time, chosen by the tabs. */
	@media (max-width: 89.99rem) {
		.panel:not(.current) {
			display: none;
		}
		.panel > h2 {
			position: absolute;
			width: 1px;
			height: 1px;
			overflow: hidden;
			clip-path: inset(50%);
			white-space: nowrap;
		}
		.panels[data-content] .grid {
			grid-template-columns: minmax(0, 1fr) minmax(0, 1fr);
		}
	}
	@media (max-width: 64rem) {
		.panels[data-content] .grid {
			grid-template-columns: minmax(0, 1fr);
		}
	}

	/* From 90 rem: the four panels side by side, as in the mock-up. */
	@media (min-width: 90rem) {
		.switch {
			display: none;
		}
		.grid {
			grid-template-columns: minmax(0, 1.15fr) minmax(0, 0.85fr) minmax(0, 1.1fr) minmax(0, 1.25fr);
		}
		.panel.current {
			border-color: var(--trait-fort);
		}
	}
</style>
