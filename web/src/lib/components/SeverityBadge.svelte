<script lang="ts">
	import { Badge } from '$lib/components/ui/badge';

	interface Props {
		severity: 'error' | 'warning' | 'info';
	}

	let { severity }: Props = $props();

	const config = $derived.by(() => {
		switch (severity) {
			case 'error':
				return {
					label: 'Error',
					class: 'bg-red-100 text-red-700 border-red-200'
				};
			case 'warning':
				return {
					label: 'Warning',
					class: 'bg-amber-100 text-amber-700 border-amber-200'
				};
			case 'info':
			default:
				return {
					label: 'Info',
					class: 'bg-blue-100 text-blue-700 border-blue-200'
				};
		}
	});
</script>

<Badge variant="outline" class={config.class}>{config.label}</Badge>
