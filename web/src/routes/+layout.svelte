<script lang="ts">
	import '../app.css';
	import favicon from '#lib/assets/favicon.svg';
	import { setContext } from 'svelte';
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import { ApiError, getMeta, logout, type Meta } from '#lib/api.ts';
	import { containerList } from '#lib/containerList.svelte.ts';
	import { errorMessage, t } from '#lib/i18n/index.ts';
	import { currentTheme, setTheme, toggled, type Theme } from '#lib/theme.ts';
	import Sidebar from '#lib/components/shell/Sidebar.svelte';
	import TopBar from '#lib/components/shell/TopBar.svelte';

	let { children } = $props();
	let meta = $state<Meta | null>(null);
	let theme = $state<Theme>(currentTheme());
	let navOpen = $state(false);
	// Pages read it instead of asking /api/meta again; true until the answer
	// arrives: write buttons never show by mistake (spec §9).
	setContext('readOnly', () => meta?.readOnly ?? true);
	setContext('meta', () => meta);
	setContext('theme', {
		get: () => theme,
		set: (next: Theme) => {
			theme = next;
			setTheme(next);
		}
	});

	let metaError = $state('');
	let metaReloads = $state(0);

	// Any navigation closes the drawer.
	$effect(() => {
		void page.url.href;
		navOpen = false;
	});

	// Re-checks the session on every navigation; an expired session sends back to /login.
	$effect(() => {
		const path = page.url.pathname;
		void metaReloads;
		if (path === '/login') {
			meta = null;
			return;
		}
		let current = true;
		getMeta()
			.then((m) => {
				if (!current) return;
				meta = m;
				metaError = '';
				if (containerList.items === null && !containerList.loading) void containerList.load();
			})
			.catch((e) => {
				if (!current) return;
				if (e instanceof ApiError && e.status === 401) goto('/login').catch(() => {});
				else if (!meta) metaError = errorMessage(e instanceof ApiError ? e.code : 'unknown');
			});
		return () => {
			current = false;
		};
	});

	function toggleTheme() {
		theme = toggled(theme);
		setTheme(theme);
	}

	async function doLogout() {
		meta = null;
		containerList.clear();
		await logout().catch(() => {});
		await goto('/login').catch(() => {});
	}
</script>

<svelte:window
	onkeydown={(e) => {
		if (e.key === 'Escape' && navOpen) navOpen = false;
	}}
/>

<svelte:head>
	<link rel="icon" href={favicon} />
	<title>{t('app.title')}</title>
</svelte:head>

{#if page.url.pathname === '/login'}
	{@render children()}
{:else if meta}
	<a class="skip" href="#main">{t('shell.skip')}</a>
	<div class="app">
		<TopBar {meta} {theme} {navOpen} ontheme={toggleTheme} onnav={() => (navOpen = !navOpen)} onlogout={doLogout} />
		<div class="body">
			<Sidebar readOnly={meta.readOnly} version={meta.version} open={navOpen} />
			{#if navOpen}
				<button type="button" class="scrim" aria-label={t('shell.navClose')} onclick={() => (navOpen = false)}></button>
			{/if}
			<main id="main" tabindex="-1">
				{@render children()}
			</main>
		</div>
	</div>
{:else if metaError}
	<div class="down">
		<p role="alert">{metaError}</p>
		<button type="button" class="btn" onclick={() => metaReloads++}>{t('files.reload')}</button>
	</div>
{/if}

<style>
	.app {
		min-height: 100vh;
		min-height: 100dvh;
	}
	.body {
		display: grid;
		grid-template-columns: var(--lateral) minmax(0, 1fr);
	}
	main {
		min-width: 0;
		padding: 1.5rem 1.75rem 2rem;
		outline: none;
	}
	.skip {
		position: absolute;
		left: 0.75rem;
		top: -3rem;
		z-index: 40;
		padding: 0.5rem 0.75rem;
		background: var(--emeraude);
		color: var(--sur-emeraude);
		border-radius: var(--rayon);
		font-weight: 600;
	}
	.skip:focus {
		top: 0.6rem;
	}
	.scrim {
		display: none;
	}
	.down {
		display: grid;
		gap: 1rem;
		justify-items: start;
		max-width: 32rem;
		margin: 4rem auto;
		padding: 0 1rem;
	}
	@media (max-width: 80rem) {
		.body {
			grid-template-columns: 4.25rem minmax(0, 1fr);
		}
		main {
			padding: 1.25rem 1.25rem 2rem;
		}
	}
	@media (max-width: 48rem) {
		.body {
			grid-template-columns: minmax(0, 1fr);
		}
		main {
			padding: 1rem 1rem 2rem;
		}
		.scrim {
			display: block;
			position: fixed;
			inset: var(--barre) 0 0 0;
			z-index: 14;
			border: 0;
			background: rgb(0 0 0 / 0.45);
		}
	}
</style>
