<script lang="ts">
	import { resetDemo } from '$lib/stores/appConfig.svelte';

	let resetting = $state(false);

	async function handleReset() {
		resetting = true;
		const ok = await resetDemo();
		if (ok) {
			window.location.reload();
		}
		resetting = false;
	}
</script>

<div class="demo-banner" role="status">
	<span class="demo-label">Demo Mode</span>
	<span class="demo-text">Exploring with sample data. Changes won't be saved.</span>
	<button class="demo-reset" onclick={handleReset} disabled={resetting}>
		{resetting ? 'Resetting...' : 'Reset Demo'}
	</button>
</div>

<style>
	.demo-banner {
		display: flex;
		align-items: center;
		gap: 0.75rem;
		padding: 0.5rem 1.5rem;
		background: #fef3c7;
		border-bottom: 1px solid #f59e0b;
		font-size: 0.875rem;
		color: #92400e;
	}

	:global(body.high-contrast) .demo-banner {
		background: #78350f;
		border-bottom-color: #f59e0b;
		color: #fef3c7;
	}

	.demo-label {
		font-weight: 700;
		white-space: nowrap;
	}

	.demo-text {
		flex: 1;
	}

	.demo-reset {
		padding: 0.25rem 0.75rem;
		border: 1px solid #d97706;
		border-radius: 4px;
		background: white;
		color: #92400e;
		font-size: 0.8125rem;
		font-weight: 500;
		cursor: pointer;
		white-space: nowrap;
		transition: background 0.15s;
	}

	:global(body.high-contrast) .demo-reset {
		background: #451a03;
		border-color: #f59e0b;
		color: #fef3c7;
	}

	.demo-reset:hover:not(:disabled) {
		background: #fef3c7;
	}

	.demo-reset:disabled {
		opacity: 0.6;
		cursor: not-allowed;
	}
</style>
