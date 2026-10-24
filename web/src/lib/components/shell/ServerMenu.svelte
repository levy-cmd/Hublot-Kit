<script lang="ts">
	import { page } from '$app/state';
	import { t } from '#lib/i18n/index.ts';
	import Icon from '#lib/icons/Icon.svelte';

	// Hublot-Kit has no accounts, only a token: the menu is the server's
	// (spec v0.2 §3), not a user's. A disclosure, not an ARIA menu: plain
	// links and a button, reached with Tab.
	let { onlogout }: { onlogout: () => void } = $props();

	let open = $state(false);
	let root = $state<HTMLElement>();
	let toggle = $state<HTMLButtonElement>();
	const host = typeof location === 'undefined' ? '' : location.host;

	// Any navigation closes it.
	$effect(() => {
		void page.url.href;
		open = false;
	});

	function onkeydown(e: KeyboardEvent) {
		if (e.key === 'Escape' && open) {
			open = false;
			toggle?.focus();
		}
	}

	function onpointerdown(e: PointerEvent) {
		if (open && root && !root.contains(e.target as Node)) open = false;
	}
</script>

<svelte:window {onkeydown} {onpointerdown} />

<div class="menu" bind:this={root}>
	<button
		type="button"
		class="trigger"
		bind:this={toggle}
		aria-expanded={open}
		aria-controls="server-menu"
		aria-label={t('shell.menu')}
		onclick={() => (open = !open)}
	>
		<Icon name="server" size={20} />
		<span class="host mono">{host}</span>
		<Icon name="chevronDown" />
	</button>
	{#if open}
		<div class="popover" id="server-menu">
			<p class="label">{t('shell.agentAddress')}</p>
			<p class="addr mono">{host}</p>
			<ul>
				<li><a href="/servers"><Icon name="server" /> {t('nav.servers')}</a></li>
				<li><a href="/settings"><Icon name="settings" /> {t('nav.settings')}</a></li>
				<li>
					<button type="button" onclick={onlogout}><Icon name="logout" /> {t('nav.logout')}</button>
				</li>
			</ul>
		</div>
	{/if}
</div>

<style>
	.menu {
		position: relative;
	}
	.trigger {
		display: flex;
		align-items: center;
		gap: 0.45rem;
		height: 2.25rem;
		padding: 0 0.6rem;
		border: 1px solid var(--trait);
		border-radius: var(--rayon);
		background: var(--releve);
		color: var(--texte);
		cursor: pointer;
	}
	.trigger:hover {
		border-color: var(--trait-fort);
	}
	.host {
		font-size: var(--t-s);
		max-width: 11rem;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.popover {
		position: absolute;
		right: 0;
		top: calc(100% + 0.4rem);
		z-index: 30;
		min-width: 15rem;
		padding: 0.75rem;
		background: var(--surface);
		border: 1px solid var(--trait-fort);
		border-radius: var(--rayon);
		box-shadow: var(--ombre);
	}
	.label {
		color: var(--texte-doux);
		font-size: var(--t-xs);
	}
	.addr {
		font-size: var(--t-s);
		overflow-wrap: anywhere;
		padding-bottom: 0.6rem;
		margin-bottom: 0.4rem;
		border-bottom: 1px solid var(--trait);
	}
	ul {
		list-style: none;
		margin: 0;
		padding: 0;
	}
	a,
	li button {
		display: flex;
		align-items: center;
		gap: 0.6rem;
		width: 100%;
		padding: 0.45rem 0.5rem;
		border: 0;
		border-radius: var(--rayon);
		background: none;
		color: var(--texte);
		text-decoration: none;
		text-align: left;
		cursor: pointer;
	}
	a:hover,
	li button:hover {
		background: var(--releve);
	}
	@media (max-width: 48rem) {
		.host {
			display: none;
		}
	}
</style>
