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
	<h1>{t('login.title')}</h1>
	<form onsubmit={submit}>
		<label for="token">{t('login.token')}</label>
		<input id="token" type="password" autocomplete="current-password" aria-describedby="token-hint" bind:value={token} required />
		<p class="hint" id="token-hint">{t('login.hint')}</p>
		<button type="submit" disabled={busy}>{t('login.submit')}</button>
		{#if error}<p role="alert">{error}</p>{/if}
	</form>
</main>

<style>
	.login { max-width: 24rem; margin: 4rem auto; padding: 0 1rem; }
	form { display: grid; gap: 0.5rem; }
	.hint { font-size: 0.85rem; opacity: 0.7; margin: 0; }
	[role='alert'] { color: #b00020; }
</style>
