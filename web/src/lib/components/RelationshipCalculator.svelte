<script lang="ts">
	import { api, type RelationshipResult, type RelationshipPath, type Person, type SearchResult, formatPersonName } from '$lib/api/client';
	import PersonSelector from './PersonSelector.svelte';

	interface Props {
		initialPersonA?: Person | null;
		initialPersonB?: Person | null;
	}

	let { initialPersonA = null, initialPersonB = null }: Props = $props();

	let personA = $state<Person | SearchResult | null>(initialPersonA);
	let personB = $state<Person | SearchResult | null>(initialPersonB);
	let result = $state<RelationshipResult | null>(null);
	let loading = $state(false);
	let error = $state<string | null>(null);
	let hasCalculated = $state(false);

	// Track if inputs have changed since last calculation
	let inputsChanged = $derived.by(() => {
		const currentA = personA?.id;
		const currentB = personB?.id;
		const resultA = result?.personA?.id;
		const resultB = result?.personB?.id;
		return !result || currentA !== resultA || currentB !== resultB;
	});

	async function calculateRelationship() {
		if (!personA || !personB) return;

		// Check if same person is selected twice
		if (personA.id === personB.id) {
			error = 'Please select two different people to find their relationship.';
			result = null;
			hasCalculated = true;
			return;
		}

		loading = true;
		error = null;
		result = null;
		hasCalculated = true;

		try {
			result = await api.getRelationship(personA.id, personB.id);
		} catch (e) {
			const apiError = e as { message?: string; code?: string };
			if (apiError.code === 'NOT_FOUND') {
				error = 'One or both people could not be found.';
			} else {
				error = apiError.message || 'Failed to calculate relationship. Please try again.';
			}
		} finally {
			loading = false;
		}
	}

	function handlePersonASelect(person: SearchResult | null) {
		personA = person;
		// Reset result when person changes
		if (hasCalculated) {
			result = null;
			error = null;
		}
	}

	function handlePersonBSelect(person: SearchResult | null) {
		personB = person;
		// Reset result when person changes
		if (hasCalculated) {
			result = null;
			error = null;
		}
	}

	function swapPeople() {
		const temp = personA;
		personA = personB;
		personB = temp;
		if (hasCalculated) {
			result = null;
			error = null;
		}
	}

	function clearAll() {
		personA = null;
		personB = null;
		result = null;
		error = null;
		hasCalculated = false;
	}

	// Get the primary relationship name (first path)
	function getPrimaryRelationship(paths?: RelationshipPath[]): string {
		if (!paths || paths.length === 0) return 'No relationship found';
		return paths[0].name || 'Related';
	}

	// Get additional relationship paths (after the first)
	function getAdditionalPaths(paths?: RelationshipPath[]): RelationshipPath[] {
		if (!paths || paths.length <= 1) return [];
		return paths.slice(1);
	}

	// Format the path as a visual chain
	function formatPathChain(path: RelationshipPath, personA: Person | undefined, personB: Person | undefined): string[] {
		const chain: string[] = [];

		// Add person A's path to common ancestor
		if (path.pathFromA && path.pathFromA.length > 0) {
			chain.push(...path.pathFromA);
		}

		// Add person B's path from common ancestor (reversed)
		if (path.pathFromB && path.pathFromB.length > 0) {
			chain.push(...path.pathFromB.slice().reverse());
		}

		return chain;
	}
</script>

<div class="relationship-calculator">
	<div class="calculator-header">
		<h2 class="title">Relationship Calculator</h2>
		<p class="subtitle">Select two people to discover how they are related</p>
	</div>

	<div class="selector-section">
		<div class="selectors-grid">
			<div class="selector-container">
				<PersonSelector
					label="First Person"
					selectedPerson={personA}
					onSelect={handlePersonASelect}
					placeholder="Search for first person..."
					disabled={loading}
				/>
			</div>

			<div class="swap-container">
				<button
					type="button"
					class="swap-btn"
					onclick={swapPeople}
					disabled={loading || (!personA && !personB)}
					aria-label="Swap people"
					title="Swap people"
				>
					<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
						<path d="M7 16V4m0 0L3 8m4-4l4 4m6 4v12m0 0l4-4m-4 4l-4-4" />
					</svg>
				</button>
			</div>

			<div class="selector-container">
				<PersonSelector
					label="Second Person"
					selectedPerson={personB}
					onSelect={handlePersonBSelect}
					placeholder="Search for second person..."
					disabled={loading}
				/>
			</div>
		</div>

		<div class="action-buttons">
			<button
				type="button"
				class="calculate-btn"
				onclick={calculateRelationship}
				disabled={loading || !personA || !personB}
			>
				{#if loading}
					<span class="spinner"></span>
					Calculating...
				{:else}
					Calculate Relationship
				{/if}
			</button>

			{#if personA || personB || result}
				<button
					type="button"
					class="clear-btn"
					onclick={clearAll}
					disabled={loading}
				>
					Clear All
				</button>
			{/if}
		</div>
	</div>

	<!-- Results Section -->
	{#if hasCalculated && !loading}
		<div class="results-section" role="region" aria-label="Relationship results">
			{#if error}
				<div class="error-message" role="alert">
					<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
						<circle cx="12" cy="12" r="10" />
						<path d="M12 8v4m0 4h.01" />
					</svg>
					<span>{error}</span>
				</div>
			{:else if result}
				{#if result.isRelated && result.paths && result.paths.length > 0}
					<!-- Primary relationship result -->
					<div class="primary-result">
						<div class="relationship-badge">
							<span class="relationship-label">Relationship</span>
							<span class="relationship-name">{getPrimaryRelationship(result.paths)}</span>
						</div>

						{#if result.summary}
							<p class="relationship-summary">{result.summary}</p>
						{/if}
					</div>

					<!-- Relationship Path Visualization -->
					{#if result.paths[0]}
						<div class="path-visualization">
							<h3 class="path-title">Relationship Path</h3>
							<div class="path-chain">
								<!-- Person A -->
								<div class="path-node person-node" data-gender={result.personA?.gender}>
									<div class="node-avatar">
										<svg viewBox="0 0 24 24" fill="currentColor">
											<path d="M12 4a4 4 0 1 0 0 8 4 4 0 0 0 0-8zM6 8a6 6 0 1 1 12 0A6 6 0 0 1 6 8zm2 10a3 3 0 0 0-3 3 1 1 0 1 1-2 0 5 5 0 0 1 5-5h8a5 5 0 0 1 5 5 1 1 0 1 1-2 0 3 3 0 0 0-3-3H8z" />
										</svg>
									</div>
									<span class="node-name">{result.personA ? formatPersonName(result.personA) : 'Person A'}</span>
								</div>

								<!-- Path from A to common ancestor -->
								{#if result.paths[0].pathFromA && result.paths[0].pathFromA.length > 0}
									{#each result.paths[0].pathFromA as step}
										<div class="path-connector">
											<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
												<path d="M12 5v14m-7-7l7 7 7-7" />
											</svg>
										</div>
										<div class="path-step">
											<span class="step-label">{step}</span>
										</div>
									{/each}
								{/if}

								<!-- Common ancestor indicator -->
								{#if result.paths[0].commonAncestorId}
									<div class="path-connector common-ancestor-connector">
										<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
											<circle cx="12" cy="12" r="4" />
										</svg>
									</div>
									<div class="path-node common-ancestor-node">
										<span class="node-label">Common Ancestor</span>
									</div>
								{/if}

								<!-- Path from common ancestor to B (reversed) -->
								{#if result.paths[0].pathFromB && result.paths[0].pathFromB.length > 0}
									{#each result.paths[0].pathFromB.slice().reverse() as step}
										<div class="path-connector">
											<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
												<path d="M12 5v14m-7-7l7 7 7-7" />
											</svg>
										</div>
										<div class="path-step">
											<span class="step-label">{step}</span>
										</div>
									{/each}
								{/if}

								<div class="path-connector">
									<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
										<path d="M12 5v14m-7-7l7 7 7-7" />
									</svg>
								</div>

								<!-- Person B -->
								<div class="path-node person-node" data-gender={result.personB?.gender}>
									<div class="node-avatar">
										<svg viewBox="0 0 24 24" fill="currentColor">
											<path d="M12 4a4 4 0 1 0 0 8 4 4 0 0 0 0-8zM6 8a6 6 0 1 1 12 0A6 6 0 0 1 6 8zm2 10a3 3 0 0 0-3 3 1 1 0 1 1-2 0 5 5 0 0 1 5-5h8a5 5 0 0 1 5 5 1 1 0 1 1-2 0 3 3 0 0 0-3-3H8z" />
										</svg>
									</div>
									<span class="node-name">{result.personB ? formatPersonName(result.personB) : 'Person B'}</span>
								</div>
							</div>

							<!-- Generation distances -->
							{#if result.paths[0].generationDistanceA !== undefined || result.paths[0].generationDistanceB !== undefined}
								<div class="generation-info">
									{#if result.paths[0].generationDistanceA !== undefined}
										<span class="gen-distance">
											{result.personA ? formatPersonName(result.personA) : 'Person A'}: {result.paths[0].generationDistanceA} generation{result.paths[0].generationDistanceA !== 1 ? 's' : ''} to common ancestor
										</span>
									{/if}
									{#if result.paths[0].generationDistanceB !== undefined}
										<span class="gen-distance">
											{result.personB ? formatPersonName(result.personB) : 'Person B'}: {result.paths[0].generationDistanceB} generation{result.paths[0].generationDistanceB !== 1 ? 's' : ''} to common ancestor
										</span>
									{/if}
								</div>
							{/if}
						</div>
					{/if}

					<!-- Additional relationships -->
					{#if getAdditionalPaths(result.paths).length > 0}
						<div class="additional-relationships">
							<h3 class="additional-title">Also Related As</h3>
							<ul class="additional-list">
								{#each getAdditionalPaths(result.paths) as path}
									<li class="additional-item">
										<span class="additional-name">{path.name || 'Related'}</span>
									</li>
								{/each}
							</ul>
						</div>
					{/if}
				{:else}
					<!-- Not related -->
					<div class="not-related">
						<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
							<circle cx="12" cy="12" r="10" />
							<path d="M8 12h8" />
						</svg>
						<h3>No Relationship Found</h3>
						<p>These two people do not appear to be related in your family tree, or their connection has not yet been documented.</p>
					</div>
				{/if}
			{/if}
		</div>
	{/if}
</div>

<style>
	.relationship-calculator {
		max-width: 800px;
		margin: 0 auto;
		padding: 1.5rem;
	}

	.calculator-header {
		text-align: center;
		margin-bottom: 2rem;
	}

	.title {
		margin: 0 0 0.5rem;
		font-size: 1.75rem;
		font-weight: 700;
		color: #1e293b;
	}

	.subtitle {
		margin: 0;
		font-size: 1rem;
		color: #64748b;
	}

	/* Selector Section */
	.selector-section {
		background: white;
		border-radius: 12px;
		padding: 1.5rem;
		box-shadow: 0 1px 3px rgba(0, 0, 0, 0.1);
		margin-bottom: 1.5rem;
	}

	.selectors-grid {
		display: grid;
		grid-template-columns: 1fr auto 1fr;
		gap: 1rem;
		align-items: end;
		margin-bottom: 1.5rem;
	}

	@media (max-width: 640px) {
		.selectors-grid {
			grid-template-columns: 1fr;
			gap: 1rem;
		}

		.swap-container {
			order: -1;
			justify-self: center;
		}
	}

	.selector-container {
		min-width: 0;
	}

	.swap-container {
		display: flex;
		align-items: flex-end;
		padding-bottom: 0.5rem;
	}

	.swap-btn {
		display: flex;
		align-items: center;
		justify-content: center;
		width: 2.5rem;
		height: 2.5rem;
		padding: 0;
		border: 1px solid #d1d5db;
		border-radius: 50%;
		background: white;
		color: #64748b;
		cursor: pointer;
		transition: all 0.15s;
	}

	.swap-btn:hover:not(:disabled) {
		background: #f1f5f9;
		border-color: #9ca3af;
		color: #374151;
	}

	.swap-btn:disabled {
		opacity: 0.5;
		cursor: not-allowed;
	}

	.swap-btn svg {
		width: 1.25rem;
		height: 1.25rem;
	}

	.action-buttons {
		display: flex;
		gap: 0.75rem;
		justify-content: center;
		flex-wrap: wrap;
	}

	.calculate-btn {
		display: flex;
		align-items: center;
		justify-content: center;
		gap: 0.5rem;
		padding: 0.75rem 1.5rem;
		border: none;
		border-radius: 8px;
		background: #3b82f6;
		color: white;
		font-size: 1rem;
		font-weight: 600;
		cursor: pointer;
		transition: background 0.15s;
		min-width: 200px;
	}

	.calculate-btn:hover:not(:disabled) {
		background: #2563eb;
	}

	.calculate-btn:disabled {
		background: #94a3b8;
		cursor: not-allowed;
	}

	.spinner {
		width: 1rem;
		height: 1rem;
		border: 2px solid rgba(255, 255, 255, 0.3);
		border-top-color: white;
		border-radius: 50%;
		animation: spin 0.6s linear infinite;
	}

	@keyframes spin {
		to {
			transform: rotate(360deg);
		}
	}

	.clear-btn {
		padding: 0.75rem 1.5rem;
		border: 1px solid #d1d5db;
		border-radius: 8px;
		background: white;
		color: #64748b;
		font-size: 1rem;
		font-weight: 500;
		cursor: pointer;
		transition: all 0.15s;
	}

	.clear-btn:hover:not(:disabled) {
		background: #f1f5f9;
		border-color: #9ca3af;
	}

	.clear-btn:disabled {
		opacity: 0.5;
		cursor: not-allowed;
	}

	/* Results Section */
	.results-section {
		background: white;
		border-radius: 12px;
		padding: 1.5rem;
		box-shadow: 0 1px 3px rgba(0, 0, 0, 0.1);
	}

	.error-message {
		display: flex;
		align-items: center;
		gap: 0.75rem;
		padding: 1rem;
		background: #fef2f2;
		border: 1px solid #fecaca;
		border-radius: 8px;
		color: #dc2626;
	}

	.error-message svg {
		flex-shrink: 0;
		width: 1.5rem;
		height: 1.5rem;
	}

	/* Primary Result */
	.primary-result {
		text-align: center;
		margin-bottom: 1.5rem;
	}

	.relationship-badge {
		display: inline-flex;
		flex-direction: column;
		align-items: center;
		padding: 1rem 2rem;
		background: linear-gradient(135deg, #eff6ff 0%, #dbeafe 100%);
		border: 2px solid #3b82f6;
		border-radius: 12px;
	}

	.relationship-label {
		font-size: 0.75rem;
		font-weight: 500;
		color: #64748b;
		text-transform: uppercase;
		letter-spacing: 0.05em;
		margin-bottom: 0.25rem;
	}

	.relationship-name {
		font-size: 1.5rem;
		font-weight: 700;
		color: #1e293b;
	}

	@media (max-width: 640px) {
		.relationship-name {
			font-size: 1.25rem;
		}
	}

	.relationship-summary {
		margin: 1rem 0 0;
		font-size: 0.9375rem;
		color: #475569;
	}

	/* Path Visualization */
	.path-visualization {
		margin-top: 1.5rem;
		padding-top: 1.5rem;
		border-top: 1px solid #e2e8f0;
	}

	.path-title {
		margin: 0 0 1rem;
		font-size: 1rem;
		font-weight: 600;
		color: #374151;
		text-align: center;
	}

	.path-chain {
		display: flex;
		flex-direction: column;
		align-items: center;
		gap: 0.5rem;
	}

	.path-node {
		display: flex;
		flex-direction: column;
		align-items: center;
		gap: 0.5rem;
		padding: 0.75rem 1.5rem;
		background: #f8fafc;
		border: 1px solid #e2e8f0;
		border-radius: 8px;
		min-width: 160px;
	}

	.path-node.person-node {
		background: white;
		border-width: 2px;
	}

	.path-node.person-node[data-gender="male"] {
		border-color: #3b82f6;
		background: #eff6ff;
	}

	.path-node.person-node[data-gender="female"] {
		border-color: #ec4899;
		background: #fdf2f8;
	}

	.node-avatar {
		width: 2.5rem;
		height: 2.5rem;
		border-radius: 50%;
		display: flex;
		align-items: center;
		justify-content: center;
		background: #f1f5f9;
		color: #64748b;
	}

	.path-node.person-node[data-gender="male"] .node-avatar {
		background: #dbeafe;
		color: #3b82f6;
	}

	.path-node.person-node[data-gender="female"] .node-avatar {
		background: #fce7f3;
		color: #ec4899;
	}

	.node-avatar svg {
		width: 1.25rem;
		height: 1.25rem;
	}

	.node-name {
		font-size: 0.875rem;
		font-weight: 600;
		color: #1e293b;
		text-align: center;
	}

	.node-label {
		font-size: 0.8125rem;
		font-weight: 500;
		color: #64748b;
	}

	.common-ancestor-node {
		background: #fef3c7;
		border-color: #f59e0b;
	}

	.path-connector {
		display: flex;
		align-items: center;
		justify-content: center;
		color: #94a3b8;
	}

	.path-connector svg {
		width: 1.25rem;
		height: 1.25rem;
	}

	.common-ancestor-connector svg {
		color: #f59e0b;
	}

	.path-step {
		display: flex;
		align-items: center;
		justify-content: center;
		padding: 0.5rem 1rem;
		background: #f1f5f9;
		border-radius: 9999px;
	}

	.step-label {
		font-size: 0.8125rem;
		color: #475569;
	}

	.generation-info {
		display: flex;
		flex-direction: column;
		gap: 0.25rem;
		margin-top: 1rem;
		padding: 0.75rem;
		background: #f8fafc;
		border-radius: 8px;
		text-align: center;
	}

	.gen-distance {
		font-size: 0.8125rem;
		color: #64748b;
	}

	/* Additional Relationships */
	.additional-relationships {
		margin-top: 1.5rem;
		padding-top: 1.5rem;
		border-top: 1px solid #e2e8f0;
	}

	.additional-title {
		margin: 0 0 0.75rem;
		font-size: 0.9375rem;
		font-weight: 600;
		color: #374151;
	}

	.additional-list {
		list-style: none;
		margin: 0;
		padding: 0;
		display: flex;
		flex-wrap: wrap;
		gap: 0.5rem;
	}

	.additional-item {
		padding: 0.375rem 0.75rem;
		background: #f1f5f9;
		border-radius: 9999px;
	}

	.additional-name {
		font-size: 0.875rem;
		color: #475569;
	}

	/* Not Related */
	.not-related {
		text-align: center;
		padding: 2rem;
	}

	.not-related svg {
		width: 3rem;
		height: 3rem;
		color: #94a3b8;
		margin-bottom: 1rem;
	}

	.not-related h3 {
		margin: 0 0 0.5rem;
		font-size: 1.125rem;
		font-weight: 600;
		color: #374151;
	}

	.not-related p {
		margin: 0;
		font-size: 0.9375rem;
		color: #64748b;
		max-width: 400px;
		margin-inline: auto;
	}
</style>
