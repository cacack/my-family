<script lang="ts">
	import { api, type ExportPreview } from '$lib/api/client';
	import { Label } from '$lib/components/ui/label';
	import { Select, SelectContent, SelectItem, SelectTrigger } from '$lib/components/ui/select';

	interface Props {
		/** Selected GEDCOM version: 'auto' (default) or an explicit version string. */
		value?: string;
		/** Disable the selector while an export is running. */
		disabled?: boolean;
	}

	let { value = $bindable('auto'), disabled = false }: Props = $props();

	const OPTIONS = [
		{ value: 'auto', label: 'Automatic (recommended)' },
		{ value: '5.5', label: 'GEDCOM 5.5' },
		{ value: '5.5.1', label: 'GEDCOM 5.5.1' },
		{ value: '7.0', label: 'GEDCOM 7.0' }
	];

	let preview = $state<ExportPreview | null>(null);
	let loading = $state(false);
	let error = $state<string | null>(null);

	function labelFor(v: string): string {
		return OPTIONS.find((o) => o.value === v)?.label ?? v;
	}

	// Whenever the selected version changes, fetch a fresh data-loss preview.
	// 'auto' never downgrades (it upgrades to 7.0 when the data needs it), so
	// there is nothing to warn about and no need to hit the preview endpoint.
	// Stale in-flight requests are cancelled so a slow response can't overwrite
	// a newer selection.
	$effect(() => {
		const v = value;
		preview = null;
		error = null;
		if (v === 'auto') {
			loading = false;
			return;
		}
		let cancelled = false;
		loading = true;
		api
			.previewGedcomExport(v)
			.then((p) => {
				if (!cancelled) preview = p;
			})
			.catch((e) => {
				if (!cancelled) error = (e as { message?: string }).message || 'Could not check for data loss';
			})
			.finally(() => {
				if (!cancelled) loading = false;
			});
		return () => {
			cancelled = true;
		};
	});
</script>

<div class="version-select">
	<Label for="export-version">GEDCOM version</Label>
	<Select type="single" {value} onValueChange={(v) => (value = v ?? 'auto')} {disabled}>
		<SelectTrigger id="export-version" class="w-full">
			{labelFor(value)}
		</SelectTrigger>
		<SelectContent>
			{#each OPTIONS as opt}
				<SelectItem value={opt.value} label={opt.label}>{opt.label}</SelectItem>
			{/each}
		</SelectContent>
	</Select>

	{#if loading}
		<p class="hint" aria-live="polite">Checking for data loss…</p>
	{:else if error}
		<p class="error" role="alert">{error}</p>
	{:else if preview?.hasDataLoss}
		<div class="warning" role="alert">
			<p class="warning-title">
				Exporting as {labelFor(value)} affects {preview.dataLoss.length} feature{preview
					.dataLoss.length === 1
					? ''
					: 's'} not standard in that version:
			</p>
			<ul>
				{#each preview.dataLoss as item}
					<li><strong>{item.feature}</strong> — {item.reason}</li>
				{/each}
			</ul>
			<p class="warning-note">
				These are written using non-standard tags so the data is preserved, but older software
				may not read them.
			</p>
		</div>
	{/if}
</div>

<style>
	.version-select {
		display: flex;
		flex-direction: column;
		gap: 0.5rem;
	}

	.hint {
		font-size: 0.8125rem;
		color: hsl(var(--muted-foreground));
	}

	.error {
		font-size: 0.8125rem;
		color: hsl(var(--destructive));
	}

	.warning {
		background: hsl(var(--muted));
		border: 1px solid hsl(var(--border));
		border-left: 3px solid #f59e0b;
		border-radius: 8px;
		padding: 0.75rem 1rem;
		font-size: 0.8125rem;
	}

	.warning-title {
		font-weight: 500;
		color: hsl(var(--foreground));
	}

	.warning ul {
		margin: 0.5rem 0;
		padding-left: 1.1rem;
		list-style: disc;
		color: hsl(var(--muted-foreground));
	}

	.warning li {
		padding: 0.125rem 0;
	}

	.warning-note {
		color: hsl(var(--muted-foreground));
	}
</style>
