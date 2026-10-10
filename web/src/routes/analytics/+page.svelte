<script lang="ts">
	import { api, type QualityOverview, type ResearchStatus } from '$lib/api/client';
	import QualityScore from '$lib/components/QualityScore.svelte';
	import PageHeader from '$lib/components/PageHeader.svelte';
	import QualityChart from '$lib/components/QualityChart.svelte';
	import UncertaintyBadge from '$lib/components/UncertaintyBadge.svelte';

	// Every figure here is aggregated by the server over the whole tree (#894);
	// the page only lays it out.
	let overview = $state<QualityOverview | null>(null);
	let totalFamilies = $state(0);
	let loading = $state(true);
	let error = $state<string | null>(null);

	const totalPersons = $derived(overview?.total_persons ?? 0);
	const issuesCounts = $derived(
		(overview?.top_issues ?? []).map((i) => ({ label: i.issue, value: i.count }))
	);

	// Bar colours match UncertaintyBadge's dots (Tailwind green/yellow/orange-500, gray-400).
	const researchStatusCounts = $derived.by(() => {
		const c = overview?.research_status_counts;
		return [
			{ status: 'certain', count: c?.certain ?? 0, color: '#22c55e' },
			{ status: 'probable', count: c?.probable ?? 0, color: '#eab308' },
			{ status: 'possible', count: c?.possible ?? 0, color: '#f97316' },
			{ status: 'unknown', count: c?.unknown ?? 0, color: '#9ca3af' },
			{ status: 'unset', count: c?.unset ?? 0, color: '#94a3b8' }
		] satisfies { status: ResearchStatus | 'unset'; count: number; color: string }[];
	});

	async function loadData() {
		loading = true;
		error = null;
		try {
			// The family count comes from the list total, as on the home page.
			const [overviewResult, familyResult] = await Promise.all([
				api.getQualityOverview(),
				api.listFamilies({ limit: 1 })
			]);
			overview = overviewResult;
			totalFamilies = familyResult.total;
		} catch (e) {
			console.error('Failed to load data:', e);
			error = 'Failed to load data. Please try again.';
		} finally {
			loading = false;
		}
	}

	$effect(() => {
		loadData();
	});

	const lowestScoringRecords = $derived(overview?.lowest_scoring ?? []);
</script>

<svelte:head>
	<title>Completeness | My Family</title>
</svelte:head>

<div class="analytics-page">
	<PageHeader title="Completeness">
		<p class="page-description">
			How complete each record is across the whole tree. For date conflicts and possible
			duplicates, see <a href="/quality">Quality</a>.
		</p>
	</PageHeader>

	{#if loading}
		<div class="loading">Loading completeness metrics...</div>
	{:else if error}
		<div class="error">{error}</div>
	{:else if overview}
		<!-- Overview Cards -->
		<section class="stat-cards">
			<div class="stat-card">
				<div class="stat-value">{totalPersons.toLocaleString()}</div>
				<div class="stat-label">Total Persons</div>
			</div>
			<div class="stat-card">
				<div class="stat-value">{totalFamilies.toLocaleString()}</div>
				<div class="stat-label">Total Families</div>
			</div>
			<div class="stat-card">
				<div class="stat-value-with-score">
					<QualityScore score={Math.round(overview.average_completeness)} size="large" />
				</div>
				<div class="stat-label">Overall Completeness</div>
			</div>
			<div class="stat-card" class:attention={overview.records_with_issues > 0}>
				<div class="stat-value">{overview.records_with_issues.toLocaleString()}</div>
				<div class="stat-label">Records With Issues</div>
			</div>
		</section>

		<!-- Quality Issues Chart -->
		{#if issuesCounts.length > 0}
			<section class="section">
				<h2>Most Common Issues</h2>
				<div class="chart-container">
					<QualityChart data={issuesCounts} />
				</div>
			</section>
		{/if}

		<!-- Research Status Distribution -->
		<section class="section">
			<h2>Research Confidence</h2>
			<p class="section-description">Distribution of research status across all persons</p>
			<div class="status-distribution">
				{#each researchStatusCounts as item}
					<div class="status-bar-row">
						<div class="status-label">
							{#if item.status !== 'unset'}
								<UncertaintyBadge status={item.status} size="small" showLabel={true} />
							{:else}
								<span class="unset-label">Not assessed</span>
							{/if}
						</div>
						<div class="status-bar-container">
							<div
								class="status-bar"
								style="width: {totalPersons > 0 ? (item.count / totalPersons) * 100 : 0}%; background-color: {item.color};"
							></div>
						</div>
						<div class="status-count">{item.count.toLocaleString()}</div>
					</div>
				{/each}
			</div>
		</section>

		<!-- Records Needing Attention Table -->
		{#if lowestScoringRecords.length > 0}
			<section class="section">
				<h2>Records Needing Attention</h2>
				<p class="section-description">Persons with the lowest completeness scores</p>
				<div class="table-container">
					<table class="records-table">
						<thead>
							<tr>
								<th>Name</th>
								<th>Score</th>
								<th>Issues</th>
							</tr>
						</thead>
						<tbody>
							{#each lowestScoringRecords as person (person.person_id)}
								<tr>
									<td>
										<a href="/persons/{person.person_id}" class="person-link">
											{person.given_name} {person.surname}
										</a>
									</td>
									<td>
										<QualityScore score={Math.round(person.completeness_score)} size="small" />
									</td>
									<td class="issues-cell">
										<ul class="issues-list">
											{#each person.issues as issue}
												<li>{issue}</li>
											{/each}
										</ul>
									</td>
								</tr>
							{/each}
						</tbody>
					</table>
				</div>
			</section>
		{:else if totalPersons === 0}
			<section class="section">
				<div class="empty-state">
					<p>No people yet. <a href="/import">Import a GEDCOM file</a> or add people to see data quality.</p>
				</div>
			</section>
		{:else}
			<section class="section">
				<div class="empty-state">
					<p>No quality issues found. Your data looks great!</p>
				</div>
			</section>
		{/if}
	{/if}
</div>

<style>
	.analytics-page {
		max-width: 1200px;
		margin: 0 auto;
		padding: 1.5rem;
	}

	.page-description {
		margin: 0.25rem 0 0;
		color: #64748b;
		font-size: 0.875rem;
	}

	.page-description a {
		color: #1d4ed8;
		text-decoration: underline;
	}

	.loading,
	.error {
		text-align: center;
		padding: 3rem;
		color: #64748b;
	}

	.error {
		color: #ef4444;
	}

	/* Stat Cards */
	.stat-cards {
		display: grid;
		grid-template-columns: repeat(auto-fit, minmax(200px, 1fr));
		gap: 1rem;
		margin-bottom: 2rem;
	}

	.stat-card {
		background: white;
		border: 1px solid #e2e8f0;
		border-radius: 8px;
		padding: 1.5rem;
	}

	.stat-card.attention {
		border-color: #fbbf24;
		background: #fffbeb;
	}

	.stat-value {
		font-size: 2rem;
		font-weight: 600;
		color: #1e293b;
	}

	.stat-value-with-score {
		display: flex;
		align-items: center;
	}

	.stat-label {
		color: #64748b;
		font-size: 0.875rem;
		margin-top: 0.25rem;
	}

	/* Sections */
	.section {
		background: white;
		border: 1px solid #e2e8f0;
		border-radius: 8px;
		padding: 1.5rem;
		margin-bottom: 1.5rem;
	}

	.section h2 {
		margin: 0 0 0.5rem;
		font-size: 1.125rem;
		color: #1e293b;
	}

	.section-description {
		margin: 0 0 1rem;
		color: #64748b;
		font-size: 0.875rem;
	}

	.chart-container {
		padding: 1rem 0;
	}

	/* Table */
	.table-container {
		overflow-x: auto;
	}

	.records-table {
		width: 100%;
		border-collapse: collapse;
		font-size: 0.875rem;
	}

	.records-table th,
	.records-table td {
		padding: 0.75rem 1rem;
		text-align: left;
		border-bottom: 1px solid #e2e8f0;
	}

	.records-table th {
		font-weight: 600;
		color: #475569;
		background: #f8fafc;
	}

	.records-table tbody tr:hover {
		background: #f8fafc;
	}

	.person-link {
		color: #3b82f6;
		text-decoration: none;
		font-weight: 500;
	}

	.person-link:hover {
		text-decoration: underline;
	}

	.issues-cell {
		max-width: 300px;
	}

	.issues-list {
		margin: 0;
		padding-left: 1rem;
		color: #64748b;
		font-size: 0.8125rem;
	}

	.issues-list li {
		margin: 0.125rem 0;
	}

	.empty-state {
		text-align: center;
		padding: 2rem;
		color: #64748b;
	}

	/* Research Status Distribution */
	.status-distribution {
		display: flex;
		flex-direction: column;
		gap: 0.75rem;
	}

	.status-bar-row {
		display: flex;
		align-items: center;
		gap: 1rem;
	}

	.status-label {
		width: 120px;
		flex-shrink: 0;
	}

	.unset-label {
		font-size: 0.75rem;
		color: #94a3b8;
		font-weight: 500;
	}

	.status-bar-container {
		flex: 1;
		height: 1.25rem;
		background: #f1f5f9;
		border-radius: 4px;
		overflow: hidden;
	}

	.status-bar {
		height: 100%;
		border-radius: 4px;
		transition: width 0.3s ease;
		min-width: 2px;
	}

	.status-count {
		width: 3rem;
		text-align: right;
		font-size: 0.875rem;
		font-weight: 500;
		color: #475569;
	}
</style>
