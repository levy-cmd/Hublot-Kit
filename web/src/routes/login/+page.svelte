<script lang="ts">
	import { goto } from '$app/navigation';
	import { ApiError, login } from '#lib/api.ts';
	import { errorMessage, t } from '#lib/i18n/index.ts';

	let token = $state('');
	let error = $state('');
	let busy = $state(false);

	async function submit(event: SubmitEvent) {
		event.preventDefault();
		busy = true;
		error = '';
		try {
			await login(token.trim());
			await goto('/');
		} catch (err) {
			error = errorMessage(err instanceof ApiError ? err.code : 'unknown');
		} finally {
			busy = false;
		}
	}
</script>

<main class="login">
	<p class="marque"><span class="hublot" aria-hidden="true"></span>{t('app.title')}</p>
	<form onsubmit={submit}>
		<h1>{t('login.title')}</h1>
		<label for="token">{t('login.token')}</label>
		<input id="token" type="password" autocomplete="current-password" aria-describedby="token-hint" bind:value={token} required />
		<p class="hint" id="token-hint">{t('login.hint')}</p>
		<button type="submit" class="primaire" disabled={busy}>{t('login.submit')}</button>
		{#if error}<p role="alert">{error}</p>{/if}
	</form>
	<p class="host mono">{location.host}</p>
</main>

<style>
	.login {
		display: grid;
		grid-template-rows: auto 1fr auto;
		min-height: 100vh;
		padding: var(--e5) var(--gouttiere);
	}
	.marque {
		display: inline-flex;
		align-items: center;
		gap: var(--e2);
		margin: 0;
		font-weight: 800;
		font-stretch: 75%;
		font-size: 1.0625rem;
	}
	form {
		align-self: center;
		display: grid;
		gap: var(--e2);
		width: 100%;
		max-width: 22rem;
		margin: var(--e7) 0;
		margin-left: clamp(0rem, 12vw, 10rem);
		padding-left: var(--e5);
		border-left: 4px solid var(--encre);
	}
	h1 {
		font-size: 2.5rem;
		font-stretch: 75%;
		font-weight: 800;
		margin-bottom: var(--e5);
	}
	label {
		font-size: var(--t-s);
		font-weight: 600;
	}
	input {
		width: 100%;
		min-height: 2.75rem;
		font-family: var(--mono);
	}
	.hint {
		font-size: var(--t-xs);
		color: var(--texte-doux);
		margin: 0 0 var(--e3);
	}
	button {
		justify-self: start;
		min-height: 2.75rem;
		padding: 0 var(--e5);
	}
	[role='alert'] {
		margin: var(--e2) 0 0;
		font-size: var(--t-s);
	}
	.host {
		margin: 0;
		font-size: var(--t-xs);
		color: var(--texte-doux);
	}
</style>
