<script lang="ts">
	interface Props {
		status: 'certain' | 'probable' | 'possible' | 'unknown';
		size?: 'small' | 'medium' | 'large';
		showLabel?: boolean;
	}

	let { status, size = 'medium', showLabel = false }: Props = $props();

	const config = $derived(() => {
		switch (status) {
			case 'certain':
				return {
					color: '#22c55e',
					bgColor: '#dcfce7',
					label: 'Certain',
					tooltip: 'Confirmed with strong evidence'
				};
			case 'probable':
				return {
					color: '#eab308',
					bgColor: '#fef9c3',
					label: 'Probable',
					tooltip: 'Likely correct, good supporting evidence'
				};
			case 'possible':
				return {
					color: '#f97316',
					bgColor: '#ffedd5',
					label: 'Possible',
					tooltip: 'Speculative, limited evidence'
				};
			default:
				return {
					color: '#6b7280',
					bgColor: '#f3f4f6',
					label: 'Unknown',
					tooltip: 'Not yet assessed'
				};
		}
	});

	const dimensions = $derived(() => {
		switch (size) {
			case 'small':
				return { padding: '0.125rem 0.375rem', fontSize: '0.625rem', iconSize: '0.5rem' };
			case 'large':
				return { padding: '0.375rem 0.75rem', fontSize: '0.875rem', iconSize: '0.75rem' };
			default:
				return { padding: '0.25rem 0.5rem', fontSize: '0.75rem', iconSize: '0.625rem' };
		}
	});
</script>

<span
	class="uncertainty-badge"
	class:small={size === 'small'}
	class:large={size === 'large'}
	style:background-color={config().bgColor}
	style:color={config().color}
	style:padding={dimensions().padding}
	style:font-size={dimensions().fontSize}
	title={config().tooltip}
>
	<span class="indicator" style:background-color={config().color} style:width={dimensions().iconSize} style:height={dimensions().iconSize}></span>
	{#if showLabel}
		<span class="label">{config().label}</span>
	{/if}
</span>

<style>
	.uncertainty-badge {
		display: inline-flex;
		align-items: center;
		gap: 0.25rem;
		border-radius: 9999px;
		font-weight: 500;
		cursor: help;
		white-space: nowrap;
	}

	.uncertainty-badge.small {
		gap: 0.125rem;
	}

	.uncertainty-badge.large {
		gap: 0.375rem;
	}

	.indicator {
		border-radius: 50%;
		flex-shrink: 0;
	}

	.label {
		text-transform: capitalize;
	}
</style>
