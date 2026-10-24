<script lang="ts">
	import type { Meta } from '#lib/api.ts';
	import { t, tf } from '#lib/i18n/index.ts';
	import Icon from '#lib/icons/Icon.svelte';
	import Ring from '#lib/icons/Ring.svelte';
	import type { Theme } from '#lib/theme.ts';
	import ServerMenu from './ServerMenu.svelte';
	import CommandPalette from '../CommandPalette.svelte';
	import { openPalette } from '#lib/palette.ts';

	let {
		meta,
		theme,
		navOpen,
		ontheme,
		onnav,
		onlogout
	}: {
		meta: Meta;
		theme: Theme;
		navOpen: boolean;
		ontheme: () => void;
		onnav: () => void;
		onlogout: () => void;
	} = $props();

	let dockerTitle = $derived(
		tf('shell.docker.title', { version: meta.docker.serverVersion || '?', host: meta.docker.host || '?' })
	);
	let roText = $derived(meta.readOnly ? t('shell.readOnly') : t('shell.writable'));
	// The shortcut as the keyboard spells it (spec v0.2 §3).
	const mac = typeof navigator !== 'undefined' && /Mac|iPhone|iPad/.test(navigator.platform || navigator.userAgent);
	const shortcut = mac ? '⌘ K' : 'Ctrl K';
</script>

<header class="topbar">
	<button
		type="button"
		class="icon-btn drawer"
		aria-controls="sidebar"
		aria-expanded={navOpen}
		aria-label={navOpen ? t('shell.navClose') : t('shell.navOpen')}
		onclick={onnav}
	>
		<Icon name={navOpen ? 'close' : 'menu'} size={20} />
	</button>
	<a class="brand" href="/">
		<Ring />
		<span class="name">{t('app.title')}</span>
		<span class="version mono" title={tf('shell.version', { v: meta.version })}>{meta.version}</span>
	</a>

	<!-- The field only opens the palette: it searches nothing by itself (spec v0.2 §3). -->
	<div class="center">
		<button type="button" class="search" aria-label={t('palette.field')} aria-haspopup="dialog" aria-keyshortcuts={mac ? 'Meta+K' : 'Control+K'} onclick={openPalette}>
			<Icon name="search" size={16} />
			<span class="search-text">{t('palette.field')}</span>
			<kbd>{shortcut}</kbd>
		</button>
	</div>
	<CommandPalette />

	<div class="right">
		<span class="docker" title={dockerTitle}>
			<span class="dot" class:ok={meta.docker.connected} class:warn={!meta.docker.connected}></span>
			<span class="docker-text">
				<span class="docker-state">{meta.docker.connected ? t('shell.docker.connected') : t('shell.docker.down')}</span>
				<span class="docker-host mono">{meta.docker.host}</span>
			</span>
		</span>
		<span class="ro" class:on={meta.readOnly} title={roText}>
			<Icon name="shield" size={20} filled={meta.readOnly} />
			<span class="ro-text">{roText}</span>
		</span>
		<button
			type="button"
			class="icon-btn"
			aria-pressed={theme === 'light'}
			aria-label={t('shell.theme.light')}
			title={t('shell.theme.light')}
			onclick={ontheme}
		>
			<Icon name={theme === 'light' ? 'moon' : 'sun'} size={20} />
		</button>
		<ServerMenu {onlogout} />
	</div>
</header>

<style>
	.topbar {
		position: sticky;
		top: 0;
		z-index: 20;
		display: flex;
		align-items: center;
		gap: 0.75rem;
		height: var(--barre);
		padding: 0 1rem;
		background: var(--surface);
		border-bottom: 1px solid var(--trait);
	}
	.brand {
		display: flex;
		align-items: center;
		gap: 0.6rem;
		color: var(--texte);
		text-decoration: none;
		min-width: 0;
	}
	.name {
		font-size: var(--t-l);
		font-weight: 700;
		font-stretch: 87.5%;
		letter-spacing: -0.01em;
	}
	.version {
		font-size: var(--t-xs);
		color: var(--texte-doux);
		padding: 0.05rem 0.4rem;
		border: 1px solid var(--trait);
		border-radius: var(--rayon);
		max-width: 9rem;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.center {
		flex: 1;
		display: flex;
		justify-content: center;
		min-width: 0;
	}
	.search {
		display: flex;
		align-items: center;
		gap: 0.6rem;
		width: min(34rem, 100%);
		min-width: 0;
		height: 2.25rem;
		padding: 0 0.5rem 0 0.75rem;
		background: var(--fond);
		color: var(--texte-doux);
		border: 1px solid var(--trait-fort);
		border-radius: var(--rayon);
		font: inherit;
		font-size: var(--t-m);
		cursor: text;
		text-align: left;
	}
	.search:hover {
		color: var(--texte);
	}
	.search-text {
		flex: 1;
		min-width: 0;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	kbd {
		padding: 0.1rem 0.4rem;
		border: 1px solid var(--trait);
		border-radius: 4px;
		font-size: var(--t-xs);
		color: var(--texte-doux);
		white-space: nowrap;
	}
	.right {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		min-width: 0;
	}
	.docker {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		margin-right: 0.5rem;
		min-width: 0;
	}
	.docker-text {
		display: grid;
		line-height: 1.2;
		min-width: 0;
	}
	.docker-state {
		font-size: var(--t-s);
		font-weight: 600;
	}
	.docker-host {
		font-size: var(--t-xs);
		color: var(--texte-doux);
		max-width: 16rem;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.ro {
		display: flex;
		align-items: center;
		gap: 0.4rem;
		padding: 0 0.5rem;
		color: var(--texte-doux);
		font-size: var(--t-s);
	}
	.ro.on {
		color: var(--emeraude-texte);
	}
	.drawer {
		display: none;
	}
	/* Hidden from sight on narrow screens, still read by screen readers
	   (the state is always text, spec v0.2 §3). */
	@media (max-width: 80rem) {
		.ro-text {
			position: absolute;
			width: 1px;
			height: 1px;
			overflow: hidden;
			clip-path: inset(50%);
			white-space: nowrap;
		}
		.docker-host {
			display: none;
		}
	}
	@media (max-width: 48rem) {
		.topbar {
			padding: 0 0.5rem;
			gap: 0.25rem;
		}
		.drawer {
			display: inline-grid;
		}
		.version {
			display: none;
		}
		.search {
			width: 2.25rem;
			padding: 0;
			justify-content: center;
		}
		.search-text,
		.search kbd {
			display: none;
		}
		.docker-text {
			position: absolute;
			width: 1px;
			height: 1px;
			overflow: hidden;
			clip-path: inset(50%);
			white-space: nowrap;
		}
		.docker {
			margin: 0 0.25rem;
		}
	}
</style>
