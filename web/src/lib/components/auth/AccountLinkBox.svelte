<script lang="ts">
	/**
	 * An invite or password reset link, shown once to the owner who issued it
	 * (ADR-007): the server keeps only its hash, and sends no email, so the
	 * owner copies it and passes it on. An API token is shown once the same
	 * way, with its own hint.
	 */
	import { Button } from '#lib/components/ui/button/index.js';
	import { Input } from '#lib/components/ui/input/index.js';
	import { Label } from '#lib/components/ui/label/index.js';

	interface Props {
		/** Unique within the page, for the input's id. */
		id: string;
		/** What the link is, e.g. "Invite link (editor)". */
		label: string;
		url: string;
		/** ISO time the link stops working. */
		expiresAt?: string;
		/** Replaces the hint about passing the link on and when it expires. */
		hint?: string;
	}

	let { id, label, url, expiresAt, hint }: Props = $props();

	let copied = $state(false);

	const expires = $derived(
		expiresAt
			? new Date(expiresAt).toLocaleString(undefined, { dateStyle: 'medium', timeStyle: 'short' })
			: ''
	);

	async function copy() {
		try {
			await navigator.clipboard.writeText(url);
			copied = true;
		} catch {
			// No clipboard (plain HTTP, or denied): the text stays selectable.
			(document.getElementById(id) as HTMLInputElement | null)?.select();
		}
	}
</script>

<div class="link-box" data-testid="account-link">
	<Label for={id}>{label}</Label>
	<div class="link-row">
		<Input {id} readonly value={url} class="link-input" onfocus={(e) => (e.currentTarget as HTMLInputElement).select()} aria-describedby="{id}-hint" />
		<Button variant="outline" onclick={copy}>{copied ? 'Copied' : 'Copy'}</Button>
	</div>
	<p id="{id}-hint" class="hint">
		{hint ?? `Send it to them yourself. It works once, until ${expires}, and is not shown again.`}
	</p>
</div>

<style>
	.link-box {
		display: flex;
		flex-direction: column;
		gap: 0.375rem;
	}

	.link-row {
		display: flex;
		gap: 0.5rem;
		min-width: 0;
	}

	.link-row :global(.link-input) {
		flex: 1 1 auto;
		min-width: 0;
		font-family: ui-monospace, monospace;
		font-size: 0.8125rem;
	}

	.hint {
		margin: 0;
		font-size: 0.75rem;
		color: var(--color-text-muted);
	}
</style>
