<script module lang="ts">
	export type Panel = 'files' | 'diff' | 'logs';
</script>

<script lang="ts">
	import { page } from '$app/state';
	import type { Entry } from '#lib/api.ts';
	import { t } from '#lib/i18n/index.ts';
	import Icon from '#lib/icons/Icon.svelte';
	import { withParams } from '#lib/url.ts';
	import ChangesList from './ChangesList.svelte';
	import FileBrowser from './FileBrowser.svelte';
	import FilePreview from './FilePreview.svelte';
	import LogsView from './LogsView.svelte';

	// The bottom panels of the chosen container (spec v0.2 §3): side by side
	// from 90 rem, tabs below (the file system tab then holds the content).

	let {
		id,
		name,
		stopped,
		readOnly,
		panel
	}: { id: string; name: string; stopped: boolean; readOnly: boolean; panel: Panel } = $props();

	let file = $derived(page.url.searchParams.get('file'));
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

<section class="panels" data-panel={panel} aria-label={t('panels.label')}>
	<nav class="tabs switch" aria-label={t('panels.label')}>
		{#each TABS as p (p.id)}
			<a href={panelHref(p.id)} aria-current={panel === p.id ? 'page' : undefined}>
				<Icon name={p.icon} />
				{t(p.key)}
			</a>
		{/each}
	</nav>

	<div class="grid">
		<section class="panel p-files" class:current={panel === 'files'} aria-labelledby="p-files">
			<h2 id="p-files">{t('panels.files')}</h2>
			<div class="body">
				<FileBrowser {id} {name} {stopped} {readOnly} compact withPreview={false} diffHref={panelHref('diff')} onselect={(e) => (entry = e)} />
			</div>
		</section>
		<section class="panel p-diff" class:current={panel === 'diff'} aria-labelledby="p-diff">
			<h2 id="p-diff">{t('panels.diff')}</h2>
			<div class="body"><ChangesList {id} {fileHref} /></div>
		</section>
		<section class="panel p-content preview" class:current={panel === 'files'} aria-labelledby="p-content">
			<h2 id="p-content" class="sr-only">{t('panels.content')}</h2>
			<div class="body">
				{#if file}
					<FilePreview {id} {name} path={file} {entry} {stopped} {readOnly} />
				{:else}
					<p class="muted empty"><Icon name="file" size={20} /> {t('files.selectFile')}</p>
				{/if}
			</div>
		</section>
		<section class="panel p-logs" class:current={panel === 'logs'} aria-labelledby="p-logs">
			<h2 id="p-logs">{t('panels.logs')}</h2>
			<div class="body logs-body"><LogsView {id} /></div>
		</section>
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
		min-width: 0;
	}
	.panel {
		display: flex;
		flex-direction: column;
		min-width: 0;
		padding: 0.9rem 1rem;
	}
	h2 {
		font-size: var(--t-m);
		margin-bottom: 0.6rem;
	}
	.body {
		min-width: 0;
		min-height: 0;
		flex: 1;
	}
	.logs-body {
		display: flex;
		flex-direction: column;
	}
	.empty {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		padding: 1rem 0;
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
		.p-logs .body {
			height: min(70vh, 36rem);
		}
		.panels[data-panel='files'] .grid {
			grid-template-columns: minmax(0, 1fr) minmax(0, 1fr);
		}
	}
	@media (max-width: 64rem) {
		.panels[data-panel='files'] .grid {
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
			grid-auto-rows: 32rem;
		}
		.body {
			overflow: auto;
		}
		.logs-body {
			overflow: hidden;
		}
		.panel.current {
			border-color: var(--trait-fort);
		}
	}
</style>
