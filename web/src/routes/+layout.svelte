<script lang="ts">
	import '../app.css';
	import favicon from '#lib/assets/favicon.svg';
	import { onMount, setContext } from 'svelte';
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import { ApiError, getMeta, logout, type Meta } from '#lib/api.ts';
	import { t } from '#lib/i18n/index.ts';
	import { currentTheme, setTheme, toggled, type Theme } from '#lib/theme.ts';

	let { children } = $props();
	let meta = $state<Meta | null>(null);
	let theme = $state<Theme>('light');
	// Pages read it instead of asking /api/meta again; true until the answer
	// arrives: write buttons never show by mistake (spec §9).
	setContext('readOnly', () => meta?.readOnly ?? true);

	let section = $derived(page.url.pathname.startsWith('/servers') ? 'servers' : 'containers');

	onMount(() => {
		theme = currentTheme();
		// Without a stored choice, follow the system when it changes.
		const mq = matchMedia('(prefers-color-scheme: dark)');
		const follow = () => {
			if (!document.documentElement.dataset.theme) theme = currentTheme();
		};
		mq.addEventListener('change', follow);
		return () => mq.removeEventListener('change', follow);
	});

	function switchTheme() {
		theme = toggled(theme);
		setTheme(theme);
	}

	// Re-checks the session on every navigation; an expired session sends back to /login.
	$effect(() => {
		const path = page.url.pathname;
		if (path === '/login') {
			meta = null;
			return;
		}
		let current = true;
		getMeta()
			.then((m) => {
				if (current) meta = m;
			})
			.catch((e) => {
				if (current && e instanceof ApiError && e.status === 401) goto('/login').catch(() => {});
			});
		return () => {
			current = false;
		};
	});

	async function doLogout() {
		meta = null;
		await logout().catch(() => {});
		await goto('/login').catch(() => {});
	}
</script>

<svelte:head>
	<link rel="icon" href={favicon} />
	<title>{t('app.title')}</title>
</svelte:head>

{#if meta}
	<header class="barre">
		<a class="marque" href="/"><span class="hublot" aria-hidden="true"></span>{t('app.title')}</a>
		<p class="serveur"><span class="visuellement-cache">{t('nav.server')} </span><span class="mono">{location.host}</span></p>
		<nav aria-label={t('nav.main')}>
			<a href="/" aria-current={section === 'containers' ? 'page' : undefined}>{t('nav.containers')}</a>
			<a href="/servers" aria-current={section === 'servers' ? 'page' : undefined}>{t('nav.servers')}</a>
		</nav>
		<div class="actions">
			{#if meta.readOnly}<span class="badge">{t('nav.readOnly')}</span>{/if}
			<button
				type="button"
				class="theme"
				aria-pressed={theme === 'dark'}
				aria-label={t('nav.darkTheme')}
				title={t('nav.darkTheme')}
				onclick={switchTheme}><span class="lune" aria-hidden="true"></span></button>
			<button type="button" class="sortie" onclick={doLogout}>{t('nav.logout')}</button>
		</div>
	</header>
{/if}

{@render children()}

<style>
	.barre {
		display: grid;
		grid-template-columns: auto auto minmax(0, 1fr) auto;
		grid-template-areas: 'marque serveur nav actions';
		align-items: center;
		column-gap: var(--e5);
		min-height: 3rem;
		padding: 0 var(--gouttiere);
		border-bottom: 1px solid var(--encre);
	}
	.marque {
		grid-area: marque;
		display: inline-flex;
		align-items: center;
		gap: var(--e2);
		font-weight: 800;
		font-stretch: 75%;
		font-size: 1.0625rem;
		text-decoration: none;
		letter-spacing: 0.01em;
		white-space: nowrap;
	}
	.serveur {
		grid-area: serveur;
		margin: 0;
		color: var(--texte-doux);
		font-size: var(--t-xs);
		white-space: nowrap;
		overflow: hidden;
		text-overflow: ellipsis;
	}
	nav {
		grid-area: nav;
		display: flex;
		gap: var(--e1);
		align-self: stretch;
	}
	nav a {
		display: inline-flex;
		align-items: center;
		padding: 0 var(--e3);
		font-size: var(--t-s);
		font-weight: 600;
		text-decoration: none;
		border-bottom: 3px solid transparent;
		border-top: 3px solid transparent;
	}
	nav a:hover {
		border-bottom-color: var(--trait);
	}
	nav a[aria-current='page'] {
		border-bottom-color: var(--encre);
	}
	.actions {
		grid-area: actions;
		display: flex;
		align-items: center;
		gap: var(--e2);
	}
	.badge {
		font-size: var(--t-xs);
		font-weight: 600;
		padding: 0.125rem var(--e2);
		border: 1px solid var(--encre);
		border-radius: var(--rayon);
	}
	button {
		min-height: 2rem;
	}
	/* la sortie, un mot et non une boîte de plus */
	.sortie {
		border-color: transparent;
		padding: 0 var(--e2);
	}
	.theme {
		width: 2rem;
		padding: 0;
		justify-content: center;
	}
	/* demi-disque : clair à gauche, nuit à droite */
	.lune {
		position: relative;
		width: 0.875rem;
		height: 0.875rem;
		border: 2px solid currentColor;
		border-radius: 50%;
		overflow: hidden;
	}
	.lune::after {
		content: '';
		position: absolute;
		inset: 0 0 0 50%;
		background: currentColor;
	}

	@media (max-width: 40rem) {
		.barre {
			grid-template-columns: minmax(0, 1fr) auto;
			grid-template-areas:
				'marque actions'
				'nav serveur';
			row-gap: 0;
			padding-top: var(--e2);
		}
		nav {
			margin-left: calc(-1 * var(--e3));
			min-height: 2.5rem;
		}
		.serveur {
			text-align: right;
		}
	}
</style>
