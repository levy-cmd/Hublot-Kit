<script lang="ts">
	import { getContext } from 'svelte';
	import type { Meta } from '#lib/api.ts';
	import { t } from '#lib/i18n/index.ts';
	import Icon from '#lib/icons/Icon.svelte';

	const meta = getContext<() => Meta | null>('meta');
	const host = location.host;
</script>

<div class="page">
	<header>
		<h1>{t('servers.title')}</h1>
		<p class="sub">{t('servers.lead')}</p>
	</header>
	<section class="panel server" aria-labelledby="this-server">
		<span class="ic"><Icon name="server" size={20} /></span>
		<div class="main">
			<h2 id="this-server">{t('servers.current')}</h2>
			<dl>
				<dt>{t('servers.address')}</dt>
				<dd class="mono">{host}</dd>
				{#if meta()}
					<dt>{t('servers.version')}</dt>
					<dd class="mono">{meta()?.version}</dd>
					<dt>{t('servers.docker')}</dt>
					<dd>
						<span class="dot" class:ok={meta()?.docker.connected} class:warn={!meta()?.docker.connected}></span>
						{meta()?.docker.connected ? t('shell.docker.connected') : t('shell.docker.down')}
						<span class="mono muted">{meta()?.docker.host}</span>
					</dd>
				{/if}
			</dl>
		</div>
	</section>
	<div>
		<button type="button" class="btn" disabled title={t('servers.soon')}>
			{t('servers.add')} ({t('servers.soon')})
		</button>
	</div>
</div>

<style>
	.page {
		display: grid;
		gap: 1.25rem;
		max-width: 48rem;
	}
	.sub {
		color: var(--texte-doux);
		margin-top: 0.35rem;
	}
	.server {
		display: flex;
		gap: 1rem;
		padding: 1.25rem;
		border-left: 3px solid var(--focus);
	}
	.ic {
		color: var(--emeraude-texte);
	}
	.main {
		display: grid;
		gap: 0.75rem;
		min-width: 0;
	}
	dl {
		display: grid;
		grid-template-columns: max-content minmax(0, 1fr);
		gap: 0.4rem 1.25rem;
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
		gap: 0.25rem 0.5rem;
		overflow-wrap: anywhere;
		min-width: 0;
	}
</style>
