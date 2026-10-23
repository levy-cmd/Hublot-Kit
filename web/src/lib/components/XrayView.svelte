<script lang="ts">
	import { goto } from '$app/navigation';
	import { onDestroy } from 'svelte';
	import { ApiError, revealEnv, type Radiography } from '#lib/api.ts';
	import { stateLabel } from '#lib/containers.ts';
	import { errorMessage, t } from '#lib/i18n/index.ts';
	import {
		addReveal, askFor, commandLine, envValue, forgetReveals, mountLabel, noReveals, removeReveal, restartLabel,
		revealedFor, shortId
	} from '#lib/xray.ts';

	// The radiography (F7), in two parts since v0.2 §3: "details" (identity,
	// state, command, networks, mounts) under the Overview's "Details", and
	// "env" in the Secrets tab. onreload: fetch the radiography again.
	let { radio, part, onreload }: { radio: Radiography; part: 'details' | 'env'; onreload: () => void } = $props();

	// A revealed value lives here only, tied to the container's ID: never in
	// localStorage, the URL or the console. Hide forgets it, and so do
	// leaving the tab, another container, and pagehide (no return from the
	// back-forward cache with values shown).
	let reveals = $state.raw(noReveals);
	let shown = $derived(revealedFor(reveals, radio.id));
	let busy = $state(''); // the variable being revealed
	// Unmount: a reveal still pending lands nowhere.
	onDestroy(() => (reveals = forgetReveals(reveals)));
	let error = $state('');

	async function reveal(name: string) {
		if (busy) return;
		busy = name;
		error = '';
		const asked = askFor(reveals, radio.id); // before the await
		try {
			const value = await revealEnv(asked.id, name);
			reveals = addReveal(reveals, asked, radio.id, name, value);
		} catch (e) {
			if (e instanceof ApiError && e.status === 401) {
				goto('/login').catch(() => {});
				return;
			}
			error = errorMessage(e instanceof ApiError ? e.code : 'unknown');
		} finally {
			busy = '';
		}
	}
</script>

<svelte:window onpagehide={() => (reveals = forgetReveals(reveals))} />

{#if part === 'env'}
	<section class="xray">
		<h3>{t('xray.env')}</h3>
		<p class="note">{t('xray.envHint')}</p>
		{#if error}
			<p role="alert">{error}</p>
			<button type="button" class="btn small" onclick={onreload}>{t('files.reload')}</button>
		{/if}
		{#if radio.env.length === 0}
			<p class="muted">{t('xray.none')}</p>
		{:else}
			<ul class="env">
				{#each radio.env as v, i (i)}
					{@const value = envValue(v, shown)}
					<li>
						<code class="name">{v.name}</code>
						{#if value === null}
							<span class="masked">••••••<span class="sr-only"> ({t('xray.masked')})</span></span>
							<button type="button" class="btn small" disabled={busy !== ''} onclick={() => reveal(v.name)}>{t('xray.reveal')}</button>
						{:else}
							<code class="value">{value}</code>
							{#if v.masked}<button type="button" class="btn small" onclick={() => (reveals = removeReveal(reveals, v.name))}>{t('xray.hide')}</button>{/if}
						{/if}
					</li>
				{/each}
			</ul>
		{/if}
	</section>
{:else}
	<section class="xray">
		<h3>{t('xray.identity')}</h3>
		<dl>
			<dt>{t('xray.imageId')}</dt><dd><code title={radio.imageId}>{shortId(radio.imageId)}</code></dd>
			<dt>{t('xray.created')}</dt><dd>{radio.created}</dd>
		</dl>

		<h3>{t('xray.state')}</h3>
		<dl>
			<dt>{t('xray.state')}</dt><dd>{stateLabel(radio.state.status)}</dd>
			{#if !radio.state.running}<dt>{t('xray.exitCode')}</dt><dd>{radio.state.exitCode}</dd>{/if}
			{#if radio.state.error}<dt>{t('xray.error')}</dt><dd class="warn">{radio.state.error}</dd>{/if}
			{#if radio.state.startedAt}<dt>{t('xray.startedAt')}</dt><dd>{radio.state.startedAt}</dd>{/if}
			{#if !radio.state.running && radio.state.finishedAt}<dt>{t('xray.finishedAt')}</dt><dd>{radio.state.finishedAt}</dd>{/if}
			{#if radio.state.oomKilled}<dt>{t('xray.oom')}</dt><dd class="warn">{t('xray.yes')}</dd>{/if}
			{#if radio.state.health}<dt>{t('xray.health')}</dt><dd>{radio.state.health}</dd>{/if}
			<dt>{t('xray.restarts')}</dt><dd>{restartLabel(radio.restartPolicy, radio.restartCount)}</dd>
		</dl>

		<h3>{t('xray.command')}</h3>
		<dl>
			<dt>{t('xray.entrypoint')}</dt><dd><code>{commandLine(radio.entrypoint) || t('xray.none')}</code></dd>
			<dt>{t('xray.cmd')}</dt><dd><code>{commandLine(radio.cmd) || t('xray.none')}</code></dd>
			{#if radio.workingDir}<dt>{t('xray.workingDir')}</dt><dd><code>{radio.workingDir}</code></dd>{/if}
			{#if radio.user}<dt>{t('xray.user')}</dt><dd><code>{radio.user}</code></dd>{/if}
			<dt>{t('xray.tty')}</dt><dd>{radio.tty ? t('xray.yes') : t('xray.no')}</dd>
			<dt>{t('xray.logDriver')}</dt><dd>{radio.logDriver}</dd>
		</dl>
		{#if radio.commandMasked}<p class="note">{t('xray.commandMasked')}</p>{/if}

		<h3>{t('xray.networks')}</h3>
		{#if radio.networkMode}<p>{t('xray.networkMode')} <code>{radio.networkMode}</code></p>{/if}
		{#if radio.networks.length > 0}
			<ul>
				{#each radio.networks as n (n.name)}
					<li>
						<code>{n.name}</code>
						{#if n.ip}<span class="part">{n.ip}</span>{/if}
						{#if n.gateway}<span class="part">{t('xray.gateway')} {n.gateway}</span>{/if}
						{#if n.mac}<span class="part">{n.mac}</span>{/if}
						{#if n.aliases?.length}<span class="part">{n.aliases.join(', ')}</span>{/if}
					</li>
				{/each}
			</ul>
		{:else if !radio.networkMode}
			<p class="muted">{t('xray.none')}</p>
		{/if}

		<h3>{t('xray.mounts')}</h3>
		{#if radio.mounts.length === 0 && radio.tmpfs.length === 0}<p class="muted">{t('xray.none')}</p>{:else}
			<ul>
				{#each radio.mounts as m, i (i)}
					<li><span class="kind">{m.type}</span> <code>{mountLabel(m)}</code>{#if !m.rw}{` (${t('xray.readOnly')})`}{/if}</li>
				{/each}
				{#each radio.tmpfs as d (d)}
					<li><span class="kind">{t('xray.tmpfs')}</span> <code>{d}</code></li>
				{/each}
			</ul>
		{/if}
	</section>
{/if}

<style>
	.xray {
		display: grid;
		gap: 0.5rem;
	}
	h3 {
		margin-top: 0.75rem;
		font-size: var(--t-s);
		color: var(--texte-doux);
		font-weight: 600;
	}
	h3:first-child {
		margin-top: 0;
	}
	dl {
		display: grid;
		grid-template-columns: minmax(0, 11rem) minmax(0, 1fr);
		gap: 0.3rem 1rem;
		margin: 0;
	}
	dt {
		color: var(--texte-doux);
	}
	dd {
		margin: 0;
		overflow-wrap: anywhere;
	}
	ul {
		list-style: none;
		padding: 0;
		margin: 0;
	}
	li {
		padding: 0.2rem 0;
		overflow-wrap: anywhere;
	}
	code {
		overflow-wrap: anywhere;
	}
	.part {
		margin-left: 0.6rem;
		color: var(--texte-doux);
	}
	.env li {
		display: flex;
		flex-wrap: wrap;
		gap: 0.4rem 0.75rem;
		align-items: center;
		padding: 0.45rem 0;
		border-bottom: 1px solid var(--trait);
	}
	.env .name {
		font-weight: 600;
	}
	.env .value {
		min-width: 0;
		flex: 1;
	}
	.masked {
		color: var(--texte-doux);
		flex: 1;
		letter-spacing: 0.1em;
	}
	.kind {
		display: inline-block;
		min-width: 4rem;
		color: var(--texte-doux);
	}
	.warn {
		color: var(--ambre-texte);
	}
	@media (max-width: 30rem) {
		dl {
			grid-template-columns: minmax(0, 1fr);
		}
	}
</style>
