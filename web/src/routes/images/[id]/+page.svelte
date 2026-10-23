<script lang="ts">
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import { ApiError } from '#lib/api.ts';
	import { stateLabel, stateTone } from '#lib/containers.ts';
	import CopyButton from '#lib/components/CopyButton.svelte';
	import { formatSize } from '#lib/files.ts';
	import { errorMessage, lang, t, tf } from '#lib/i18n/index.ts';
	import Icon from '#lib/icons/Icon.svelte';
	import { getImage, imageName, imageShortId, layerCounts, platform, type ImageDetail } from '#lib/images.ts';
	import { commandLine, splitLabels } from '#lib/xray.ts';

	// Spec v0.2 §5.4: one image, by its full ID only (a name in the address
	// is never sent: 404). Everything shown is masked by the agent; nothing
	// is revealed here.
	let id = $derived(page.params.id ?? '');
	let img = $state<ImageDetail | null>(null);
	let error = $state('');
	let reloads = $state(0);
	let showEmpty = $state(false);

	$effect(() => {
		const want = id;
		void reloads;
		const ctrl = new AbortController();
		img = null;
		error = '';
		showEmpty = false;
		getImage(want, ctrl.signal)
			.then((d) => {
				if (!ctrl.signal.aborted) img = d;
			})
			.catch((e) => {
				if (ctrl.signal.aborted) return;
				if (e instanceof ApiError && e.status === 401) {
					goto('/login').catch(() => {});
					return;
				}
				error = errorMessage(e instanceof ApiError ? e.code : 'unknown');
			});
		return () => ctrl.abort();
	});

	let command = $derived(img ? commandLine([...img.config.entrypoint, ...img.config.cmd]) : '');
	let labels = $derived(splitLabels(img?.config.labels ?? []));
	let counts = $derived(layerCounts(img?.layers ?? []));
	let layers = $derived((img?.layers ?? []).map((l, i) => ({ ...l, i })).filter((l) => showEmpty || !l.empty));

	function when(iso: string): string {
		if (!iso) return '—';
		const d = new Date(iso);
		return Number.isNaN(d.getTime()) ? iso : d.toLocaleString(lang, { dateStyle: 'medium', timeStyle: 'short' });
	}
</script>

<div class="page">
	<a class="back" href="/images"><Icon name="arrowLeft" /> {t('images.back')}</a>

	{#if error}
		<div class="panel card">
			<p role="alert">{error}</p>
			<button type="button" class="btn small" onclick={() => reloads++}>{t('files.reload')}</button>
		</div>
	{:else if !img}
		<div class="panel card"><p class="muted" role="status">{t('images.loadingOne')}</p></div>
	{:else}
		<section class="panel card" aria-labelledby="image-name">
			<header>
				<span class="tile" aria-hidden="true"><Icon name="image" size={20} /></span>
				<div class="title">
					<h1 id="image-name" class="mono">{imageName(img)}</h1>
					{#if img.tags.length === 0}<span class="pill">{t('images.untagged')}</span>{/if}
				</div>
			</header>

			<dl class="facts">
				<dt>{t('images.id')}</dt>
				<dd class="with-copy">
					<span class="mono id" title={img.id}>{img.id}</span>
					<CopyButton text={img.id} label={t('images.copyId')} />
				</dd>
				{#if img.tags.length > 0}
					<dt>{t('images.tags')}</dt>
					<dd>
						<ul class="chips">
							{#each img.tags as tag (tag)}<li class="mono">{tag}</li>{/each}
						</ul>
					</dd>
				{/if}
				<dt>{t('images.size')}</dt>
				<dd>{formatSize(img.size)}</dd>
				<dt>{t('images.created')}</dt>
				<dd>{when(img.created)}</dd>
				<dt>{t('images.platform')}</dt>
				<dd class="mono">{platform(img) || '—'}</dd>
			</dl>
		</section>

		<section class="panel card" aria-labelledby="image-config">
			<h2 id="image-config">{t('images.config')}</h2>
			<h3>{t('images.command')}</h3>
			{#if command}
				<div class="with-copy">
					<pre class="cmd mono">{command}</pre>
					<CopyButton text={command} label={t('images.copyCommand')} />
				</div>
				{#if img.config.commandMasked}<p class="note small">{t('images.commandMasked')}</p>{/if}
			{:else}
				<p class="muted">{t('images.noCommand')}</p>
			{/if}

			<dl class="facts">
				<dt>{t('images.workingDir')}</dt>
				<dd class="mono">{img.config.workingDir || '—'}</dd>
				<dt>{t('images.user')}</dt>
				<dd class="mono">{img.config.user || '—'}</dd>
				<dt>{t('images.ports')}</dt>
				<dd class="mono">{img.config.exposedPorts.join(', ') || '—'}</dd>
			</dl>

			<h3>{t('images.env')}</h3>
			{#if img.config.env.length === 0}
				<p class="muted">{t('images.noEnv')}</p>
			{:else}
				<ul class="env">
					{#each img.config.env as v, i (i)}
						<li>
							<code class="vname">{v.name}</code>
							{#if v.masked || v.value === null}
								<span class="masked">••••••<span class="sr-only"> ({t('images.masked')})</span></span>
							{:else}
								<code class="value">{v.value}</code>
							{/if}
						</li>
					{/each}
				</ul>
				{#if img.config.env.some((v) => v.masked)}<p class="note small">{t('images.envHint')}</p>{/if}
			{/if}

			<h3>{t('images.labels')}</h3>
			{#if labels.shown.length === 0 && labels.masked.length === 0}
				<p class="muted">{t('images.noLabels')}</p>
			{/if}
			{#if labels.shown.length > 0}
				<ul class="chips">
					{#each labels.shown as l (l.key)}<li><span class="mono">{l.key}</span>=<span class="mono">{l.value}</span></li>{/each}
				</ul>
			{/if}
			{#if labels.masked.length > 0}
				{@const n = labels.masked.length}
				{@const others = labels.shown.length > 0}
				<details class="masked-labels">
					<summary>{tf(`images.labelsMasked${others ? 'Other' : ''}.${n === 1 ? 'one' : 'other'}`, { n })}</summary>
					<ul class="plain">
						{#each labels.masked as k (k)}<li class="mono">{k}</li>{/each}
					</ul>
					<p class="note small">{t('images.labelsHint')}</p>
				</details>
			{/if}
		</section>

		<section class="panel card" aria-labelledby="image-layers">
			<div class="row-head">
				<h2 id="image-layers">{t('images.layers')}</h2>
				<span class="muted small">{tf('images.layersSummary', counts)}</span>
			</div>
			<p class="note small">{t('images.layersHint')}</p>
			<ol class="layers">
				{#each layers as l (l.i)}
					<li class:empty={l.empty}>
						<pre class="mono by">{l.createdBy || '—'}</pre>
						<span class="meta small">
							{#if l.empty}<span class="muted">{t('images.layer.empty')}</span>{:else}{formatSize(l.size)}{/if}
							· {when(l.created)}
						</span>
					</li>
				{/each}
			</ol>
			{#if counts.empty > 0}
				<button type="button" class="btn small" aria-expanded={showEmpty ? 'true' : 'false'} onclick={() => (showEmpty = !showEmpty)}>
					{showEmpty ? t('images.emptyLayers.hide') : tf('images.emptyLayers.show', { n: counts.empty })}
				</button>
			{/if}
		</section>

		<section class="panel card" aria-labelledby="image-ctrs">
			<h2 id="image-ctrs">{t('images.containersTitle')}</h2>
			{#if img.containers.length === 0}
				<p class="muted">{t('images.noContainer')}</p>
			{:else}
				<ul class="ctrs">
					{#each img.containers as c (c.id)}
						<li>
							<a href="/?c={c.id}" title={t('images.openContainer')}>{c.name}</a>
							<span class="state tone-{stateTone(c.state)}"><span class="dot {stateTone(c.state)}"></span>{stateLabel(c.state)}</span>
							<span class="mono muted small">{imageShortId(c.id)}</span>
						</li>
					{/each}
				</ul>
			{/if}
		</section>
	{/if}
</div>

<style>
	.page {
		display: grid;
		gap: 0.75rem;
		min-width: 0;
	}
	.back {
		display: inline-flex;
		align-items: center;
		gap: 0.35rem;
		width: fit-content;
		color: var(--texte-doux);
		font-size: var(--t-s);
	}
	.card {
		display: grid;
		gap: 0.75rem;
		padding: 1.25rem;
		min-width: 0;
	}
	header {
		display: flex;
		align-items: center;
		gap: 0.75rem;
		min-width: 0;
	}
	.title {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: 0.25rem 0.75rem;
		min-width: 0;
	}
	h1 {
		font-size: var(--t-xl);
		overflow-wrap: anywhere;
	}
	h2 {
		font-size: var(--t-l);
	}
	h3 {
		font-size: var(--t-s);
		color: var(--texte-doux);
		font-weight: 600;
		margin-top: 0.4rem;
	}
	.tile {
		display: inline-grid;
		place-items: center;
		flex: none;
		width: 2.25rem;
		height: 2.25rem;
		border-radius: var(--rayon);
		background: var(--releve);
		border: 1px solid var(--trait);
	}
	.facts {
		display: grid;
		grid-template-columns: max-content minmax(0, 1fr);
		gap: 0.4rem 1rem;
		margin: 0;
	}
	.facts dt {
		color: var(--texte-doux);
		font-size: var(--t-s);
	}
	.facts dd {
		margin: 0;
		min-width: 0;
		overflow-wrap: anywhere;
	}
	.with-copy {
		display: flex;
		align-items: flex-start;
		gap: 0.5rem;
		min-width: 0;
	}
	.id {
		min-width: 0;
		overflow-wrap: anywhere;
		font-size: var(--t-s);
	}
	.cmd,
	.by {
		margin: 0;
		min-width: 0;
		flex: 1;
		white-space: pre-wrap;
		overflow-wrap: anywhere;
		font-size: var(--t-s);
	}
	.cmd {
		padding: 0.5rem 0.65rem;
		border: 1px solid var(--trait);
		border-radius: var(--rayon);
		background: var(--releve);
	}
	.small {
		font-size: var(--t-xs);
	}
	ul.chips,
	ul.env,
	ul.plain,
	ul.ctrs,
	ol.layers {
		list-style: none;
		margin: 0;
		padding: 0;
	}
	.chips {
		display: flex;
		flex-wrap: wrap;
		gap: 0.35rem;
	}
	.chips li {
		padding: 0.05rem 0.5rem;
		border: 1px solid var(--trait);
		border-radius: var(--rayon);
		background: var(--surface);
		font-size: var(--t-xs);
		overflow-wrap: anywhere;
	}
	.env li {
		display: flex;
		flex-wrap: wrap;
		gap: 0.4rem 0.75rem;
		align-items: baseline;
		padding: 0.3rem 0;
		border-bottom: 1px solid var(--trait);
	}
	.env li:last-child {
		border-bottom: 0;
	}
	.vname {
		font-weight: 600;
	}
	.value {
		min-width: 0;
		flex: 1;
		overflow-wrap: anywhere;
	}
	.masked {
		color: var(--texte-doux);
		letter-spacing: 0.1em;
	}
	details.masked-labels summary {
		cursor: pointer;
		color: var(--texte-doux);
		font-size: var(--t-s);
	}
	details.masked-labels[open] summary {
		margin-bottom: 0.35rem;
	}
	.plain li {
		font-size: var(--t-xs);
		overflow-wrap: anywhere;
	}
	.row-head {
		display: flex;
		flex-wrap: wrap;
		align-items: baseline;
		gap: 0.25rem 1rem;
	}
	.layers li {
		display: grid;
		gap: 0.2rem;
		padding: 0.5rem 0;
		border-bottom: 1px solid var(--trait);
	}
	.layers li:last-child {
		border-bottom: 0;
	}
	.layers li.empty .by {
		color: var(--texte-doux);
	}
	.meta {
		color: var(--texte-doux);
	}
	.ctrs li {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: 0.25rem 1rem;
		padding: 0.35rem 0;
		border-bottom: 1px solid var(--trait);
	}
	.ctrs li:last-child {
		border-bottom: 0;
	}
	.ctrs a {
		font-weight: 600;
	}
	.state {
		display: inline-flex;
		align-items: center;
		gap: 0.4rem;
		font-size: var(--t-s);
	}
	@media (max-width: 48rem) {
		.card {
			padding: 1rem 0.75rem;
		}
		.facts {
			grid-template-columns: minmax(0, 1fr);
			gap: 0.1rem;
		}
		.facts dd {
			margin-bottom: 0.4rem;
		}
	}
</style>
