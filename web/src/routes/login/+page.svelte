<script lang="ts">
	import { goto } from '$app/navigation';
	import { ApiError, login } from '#lib/api.ts';
	import { errorMessage, t } from '#lib/i18n/index.ts';
	import Ring from '#lib/icons/Ring.svelte';

	let token = $state('');
	let error = $state('');
	let busy = $state(false);

	async function submit(event: SubmitEvent) {
		event.preventDefault();
		busy = true;
		error = '';
		try {
			await login(token.trim());
			token = '';
			await goto('/');
		} catch (err) {
			error = errorMessage(err instanceof ApiError ? err.code : 'unknown');
		} finally {
			busy = false;
		}
	}
</script>

<main class="login">
	<div class="card">
		<div class="brand">
			<Ring size={40} />
			<p class="name">{t('app.title')}</p>
		</div>
		<h1>{t('login.title')}</h1>
		<p class="lead">{t('login.lead')}</p>
		<form onsubmit={submit}>
			<label for="token">{t('login.token')}</label>
			<input
				id="token"
				type="password"
				autocomplete="current-password"
				aria-describedby="token-hint"
				aria-invalid={error ? 'true' : undefined}
				bind:value={token}
				required
			/>
			<p class="hint" id="token-hint">{t('login.hint')}</p>
			<button type="submit" class="btn primary" disabled={busy}>{busy ? t('login.busy') : t('login.submit')}</button>
			{#if error}<p role="alert">{error}</p>{/if}
		</form>
	</div>
</main>

<style>
	.login {
		min-height: 100vh;
		min-height: 100dvh;
		display: grid;
		place-items: center;
		padding: 1.5rem 1rem;
	}
	.card {
		width: 100%;
		max-width: 24rem;
		padding: 2rem 1.75rem;
		background: var(--surface);
		border: 1px solid var(--trait);
		border-radius: var(--rayon);
	}
	.brand {
		display: flex;
		align-items: center;
		gap: 0.75rem;
		margin-bottom: 1.75rem;
	}
	.name {
		font-size: var(--t-xl);
		font-weight: 700;
		font-stretch: 87.5%;
		letter-spacing: -0.01em;
	}
	.lead {
		color: var(--texte-doux);
		margin: 0.4rem 0 1.5rem;
	}
	form {
		display: grid;
		gap: 0.5rem;
	}
	label {
		font-weight: 600;
		font-size: var(--t-s);
	}
	input {
		width: 100%;
	}
	.hint {
		font-size: var(--t-xs);
		color: var(--texte-doux);
		margin-bottom: 0.75rem;
	}
	.btn {
		width: 100%;
	}
</style>
