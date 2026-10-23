<script lang="ts">
	import { getContext } from 'svelte';
	import type { Meta } from '#lib/api.ts';
	import { lang, t } from '#lib/i18n/index.ts';
	import type { Theme } from '#lib/theme.ts';

	// Spec v0.2 §3: the browser's preferences, then the agent's settings, read
	// only: they are start options (§8), never written from the interface.
	const meta = getContext<() => Meta | null>('meta');
	const theme = getContext<{ get: () => Theme; set: (t: Theme) => void }>('theme');
	const yesNo = (b: boolean | undefined) => (b ? t('settings.yes') : t('settings.no'));
</script>

<div class="page">
	<h1>{t('settings.title')}</h1>

	<section class="panel" aria-labelledby="prefs">
		<h2 id="prefs">{t('settings.prefs')}</h2>
		<dl>
			<dt>{t('settings.language')}</dt>
			<dd>{t(`settings.lang.${lang}`)} <span class="muted">{t('settings.languageHint')}</span></dd>
		</dl>
		<fieldset>
			<legend>{t('settings.theme')}</legend>
			<div class="choice">
				{#each ['dark', 'light'] as const as k (k)}
					<label class:on={theme.get() === k}>
						<input type="radio" name="theme" value={k} checked={theme.get() === k} onchange={() => theme.set(k)} />
						{k === 'dark' ? t('settings.dark') : t('settings.light')}
					</label>
				{/each}
			</div>
			<p class="note">{t('settings.themeHint')}</p>
		</fieldset>
	</section>

	{#if meta()}
		{@const m = meta()!}
		<section class="panel" aria-labelledby="agent">
			<h2 id="agent">{t('settings.agent')}</h2>
			<dl>
				<dt>{t('settings.version')}</dt>
				<dd class="mono">{m.version}</dd>
				<dt>{t('settings.readOnly')}</dt>
				<dd>{yesNo(m.readOnly)} <span class="muted">{m.readOnly ? t('shell.notice.readOnly') : t('shell.notice.writable')}</span></dd>
			</dl>
		</section>
		<section class="panel" aria-labelledby="docker">
			<h2 id="docker">{t('settings.docker')}</h2>
			<dl>
				<dt>{t('settings.dockerState')}</dt>
				<dd>
					<span class="dot" class:ok={m.docker.connected} class:warn={!m.docker.connected}></span>
					{m.docker.connected ? t('shell.docker.connected') : t('shell.docker.down')}
				</dd>
				<dt>{t('settings.dockerHost')}</dt>
				<dd class="mono">{m.docker.host}</dd>
				<dt>{t('settings.dockerVersion')}</dt>
				<dd class="mono">{m.docker.serverVersion || t('settings.unknown')}</dd>
				<dt>{t('settings.apiVersion')}</dt>
				<dd class="mono">{m.docker.apiVersion || t('settings.unknown')}</dd>
			</dl>
			<p class="note">{t('settings.startupHint')}</p>
		</section>
	{/if}
</div>

<style>
	.page {
		display: grid;
		gap: 1.25rem;
		max-width: 48rem;
	}
	section {
		display: grid;
		gap: 1rem;
		padding: 1.25rem;
	}
	dl {
		display: grid;
		grid-template-columns: max-content minmax(0, 1fr);
		gap: 0.5rem 1.5rem;
		margin: 0;
	}
	dt {
		color: var(--texte-doux);
	}
	dd {
		margin: 0;
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: 0.25rem 0.6rem;
		overflow-wrap: anywhere;
		min-width: 0;
	}
	fieldset {
		border: 0;
		margin: 0;
		padding: 0;
		display: grid;
		gap: 0.5rem;
	}
	legend {
		color: var(--texte-doux);
		padding: 0;
		margin-bottom: 0.5rem;
	}
	.choice {
		display: inline-flex;
		gap: 0.5rem;
		flex-wrap: wrap;
	}
	.choice label {
		display: inline-flex;
		align-items: center;
		gap: 0.5rem;
		padding: 0.45rem 0.9rem;
		border: 1px solid var(--trait-fort);
		border-radius: var(--rayon);
		cursor: pointer;
	}
	.choice label.on {
		background: var(--selection);
		border-color: var(--focus);
		color: var(--emeraude-texte);
	}
	.choice input {
		accent-color: var(--emeraude);
		margin: 0;
	}
	@media (max-width: 30rem) {
		dl {
			grid-template-columns: minmax(0, 1fr);
			gap: 0.15rem;
		}
		dd {
			margin-bottom: 0.5rem;
		}
	}
</style>
