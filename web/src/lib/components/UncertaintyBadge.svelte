<script lang="ts">
	import { Badge } from '$lib/components/ui/badge';

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
					indicatorColor: 'bg-green-500',
					badgeClass: 'bg-green-50 text-green-600 border-green-200 dark:bg-green-950 dark:text-green-400 dark:border-green-800',
					label: 'Certain',
					tooltip: 'Confirmed with strong evidence'
				};
			case 'probable':
				return {
					indicatorColor: 'bg-yellow-500',
					badgeClass: 'bg-yellow-50 text-yellow-600 border-yellow-200 dark:bg-yellow-950 dark:text-yellow-400 dark:border-yellow-800',
					label: 'Probable',
					tooltip: 'Likely correct, good supporting evidence'
				};
			case 'possible':
				return {
					indicatorColor: 'bg-orange-500',
					badgeClass: 'bg-orange-50 text-orange-600 border-orange-200 dark:bg-orange-950 dark:text-orange-400 dark:border-orange-800',
					label: 'Possible',
					tooltip: 'Speculative, limited evidence'
				};
			default:
				return {
					indicatorColor: 'bg-gray-400',
					badgeClass: 'bg-gray-50 text-gray-500 border-gray-200 dark:bg-gray-900 dark:text-gray-400 dark:border-gray-700',
					label: 'Unknown',
					tooltip: 'Not yet assessed'
				};
		}
	});

	const sizeClass = $derived(() => {
		switch (size) {
			case 'small':
				return { badge: 'h-4 px-1.5 text-[0.625rem] gap-1', indicator: 'size-2' };
			case 'large':
				return { badge: 'h-6 px-3 text-sm gap-1.5', indicator: 'size-3' };
			default:
				return { badge: 'gap-1', indicator: 'size-2.5' };
		}
	});
</script>

<Badge
	variant="outline"
	class="cursor-help {config().badgeClass} {sizeClass().badge}"
	title={config().tooltip}
>
	<span class="rounded-full flex-shrink-0 {config().indicatorColor} {sizeClass().indicator}"></span>
	{#if showLabel}
		<span class="capitalize">{config().label}</span>
	{/if}
</Badge>
