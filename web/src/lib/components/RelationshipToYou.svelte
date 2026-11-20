<script lang="ts">
	import { untrack } from 'svelte';
	import { api, type Person } from '#lib/api/client.js';
	import { Button } from '#lib/components/ui/button/index.js';
	import { homePerson, setHomePerson, clearHomePerson } from '#lib/stores/homePerson.svelte.js';
	import { canChooseHomePerson } from '#lib/stores/auth.svelte.js';
	import { relationshipToYou } from '#lib/utils/relationship.js';

	/**
	 * How a person relates to the home person ("me"), linked to the full path in
	 * the relationship calculator, with the control that marks or unmarks them as
	 * "me" (#1034).
	 */
	interface Props {
		person: Pick<Person, 'id' | 'gender'>;
	}

	let { person }: Props = $props();

	const home = $derived(homePerson.person);
	const isHome = $derived(home?.id === person.id);

	// The phrase for the pair being shown (text null: not related); null while
	// loading or when the calculation failed.
	let relation: { pair: string; text: string | null } | null = $state(null);
	let busy = $state(false);
	let error: string | null = $state(null);

	const pair = $derived(home && !isHome ? `${home.id}|${person.id}` : null);

	$effect(() => {
		const current = pair;
		const homeId = home?.id;
		const gender = person.gender;
		untrack(() => {
			relation = null;
			if (!current || !homeId) return;
			api
				.getRelationship(homeId, person.id)
				.then((res) => {
					if (pair === current) relation = { pair: current, text: relationshipToYou(res, gender) };
				})
				// A relationship that cannot be calculated is left unsaid; the
				// calculator is one menu away.
				.catch(() => {});
		});
	});

	const calculatorHref = $derived(
		home ? `/relationship?personA=${encodeURIComponent(home.id)}&personB=${encodeURIComponent(person.id)}` : ''
	);

	async function change(action: () => Promise<void>, failure: string) {
		busy = true;
		error = null;
		try {
			await action();
		} catch {
			error = failure;
		} finally {
			busy = false;
		}
	}
</script>

{#if homePerson.loaded}
	<div class="relationship-to-you" data-testid="relationship-to-you">
		{#if isHome}
			<span class="you">This is you</span>
			{#if canChooseHomePerson()}
				<Button
					variant="outline"
					size="xs"
					disabled={busy}
					onclick={() => change(clearHomePerson, 'Could not clear who you are. Please try again.')}
				>
					Not me
				</Button>
			{/if}
		{:else}
			{#if relation && relation.pair === pair}
				<a href={calculatorHref} class="relation">{relation.text ?? 'No known relationship to you'}</a>
			{/if}
			{#if canChooseHomePerson()}
				<Button
					variant="outline"
					size="xs"
					disabled={busy}
					onclick={() => change(() => setHomePerson(person.id), 'Could not set who you are. Please try again.')}
				>
					This is me
				</Button>
			{/if}
		{/if}
		{#if error}
			<span class="error" role="alert">{error}</span>
		{/if}
	</div>
{/if}

<style>
	.relationship-to-you {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: 0.25rem 0.75rem;
		margin-top: 0.25rem;
		font-size: 0.875rem;
	}

	.you {
		font-weight: 600;
		color: var(--color-text);
	}

	.relation {
		color: var(--color-link);
		font-weight: 500;
		overflow-wrap: anywhere;
	}

	.error {
		color: var(--color-destructive);
	}
</style>
