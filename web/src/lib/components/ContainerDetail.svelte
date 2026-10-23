<script module lang="ts">
	export type DetailTab = 'overview' | 'logs' | 'diff' | 'files' | 'secrets';
</script>

<script lang="ts">
	import { goto } from '$app/navigation';
	import { ApiError, getContainer, getFigures, type Figures, type Radiography } from '#lib/api.ts';
	import { stateLabel, stateTone } from '#lib/containers.ts';
	import { formatSize } from '#lib/files.ts';
	import { errorMessage, lang, t, tf } from '#lib/i18n/index.ts';
	import Icon from '#lib/icons/Icon.svelte';
	import { commandLine, isStopped, lastExit, portLabel, splitLabels } from '#lib/xray.ts';
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

	// Spec v0.2 §5.3: the key figures come apart, and never hold the detail
	// back ("—" while they load, or if they fail: up to 30 s while dockerd
	// exports the container).
	// Asked by the full ID of the radiography, never by the URL's (a /c/<short
	// ID> link): the agent recognises an export in progress by the full ID
	// only (review of tranche 3).
	let figures = $state<Figures | null>(null);
	let fullID = $derived(radio?.id ?? null);
	$effect(() => {
		const wanted = fullID;
		void reloads;
		figures = null;
		if (!wanted) return;
		let current = true;
		const abort = new AbortController(); // another container: the request goes
		getFigures(wanted, abort.signal)
			.then((f) => {
				if (current) figures = f;
			})
			.catch(() => {}); // "—": the figures are a bonus, the detail tells the errors
		return () => {
			current = false;
			abort.abort();
		};
	});
	const size = (n: number | null | undefined) => (n == null ? '—' : formatSize(n));

	const TABS: DetailTab[] = ['overview', 'logs', 'diff', 'files', 'secrets'];
	let tabs = $derived(mode === 'page' ? TABS : (['overview', 'secrets'] as DetailTab[]));
	let shownTab = $derived(tabs.includes(tab) ? tab : 'overview');
	let tone = $derived(radio ? stateTone(radio.state.status, radio.state.exitCode) : 'idle');
	// The command as the radiography shows it: masked by the agent. Copying
	// copies this form, never a revealed value.
	let command = $derived(radio ? commandLine([...radio.entrypoint, ...radio.cmd]) : '');
	let exit = $derived(radio ? lastExit(radio.state) : null);
	// Chips for the labels in clear; the masked ones (often thirty of an
	// image's) are keys only, behind one folded line.
	let labels = $derived(splitLabels(radio?.labels ?? []));
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
			<dt>{t('container.lastExit')}</dt>
			<dd>
				{#if exit}{when(exit.at)}{#if exit.code}{tf('container.exitCode', { n: exit.code })}{/if}{:else}{t('container.never')}{/if}
			</dd>
		</dl>

		<dl class="figures" aria-label={t('container.figures')}>
			<div class="figure">
				<dt>{t('container.figure.files')}</dt>
				<dd
					title={figures?.files
						? tf('container.figure.filesAt', { at: when(figures.files.at) })
						: t('container.figure.filesNone')}
				>
					{figures?.files ? figures.files.count.toLocaleString(lang) : '—'}
				</dd>
			</div>
			<div class="figure">
				<dt>{t('container.figure.imageSize')}</dt>
				<dd>{size(figures?.imageSize)}</dd>
			</div>
			<div class="figure">
				<dt>{t('container.figure.size')}</dt>
				<dd title={t('container.figure.sizeHint')}>
					{size(figures?.sizeRw)}
					{#if figures?.sizeRootFs != null}<span class="sub">{tf('container.figure.total', { size: size(figures.sizeRootFs) })}</span>{/if}
				</dd>
			</div>
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
				<div class="block">
					<span class="block-icon"><Icon name="info" /></span>
					<div class="block-main">
						<h3>{t('container.labels')}</h3>
						{#if labels.shown.length === 0 && labels.masked.length === 0}
							<p class="muted">{t('container.noLabels')}</p>
						{/if}
						{#if labels.shown.length > 0}
							<ul class="chips">
								{#each labels.shown as l (l.key)}
									<li class="chip">
										<span class="k mono">{l.key}</span>
										{#if l.value}<span class="v mono">{l.value}</span>{/if}
									</li>
								{/each}
							</ul>
						{/if}
						{#if labels.masked.length > 0}
							{@const n = labels.masked.length}
							{@const others = labels.shown.length > 0}
							<details class="masked-labels">
								<summary title={t('container.labelMaskedHint')}>
									{tf(`container.labelsMasked${others ? 'Other' : ''}.${n === 1 ? 'one' : 'other'}`, { n })}
								</summary>
								<p class="note">{t('container.labelMaskedHint')}</p>
								<ul class="keys">
									{#each labels.masked as k (k)}<li class="mono">{k}</li>{/each}
								</ul>
							</details>
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
	.figures {
		display: grid;
		grid-template-columns: repeat(3, minmax(0, 1fr));
		gap: 0.5rem;
		margin: 0;
	}
	.figure {
		display: grid;
		align-content: start;
		gap: 0.2rem;
		padding: 0.6rem 0.7rem;
		background: var(--releve);
		border-radius: var(--rayon);
		min-width: 0;
	}
	.figure dt {
		font-size: var(--t-xs);
		color: var(--texte-doux);
	}
	.figure dd {
		margin: 0;
		font-size: var(--t-l);
		font-weight: 600;
		font-variant-numeric: tabular-nums;
		overflow-wrap: anywhere;
	}
	.figure .sub {
		display: block;
		font-size: var(--t-xs);
		font-weight: 400;
		color: var(--texte-doux);
	}
	.chips {
		list-style: none;
		margin: 0;
		padding: 0;
		display: flex;
		flex-wrap: wrap;
		gap: 0.35rem;
		font-size: var(--t-xs);
	}
	.chip {
		display: inline-flex;
		flex-wrap: wrap;
		max-width: 100%;
		border: 1px solid var(--trait);
		border-radius: var(--rayon);
		background: var(--surface);
		overflow: hidden;
	}
	.chip .k,
	.chip .v {
		padding: 0.15rem 0.4rem;
		overflow-wrap: anywhere;
		min-width: 0;
	}
	.chip .k {
		color: var(--texte-doux);
	}
	.chip .v {
		border-left: 1px solid var(--trait);
		color: var(--texte);
	}
	details.masked-labels {
		border: 0;
		padding: 0;
		width: 100%;
		font-size: var(--t-s);
	}
	details.masked-labels summary {
		font-weight: 500;
		color: var(--texte-doux);
	}
	details.masked-labels[open] summary {
		margin-bottom: 0.4rem;
	}
	.keys {
		list-style: none;
		margin: 0.4rem 0 0;
		padding: 0;
		display: grid;
		gap: 0.15rem;
		font-size: var(--t-xs);
		color: var(--texte-doux);
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
