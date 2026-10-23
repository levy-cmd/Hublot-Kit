<script lang="ts">
	import { page } from '$app/state';
	import { t, tf } from '#lib/i18n/index.ts';
	import Icon from '#lib/icons/Icon.svelte';
	import Ring from '#lib/icons/Ring.svelte';
	import { currentNav, nav, navHref } from '#lib/nav.ts';
	import { openPalette } from '#lib/palette.ts';
	import ReadOnlyNotice from './ReadOnlyNotice.svelte';

	let { readOnly, version, open }: { readOnly: boolean; version: string; open: boolean } = $props();

	// The chosen container: ?c= on the list, or the full page /c/<id>.
	let chosen = $derived(
		page.url.searchParams.get('c') ?? (page.url.pathname.startsWith('/c/') ? (page.params.id ?? null) : null)
	);

	// The current page (nav.ts): a panel shortcut never is.
	let current = $derived(currentNav(page.url));
</script>

<nav id="sidebar" class="sidebar" class:open aria-label={t('shell.nav')}>
	<ul>
		{#each nav as item (item.id)}
			{@const n = item.count?.() ?? null}
			<li>
				{#if item.action === 'palette'}
					<!-- "Search" opens the palette, which gives the focus back here. -->
					<button type="button" class="entry" title={t(item.labelKey)} aria-haspopup="dialog" onclick={openPalette}>
						<Icon name={item.icon} size={20} />
						<span class="label">{t(item.labelKey)}</span>
					</button>
				{:else}
					<a
						class="entry"
						href={navHref(item, chosen)}
						aria-current={current === item.id ? 'page' : undefined}
						title={t(item.labelKey)}
					>
						<Icon name={item.icon} size={20} />
						<span class="label">{t(item.labelKey)}</span>
						{#if n !== null}<span class="count">{n}</span>{/if}
					</a>
				{/if}
			</li>
		{/each}
	</ul>

	<div class="foot">
		<div class="notice"><ReadOnlyNotice {readOnly} /></div>
		<div class="brand">
			<Ring size={24} />
			<div>
				<p class="name">{t('app.title')}</p>
				<p class="meta">{t('shell.tagline')}</p>
				<p class="meta mono">{tf('shell.version', { v: version })}</p>
			</div>
		</div>
	</div>
</nav>

<style>
	.sidebar {
		position: sticky;
		top: var(--barre);
		height: calc(100vh - var(--barre));
		height: calc(100dvh - var(--barre));
		display: flex;
		flex-direction: column;
		justify-content: space-between;
		gap: 1rem;
		padding: 0.75rem;
		background: var(--surface);
		border-right: 1px solid var(--trait);
		overflow-y: auto;
	}
	ul {
		list-style: none;
		margin: 0;
		padding: 0;
		display: grid;
		gap: 0.15rem;
	}
	.entry {
		position: relative;
		display: flex;
		align-items: center;
		gap: 0.75rem;
		padding: 0.55rem 0.7rem;
		border-radius: var(--rayon);
		color: var(--texte-doux);
		text-decoration: none;
		font-weight: 550;
		transition: background-color var(--vite), color var(--vite);
	}
	.entry:hover {
		background: var(--releve);
		color: var(--texte);
	}
	button.entry {
		width: 100%;
		border: 0;
		background: none;
		font: inherit;
		font-weight: 550;
		text-align: left;
		cursor: pointer;
	}
	a[aria-current='page'] {
		background: var(--selection);
		color: var(--emeraude-texte);
	}
	a[aria-current='page']::before {
		content: '';
		position: absolute;
		left: -0.75rem;
		top: 0.35rem;
		bottom: 0.35rem;
		width: 3px;
		border-radius: 0 3px 3px 0;
		background: var(--focus);
	}
	.label {
		flex: 1;
		white-space: nowrap;
	}
	.foot {
		display: grid;
		gap: 1rem;
	}
	.brand {
		display: flex;
		gap: 0.6rem;
		align-items: flex-start;
		padding: 0.25rem 0.25rem 0;
	}
	.name {
		font-weight: 700;
		font-stretch: 87.5%;
	}
	.meta {
		color: var(--texte-doux);
		font-size: var(--t-xs);
	}

	/* Icons only between 48 and 80 rem (spec v0.2 §3). */
	@media (max-width: 80rem) and (min-width: 48.01rem) {
		.sidebar {
			padding: 0.75rem 0.5rem;
			align-items: center;
		}
		.entry {
			justify-content: center;
			padding: 0.6rem;
		}
		a[aria-current='page']::before {
			left: -0.5rem;
		}
		.label,
		.count {
			position: absolute;
			width: 1px;
			height: 1px;
			overflow: hidden;
			clip-path: inset(50%);
			white-space: nowrap;
		}
		.notice,
		.brand div {
			display: none;
		}
	}

	/* A drawer under 48 rem, opened from the top bar. */
	@media (max-width: 48rem) {
		.sidebar {
			position: fixed;
			z-index: 15;
			left: 0;
			width: min(18rem, 85vw);
			transform: translateX(-100%);
			visibility: hidden;
			transition: transform var(--vite), visibility var(--vite);
			box-shadow: var(--ombre);
		}
		.sidebar.open {
			transform: none;
			visibility: visible;
		}
	}
</style>
