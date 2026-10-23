<script module lang="ts">
	export type DetailTab = 'overview' | 'logs' | 'diff' | 'files' | 'secrets';
</script>

<script lang="ts">
	import { goto } from '$app/navigation';
	import { ApiError, getContainer, type Radiography } from '#lib/api.ts';
	import { stateLabel, stateTone } from '#lib/containers.ts';
	import { errorMessage, lang, t, tf } from '#lib/i18n/index.ts';
	import Icon from '#lib/icons/Icon.svelte';
	import { commandLine, isStopped, portLabel } from '#lib/xray.ts';
	import ChangesList from './ChangesList.svelte';
	import CopyButton from './CopyButton.svelte';
	import FileBrowser from './FileBrowser.svelte';
	import LogsView from './LogsView.svelte';
	import XrayView from './XrayView.svelte';

	// The detail of a container (spec v0.2 §3). "side": the right panel of
	// the list, whose logs, diff and files are the bottom panels. "page": the
	// full page /c/[id] (links, phone), with every tab.

	let {
		id,
		mode,
		tab,
		readOnly,
		tabHref,
		panelHref,
		closeHref,
		fileHref
	}: {
		id: string;
		mode: 'side' | 'page';
		tab: DetailTab;
		readOnly: boolean;
		tabHref: (tab: DetailTab) => string;
		panelHref?: (panel: 'files' | 'diff' | 'logs') => string;
		closeHref?: string;
		fileHref?: (path: string) => string;
	} = $props();

	let radio = $state<Radiography | null>(null);
	let error = $state('');
	let reloads = $state(0);

	// GET /api/containers/{id} (F7): secrets already masked by the agent.
	$effect(() => {
		const wanted = id;
		void reloads;
		let current = true;
		error = '';
		radio = null;
		getContainer(wanted)
			.then((c) => {
				if (current) radio = c;
			})
			.catch((e) => {
				if (!current) return;
				if (e instanceof ApiError && e.status === 401) {
					goto('/login').catch(() => {});
					return;
				}
				error = errorMessage(e instanceof ApiError ? e.code : 'unknown');
			});
		return () => {
			current = false;
		};
	});

	const TABS: DetailTab[] = ['overview', 'logs', 'diff', 'files', 'secrets'];
	let tabs = $derived(mode === 'page' ? TABS : (['overview', 'secrets'] as DetailTab[]));
	let shownTab = $derived(tabs.includes(tab) ? tab : 'overview');
	let tone = $derived(radio ? stateTone(radio.state.status, radio.state.exitCode) : 'idle');
	// The command as the radiography shows it: masked by the agent. Copying
	// copies this form, never a revealed value.
	let command = $derived(radio ? commandLine([...radio.entrypoint, ...radio.cmd]) : '');
	// Three lines at most, "Show all" when it is longer (the mock-up's block).
	let cmdEl = $state<HTMLElement>();
	let cmdOpen = $state(false);
	let cmdLong = $state(false);
	$effect(() => {
		void command;
		const el = cmdEl;
		if (!el || cmdOpen) return;
		cmdLong = el.scrollHeight > el.clientHeight + 1;
	});

	function when(iso: string): string {
		const d = new Date(iso);
		return Number.isNaN(d.getTime()) ? iso : d.toLocaleString(lang, { dateStyle: 'medium', timeStyle: 'short' });
	}
</script>

<section class="detail" class:page={mode === 'page'} aria-labelledby="detail-name">
	{#if error}
		<p role="alert">{error}</p>
		<button type="button" class="btn small" onclick={() => reloads++}>{t('files.reload')}</button>
	{:else if !radio}
		<p class="muted" role="status">{t('container.loading')}</p>
	{:else}
		<header>
			<span class="tile tone-{tone}"><Icon name="container" size={20} /></span>
			<div class="title">
				{#if mode === 'page'}
					<h1 id="detail-name">{radio.name}</h1>
				{:else}
					<h2 id="detail-name">{radio.name}</h2>
				{/if}
				<span class="pill {tone}">{stateLabel(radio.state.status)}{#if !radio.state.running && radio.state.exitCode !== 0}{tf('container.exitCode', { n: radio.state.exitCode })}{/if}</span>
			</div>
			{#if mode === 'side'}
				<div class="head-actions">
					<a class="icon-btn small" href={`/c/${radio.id}`} aria-label={t('container.fullPage')} title={t('container.fullPage')}>
						<Icon name="expand" />
					</a>
					{#if closeHref}
						<a class="icon-btn small" href={closeHref} aria-label={t('container.close')} title={t('container.close')}>
							<Icon name="close" />
						</a>
					{/if}
				</div>
			{/if}
		</header>

		<dl class="facts">
			<dt>{t('container.image')}</dt>
			<dd class="image">
				<span class="mono">{radio.image}</span>
				<CopyButton text={radio.image} label={t('container.copyImage')} />
			</dd>
			<dt>{t('container.created')}</dt>
			<dd>{when(radio.created)}</dd>
		</dl>

		<nav class="tabs" aria-label={t('container.tabs')}>
			{#each tabs as k (k)}
				<a href={tabHref(k)} aria-current={shownTab === k ? 'page' : undefined}>{t(`container.tab.${k}`)}</a>
			{/each}
		</nav>

		<div class="tab-body">
			{#if shownTab === 'overview'}
				<div class="block">
					<span class="block-icon"><Icon name="terminal" /></span>
					<div class="block-main">
						<h3>{t('container.command')}</h3>
						{#if command}
							<code class="command" class:clamped={!cmdOpen} id="detail-command" bind:this={cmdEl}>{command}</code>
							{#if cmdLong}
								<button
									type="button"
									class="more"
									aria-expanded={cmdOpen}
									aria-controls="detail-command"
									onclick={() => (cmdOpen = !cmdOpen)}>{cmdOpen ? t('container.showLess') : t('container.showAll')}</button
								>
							{/if}
						{:else}<p class="muted">{t('container.noCommand')}</p>{/if}
						{#if radio.commandMasked}<p class="note">{t('xray.commandMasked')}</p>{/if}
					</div>
					{#if command}<CopyButton text={command} label={t('container.copyCommand')} />{/if}
				</div>
				<div class="block">
					<span class="block-icon"><Icon name="port" /></span>
					<div class="block-main">
						<h3>{t('container.ports')}</h3>
						{#if radio.ports.length === 0}
							<p class="muted">{t('container.noPorts')}</p>
						{:else}
							<ul class="ports">
								{#each radio.ports as p, i (i)}
									<li class="mono">{portLabel(p, t('xray.portAuto'))}{#if !p.ip && !p.public}&nbsp;<span class="muted">({t('xray.exposedOnly')})</span>{/if}</li>
								{/each}
							</ul>
						{/if}
					</div>
				</div>
				<details>
					<summary>{t('container.details')}</summary>
					<XrayView {radio} part="details" onreload={() => reloads++} />
				</details>
			{:else if shownTab === 'secrets'}
				<XrayView {radio} part="env" onreload={() => reloads++} />
			{:else if shownTab === 'logs'}
				<div class="fill"><LogsView id={radio.id} /></div>
			{:else if shownTab === 'diff'}
				<ChangesList id={radio.id} fileHref={fileHref ?? ((p) => `?tab=files&path=${encodeURIComponent(p)}`)} />
			{:else if shownTab === 'files'}
				<FileBrowser id={radio.id} name={radio.name} {readOnly} stopped={isStopped(radio.state)} diffHref={tabHref('diff')} />
			{/if}
		</div>

		{#if mode === 'side' && panelHref}
			<div class="open">
				<a class="btn primary" href={panelHref('files')}><Icon name="folder" /> {t('container.openFiles')}</a>
				<a class="btn" href={panelHref('diff')}><Icon name="diff" /> {t('container.openDiff')}</a>
				<a class="btn" href={panelHref('logs')}><Icon name="logs" /> {t('container.openLogs')}</a>
			</div>
		{/if}
	{/if}
</section>

<style>
	.detail {
		display: grid;
		gap: 1rem;
		align-content: start;
		min-width: 0;
	}
	header {
		display: flex;
		align-items: center;
		gap: 0.75rem;
		min-width: 0;
	}
	.tile {
		display: inline-grid;
		place-items: center;
		width: 2.5rem;
		height: 2.5rem;
		border-radius: var(--rayon);
		background: var(--releve);
		border: 1px solid var(--trait);
		flex: none;
	}
	.title {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: 0.25rem 0.75rem;
		min-width: 0;
		flex: 1;
	}
	h1,
	h2 {
		overflow-wrap: anywhere;
	}
	h2 {
		font-size: var(--t-xl);
	}
	.head-actions {
		display: flex;
		gap: 0.15rem;
		align-self: flex-start;
	}
	.facts {
		display: grid;
		grid-template-columns: max-content minmax(0, 1fr);
		gap: 0.4rem 1.25rem;
		margin: 0;
	}
	.facts dt {
		color: var(--texte-doux);
	}
	.facts dd {
		margin: 0;
		min-width: 0;
	}
	.image {
		display: flex;
		align-items: center;
		gap: 0.25rem;
	}
	.image .mono {
		overflow-wrap: anywhere;
	}
	.tab-body {
		display: grid;
		gap: 0.75rem;
		min-width: 0;
	}
	.block {
		display: flex;
		gap: 0.75rem;
		align-items: flex-start;
		padding: 0.75rem;
		background: var(--releve);
		border-radius: var(--rayon);
	}
	.block-icon {
		display: inline-grid;
		place-items: center;
		width: 2rem;
		height: 2rem;
		border-radius: var(--rayon);
		background: var(--surface);
		color: var(--emeraude-texte);
		flex: none;
	}
	.block-main {
		flex: 1;
		min-width: 0;
		display: grid;
		gap: 0.25rem;
		justify-items: start;
	}
	.command.clamped {
		display: -webkit-box;
		-webkit-box-orient: vertical;
		-webkit-line-clamp: 3;
		line-clamp: 3;
		overflow: hidden;
	}
	.more {
		border: 0;
		padding: 0;
		background: none;
		color: var(--emeraude-texte);
		font: inherit;
		font-size: var(--t-s);
		font-weight: 600;
		cursor: pointer;
	}
	.more:hover {
		text-decoration: underline;
	}
	.command {
		display: block;
		font-size: var(--t-s);
		overflow-wrap: anywhere;
		white-space: pre-wrap;
	}
	.ports {
		list-style: none;
		margin: 0;
		padding: 0;
		font-size: var(--t-s);
	}
	details {
		border: 1px solid var(--trait);
		border-radius: var(--rayon);
		padding: 0.5rem 0.75rem;
	}
	summary {
		cursor: pointer;
		font-weight: 600;
		padding: 0.15rem 0;
	}
	details[open] summary {
		margin-bottom: 0.75rem;
	}
	.open {
		display: flex;
		flex-wrap: wrap;
		gap: 0.5rem;
	}
	.open .btn {
		flex: 1 1 auto;
	}
	.fill {
		height: min(70vh, 40rem);
		display: grid;
	}
</style>
