<script lang="ts">
	import { Progress as ProgressPrimitive } from "bits-ui";
	import { cn, type WithoutChildrenOrChild } from "$lib/utils.js";

	let {
		ref = $bindable(null),
		class: className,
		max = 100,
		value,
		...restProps
	}: WithoutChildrenOrChild<ProgressPrimitive.RootProps> = $props();

	// Guard the indicator transform: a caller passing max=0 (or a negative value)
	// would otherwise produce Infinity/NaN in the translateX expression, leaving
	// the bar visually broken.
	const safeMax = $derived(max > 0 ? max : 1);
	const clampedValue = $derived(Math.min(Math.max(value ?? 0, 0), safeMax));
	const offsetPercent = $derived(100 - (100 * clampedValue) / safeMax);
</script>

<ProgressPrimitive.Root
	bind:ref
	data-slot="progress"
	class={cn("bg-muted h-1.5 rounded-full relative flex w-full items-center overflow-x-hidden", className)}
	{value}
	{max}
	{...restProps}
>
	<div
		data-slot="progress-indicator"
		class="bg-primary size-full flex-1 transition-all"
		style="transform: translateX(-{offsetPercent}%)"
	></div>
</ProgressPrimitive.Root>
