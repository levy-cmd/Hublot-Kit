<script lang="ts">
	import { t } from '#lib/i18n/index.ts';
	import Icon from '#lib/icons/Icon.svelte';

	// Copies text as given: the caller passes the masked form of a command,
	// never a revealed value (spec v0.2 §3). Absent without a clipboard (an
	// agent reached over plain HTTP from another machine).
	let { text, label }: { text: string; label: string } = $props();

	const can = typeof navigator !== 'undefined' && !!navigator.clipboard && globalThis.isSecureContext;
	let done = $state(false);
	let timer: ReturnType<typeof setTimeout> | undefined;

	async function copy() {
		try {
			await navigator.clipboard.writeText(text);
			done = true;
			clearTimeout(timer);
			timer = setTimeout(() => (done = false), 1500);
		} catch {
			done = false;
		}
	}
</script>

{#if can}
	<button type="button" class="icon-btn small" aria-label={label} title={label} onclick={copy}>
		<Icon name={done ? 'check' : 'copy'} />
	</button>
	<span class="sr-only" role="status">{done ? t('container.copied') : ''}</span>
{/if}
