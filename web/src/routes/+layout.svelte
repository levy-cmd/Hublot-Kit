<script lang="ts">
	import favicon from '#lib/assets/favicon.svg';
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import { ApiError, getMeta, logout, type Meta } from '#lib/api.ts';
	import { t } from '#lib/i18n/index.ts';

	let { children } = $props();
	let meta = $state<Meta | null>(null);

	// Re-checks the session on every navigation; an expired session sends back to /login.
	$effect(() => {
		const path = page.url.pathname;
		if (path === '/login') {
			meta = null;
			return;
		}
		getMeta()
			.then((m) => (meta = m))
			.catch((e) => {
				if (e instanceof ApiError && e.status === 401) goto('/login');
			});
	});

	async function doLogout() {
		await logout().catch(() => {});
		goto('/login');
	}
</script>

<svelte:head>
	<link rel="icon" href={favicon} />
	<title>{t('app.title')}</title>
</svelte:head>

{#if meta}
	<nav>
		<strong>{t('app.title')}</strong>
		<a href="/">{t('nav.containers')}</a>
		<a href="/servers">{t('nav.servers')}</a>
		{#if meta.readOnly}<span class="badge">{t('nav.readOnly')}</span>{/if}
		<button type="button" onclick={doLogout}>{t('nav.logout')}</button>
	</nav>
{/if}

{@render children()}

<style>
	nav { display: flex; gap: 1rem; align-items: center; padding: 0.75rem 1rem; border-bottom: 1px solid #ddd; flex-wrap: wrap; }
	nav button { margin-left: auto; }
	.badge { font-size: 0.8rem; padding: 0.1rem 0.4rem; border: 1px solid currentColor; border-radius: 0.25rem; }
</style>
