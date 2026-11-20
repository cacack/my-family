<script lang="ts">
	import EmptyState from '#lib/components/EmptyState.svelte';
	import PageHeader from '#lib/components/PageHeader.svelte';
	import { Button } from '#lib/components/ui/button/index.js';
	import { canChooseHomePerson } from '#lib/stores/access.svelte.js';

	/**
	 * A chart route reached without a person when no home person is set: says
	 * how to choose one, which the chart would otherwise start from (#1034).
	 */
	interface Props {
		/** The chart's name, e.g. "Pedigree Chart". */
		title: string;
	}

	let { title }: Props = $props();
</script>

<svelte:head>
	<title>{title} | My Family</title>
</svelte:head>

<div class="no-home-person">
	<PageHeader {title} />
	<!-- A visitor has no home person to choose (#1443). -->
	<EmptyState
		message={canChooseHomePerson() ? 'Choose who you are to start here' : 'Choose a person to start here'}
		description={canChooseHomePerson()
			? "Open your own person page and choose “This is me”. Charts then start from you. Until then, open a chart from any person's page."
			: "Open a chart from any person's page."}
	>
		{#snippet actions()}
			<Button href="/persons">Browse people</Button>
		{/snippet}
	</EmptyState>
</div>

<style>
	.no-home-person {
		max-width: var(--page-width-list);
		margin: 0 auto;
		padding: 1.5rem;
	}
</style>
