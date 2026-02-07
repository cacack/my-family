<script lang="ts">
	interface Props {
		onSelectImport: () => void;
		onSelectCreate: () => void;
		onSkip: () => void;
	}

	let { onSelectImport, onSelectCreate, onSkip }: Props = $props();
</script>

<div class="welcome-step">
	<div class="hero">
		<svg class="hero-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5">
			<path d="M17 21v-2a4 4 0 0 0-4-4H5a4 4 0 0 0-4 4v2" />
			<circle cx="9" cy="7" r="4" />
			<path d="M23 21v-2a4 4 0 0 0-3-3.87" />
			<path d="M16 3.13a4 4 0 0 1 0 7.75" />
		</svg>
		<h1>Welcome to My Family</h1>
		<p class="subtitle">Your self-hosted genealogy platform. Preserve your family history with
			research-grade rigor and engaging storytelling.</p>
	</div>

	<div class="action-cards">
		<button class="action-card" onclick={onSelectImport}>
			<div class="card-icon">
				<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
					<path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4" />
					<polyline points="17 8 12 3 7 8" />
					<line x1="12" y1="3" x2="12" y2="15" />
				</svg>
			</div>
			<h2>Import GEDCOM</h2>
			<p>Already have family tree data? Import a GEDCOM file to get started quickly.</p>
		</button>

		<button class="action-card" onclick={onSelectCreate}>
			<div class="card-icon">
				<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
					<path d="M16 21v-2a4 4 0 0 0-4-4H6a4 4 0 0 0-4 4v2" />
					<circle cx="9" cy="7" r="4" />
					<line x1="19" y1="8" x2="19" y2="14" />
					<line x1="16" y1="11" x2="22" y2="11" />
				</svg>
			</div>
			<h2>Start from Scratch</h2>
			<p>New to genealogy? Create your first person and begin building your family tree.</p>
		</button>
	</div>

	<div class="features">
		<div class="feature">
			<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
				<path d="M12 22s8-4 8-10V5l-8-3-8 3v7c0 6 8 10 8 10z" />
			</svg>
			<div>
				<h3>Self-Hosted</h3>
				<p>Your data stays on your server, always under your control.</p>
			</div>
		</div>
		<div class="feature">
			<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
				<circle cx="12" cy="12" r="10" />
				<polyline points="12 6 12 12 16 14" />
			</svg>
			<div>
				<h3>Full Audit Trail</h3>
				<p>Every change is tracked with event sourcing for complete history.</p>
			</div>
		</div>
		<div class="feature">
			<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
				<polygon points="12 2 15.09 8.26 22 9.27 17 14.14 18.18 21.02 12 17.77 5.82 21.02 7 14.14 2 9.27 8.91 8.26 12 2" />
			</svg>
			<div>
				<h3>Research Grade</h3>
				<p>GPS-compliant with source citations and evidence tracking.</p>
			</div>
		</div>
	</div>

	<button class="skip-link" onclick={onSkip}>Skip setup and go to dashboard</button>
</div>

<style>
	.welcome-step {
		max-width: 640px;
		margin: 0 auto;
		text-align: center;
	}

	.hero {
		margin-bottom: 2rem;
	}

	.hero-icon {
		width: 4rem;
		height: 4rem;
		color: #3b82f6;
		margin-bottom: 1rem;
	}

	.hero h1 {
		margin: 0;
		font-size: 2rem;
		color: #1e293b;
	}

	.subtitle {
		margin: 0.75rem 0 0;
		color: #64748b;
		font-size: 1.0625rem;
		line-height: 1.6;
	}

	.action-cards {
		display: grid;
		grid-template-columns: repeat(2, 1fr);
		gap: 1rem;
		margin-bottom: 2rem;
	}

	@media (max-width: 480px) {
		.action-cards {
			grid-template-columns: 1fr;
		}
	}

	.action-card {
		display: flex;
		flex-direction: column;
		align-items: center;
		padding: 1.5rem;
		background: white;
		border: 2px solid #e2e8f0;
		border-radius: 12px;
		cursor: pointer;
		transition: all 0.15s;
		text-align: center;
		font-family: inherit;
	}

	.action-card:hover {
		border-color: #3b82f6;
		box-shadow: 0 4px 12px rgba(59, 130, 246, 0.15);
	}

	.action-card:focus-visible {
		outline: 2px solid #3b82f6;
		outline-offset: 2px;
	}

	.card-icon {
		width: 3rem;
		height: 3rem;
		display: flex;
		align-items: center;
		justify-content: center;
		background: #eff6ff;
		border-radius: 12px;
		color: #3b82f6;
		margin-bottom: 0.75rem;
	}

	.card-icon svg {
		width: 1.5rem;
		height: 1.5rem;
	}

	.action-card h2 {
		margin: 0 0 0.5rem;
		font-size: 1rem;
		color: #1e293b;
	}

	.action-card p {
		margin: 0;
		font-size: 0.8125rem;
		color: #64748b;
		line-height: 1.5;
	}

	.features {
		display: flex;
		flex-direction: column;
		gap: 1rem;
		margin-bottom: 2rem;
		text-align: left;
	}

	.feature {
		display: flex;
		align-items: flex-start;
		gap: 0.75rem;
		padding: 0.75rem 1rem;
		background: #f8fafc;
		border-radius: 8px;
	}

	.feature svg {
		flex-shrink: 0;
		width: 1.25rem;
		height: 1.25rem;
		color: #3b82f6;
		margin-top: 0.125rem;
	}

	.feature h3 {
		margin: 0;
		font-size: 0.875rem;
		color: #1e293b;
	}

	.feature p {
		margin: 0.125rem 0 0;
		font-size: 0.8125rem;
		color: #64748b;
	}

	.skip-link {
		background: none;
		border: none;
		padding: 0.5rem;
		font-size: 0.8125rem;
		color: #94a3b8;
		cursor: pointer;
		text-decoration: underline;
		font-family: inherit;
	}

	.skip-link:hover {
		color: #64748b;
	}

	/* High contrast mode */
	:global(body.high-contrast) .hero h1 {
		color: var(--color-text);
	}

	:global(body.high-contrast) .subtitle {
		color: var(--color-text-muted);
	}

	:global(body.high-contrast) .action-card {
		background: var(--color-bg-secondary);
		border-color: var(--color-border);
		color: var(--color-text);
	}

	:global(body.high-contrast) .action-card:hover {
		border-color: var(--color-focus-ring);
	}

	:global(body.high-contrast) .action-card h2 {
		color: var(--color-text);
	}

	:global(body.high-contrast) .action-card p {
		color: var(--color-text-muted);
	}

	:global(body.high-contrast) .card-icon {
		background: var(--color-border);
		color: var(--color-focus-ring);
	}

	:global(body.high-contrast) .feature {
		background: var(--color-bg-secondary);
	}

	:global(body.high-contrast) .feature h3 {
		color: var(--color-text);
	}

	:global(body.high-contrast) .feature p {
		color: var(--color-text-muted);
	}

	:global(body.high-contrast) .skip-link {
		color: var(--color-text-muted);
	}

	:global(body.high-contrast) .skip-link:hover {
		color: var(--color-text);
	}
</style>
