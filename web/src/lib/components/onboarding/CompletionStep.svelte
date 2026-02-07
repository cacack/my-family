<script lang="ts">
	interface Props {
		completionData: { personCount?: number; personId?: string; personName?: string };
		onFinish: () => void;
	}

	let { completionData, onFinish }: Props = $props();
</script>

<div class="completion-step">
	<div class="success-icon">
		<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
			<path d="M22 11.08V12a10 10 0 1 1-5.93-9.14" />
			<polyline points="22 4 12 14.01 9 11.01" />
		</svg>
	</div>

	<h2>You're All Set!</h2>

	{#if completionData.personCount}
		<p class="summary">Successfully imported {completionData.personCount} {completionData.personCount === 1 ? 'person' : 'people'} into your family tree.</p>
	{:else if completionData.personName}
		<p class="summary">Successfully created {completionData.personName} as your first person.</p>
	{:else}
		<p class="summary">Your family tree is ready. Start exploring!</p>
	{/if}

	<div class="feature-cards">
		<a href="/persons" class="feature-card">
			<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
				<path d="M17 21v-2a4 4 0 0 0-4-4H5a4 4 0 0 0-4 4v2" />
				<circle cx="9" cy="7" r="4" />
				<path d="M23 21v-2a4 4 0 0 0-3-3.87" />
				<path d="M16 3.13a4 4 0 0 1 0 7.75" />
			</svg>
			<h3>Browse People</h3>
			<p>View and manage all the people in your tree.</p>
		</a>

		<a href="/families" class="feature-card">
			<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
				<path d="M17 21v-2a4 4 0 0 0-4-4H5a4 4 0 0 0-4 4v2" />
				<circle cx="9" cy="7" r="4" />
				<path d="M23 21v-2a4 4 0 0 0-3-3.87" />
				<path d="M16 3.13a4 4 0 0 1 0 7.75" />
			</svg>
			<h3>Family Groups</h3>
			<p>Create families and connect relationships.</p>
		</a>

		<a href="/import" class="feature-card">
			<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
				<path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4" />
				<polyline points="17 8 12 3 7 8" />
				<line x1="12" y1="3" x2="12" y2="15" />
			</svg>
			<h3>Import & Export</h3>
			<p>Import GEDCOM files or export your data.</p>
		</a>

		<a href="/sources" class="feature-card">
			<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
				<path d="M4 19.5A2.5 2.5 0 0 1 6.5 17H20" />
				<path d="M6.5 2H20v20H6.5A2.5 2.5 0 0 1 4 19.5v-15A2.5 2.5 0 0 1 6.5 2z" />
			</svg>
			<h3>Sources</h3>
			<p>Document your research with source citations.</p>
		</a>
	</div>

	<div class="actions">
		{#if completionData.personId}
			<a href="/persons/{completionData.personId}" class="btn btn-secondary">
				View {completionData.personName}'s Profile
			</a>
		{/if}
		<button class="btn btn-primary" onclick={onFinish}>Go to Dashboard</button>
	</div>
</div>

<style>
	.completion-step {
		max-width: 640px;
		margin: 0 auto;
		text-align: center;
	}

	.success-icon {
		width: 4rem;
		height: 4rem;
		margin: 0 auto 1rem;
		display: flex;
		align-items: center;
		justify-content: center;
		background: #f0fdf4;
		border-radius: 50%;
		color: #22c55e;
	}

	.success-icon svg {
		width: 2rem;
		height: 2rem;
	}

	h2 {
		margin: 0 0 0.5rem;
		font-size: 1.75rem;
		color: #1e293b;
	}

	.summary {
		margin: 0 0 2rem;
		color: #64748b;
		font-size: 1rem;
	}

	.feature-cards {
		display: grid;
		grid-template-columns: repeat(2, 1fr);
		gap: 1rem;
		margin-bottom: 2rem;
		text-align: left;
	}

	@media (max-width: 480px) {
		.feature-cards {
			grid-template-columns: 1fr;
		}
	}

	.feature-card {
		display: flex;
		flex-direction: column;
		padding: 1.25rem;
		background: white;
		border: 1px solid #e2e8f0;
		border-radius: 8px;
		text-decoration: none;
		color: inherit;
		transition: all 0.15s;
	}

	.feature-card:hover {
		border-color: #cbd5e1;
		box-shadow: 0 2px 8px rgba(0, 0, 0, 0.05);
	}

	.feature-card svg {
		width: 1.25rem;
		height: 1.25rem;
		color: #3b82f6;
		margin-bottom: 0.5rem;
	}

	.feature-card h3 {
		margin: 0 0 0.25rem;
		font-size: 0.875rem;
		color: #1e293b;
	}

	.feature-card p {
		margin: 0;
		font-size: 0.8125rem;
		color: #64748b;
	}

	.actions {
		display: flex;
		justify-content: center;
		gap: 0.75rem;
		flex-wrap: wrap;
	}

	.btn {
		display: inline-flex;
		align-items: center;
		gap: 0.5rem;
		padding: 0.75rem 1.5rem;
		border: 1px solid #cbd5e1;
		border-radius: 8px;
		font-size: 0.9375rem;
		font-weight: 500;
		cursor: pointer;
		transition: all 0.15s;
		text-decoration: none;
		font-family: inherit;
	}

	.btn-primary {
		background: #3b82f6;
		border-color: #3b82f6;
		color: white;
	}

	.btn-primary:hover {
		background: #2563eb;
	}

	.btn-secondary {
		background: white;
		color: #475569;
	}

	.btn-secondary:hover {
		background: #f1f5f9;
	}

	/* High contrast mode */
	:global(body.high-contrast) h2 {
		color: var(--color-text);
	}

	:global(body.high-contrast) .summary {
		color: var(--color-text-muted);
	}

	:global(body.high-contrast) .success-icon {
		background: var(--color-bg-secondary);
		color: var(--color-focus-ring);
	}

	:global(body.high-contrast) .feature-card {
		background: var(--color-bg-secondary);
		border-color: var(--color-border);
	}

	:global(body.high-contrast) .feature-card:hover {
		border-color: var(--color-focus-ring);
	}

	:global(body.high-contrast) .feature-card h3 {
		color: var(--color-text);
	}

	:global(body.high-contrast) .feature-card p {
		color: var(--color-text-muted);
	}

	:global(body.high-contrast) .btn-secondary {
		background: var(--color-bg-secondary);
		border-color: var(--color-border);
		color: var(--color-text);
	}

	:global(body.high-contrast) .btn-secondary:hover {
		background: var(--color-border);
	}
</style>
