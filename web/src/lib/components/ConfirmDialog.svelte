<script lang="ts">
	import { t } from '#lib/i18n/index.ts';

	// Every write goes through this dialog (spec §9): it names the container
	// and the path. Shown while mounted; Cancel has the focus first.
	let {
		title,
		container,
		path,
		details = [],
		onconfirm,
		oncancel
	}: {
		title: string;
		container: string;
		path: string;
		details?: string[];
		onconfirm: () => void;
		oncancel: () => void;
	} = $props();

	let dialog = $state<HTMLDialogElement>();
	let busy = false; // set at the first click, before the parent reacts: one confirmation only

	function confirm() {
		if (busy) return;
		busy = true;
		onconfirm();
	}

	$effect(() => {
		const d = dialog;
		const opener = document.activeElement;
		d?.showModal();
		return () => {
			d?.close();
			// The dialog leaves the DOM: give the focus back to the button that opened it.
			if (opener instanceof HTMLElement && opener.isConnected) opener.focus();
		};
	});
</script>

<dialog
	bind:this={dialog}
	aria-labelledby="confirm-title"
	oncancel={(e) => {
		e.preventDefault(); // Escape: same as Cancel, the parent unmounts the dialog
		oncancel();
	}}
>
	<h2 id="confirm-title">{title}</h2>
	<dl>
		<dt>{t('confirm.container')}</dt>
		<dd>{container}</dd>
		<dt>{t('confirm.path')}</dt>
		<dd><code>{path}</code></dd>
	</dl>
	{#each details as d, i (i)}<p>{d}</p>{/each}
	<div class="actions">
		<button type="button" onclick={oncancel}>{t('confirm.cancel')}</button>
		<button type="button" class="primary" onclick={confirm}>{t('confirm.ok')}</button>
	</div>
</dialog>

<style>
	dialog { box-sizing: border-box; max-width: min(36rem, calc(100vw - 2rem)); }
	h2 { font-size: 1.1rem; margin-top: 0; }
	dl { display: grid; grid-template-columns: auto minmax(0, 1fr); gap: 0.25rem 0.75rem; }
	dd { margin: 0; overflow-wrap: anywhere; }
	.actions { display: flex; justify-content: flex-end; gap: 0.5rem; }
	.primary { font-weight: 600; }
</style>
