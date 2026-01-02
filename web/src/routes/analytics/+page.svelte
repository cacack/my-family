<script lang="ts">
	import { api, type Person, type FamilyDetail } from '$lib/api/client';
	import QualityScore from '$lib/components/QualityScore.svelte';
	import QualityChart from '$lib/components/QualityChart.svelte';

	interface PersonWithScore extends Person {
		qualityScore: number;
		issues: string[];
	}

	let persons: Person[] = $state([]);
	let families: FamilyDetail[] = $state([]);
	let loading = $state(true);
	let error = $state<string | null>(null);

	// Computed quality data
	let personsWithScores: PersonWithScore[] = $state([]);
	let overallScore = $state(0);
	let issuesCounts = $state<{ label: string; value: number; color: string }[]>([]);

	// Stats
	let totalPersons = $derived(persons.length);
	let totalFamilies = $derived(families.length);
	let recordsNeedingAttention = $derived(personsWithScores.filter((p) => p.qualityScore < 50).length);

	function computePersonScore(person: Person): { score: number; issues: string[] } {
		let score = 0;
		const issues: string[] = [];
		const currentYear = new Date().getFullYear();

		// Has birth date: +20 points
		if (person.birth_date?.year) {
			score += 20;
		} else {
			issues.push('Missing birth date');
		}

		// Has birth place: +15 points
		if (person.birth_place) {
			score += 15;
		} else {
			issues.push('Missing birth place');
		}

		// For death info, only score if person likely deceased
		const likelyDeceased =
			person.birth_date?.year && currentYear - person.birth_date.year > 100;

		if (person.death_date?.year) {
			score += 20;
		} else if (!likelyDeceased) {
			// Living person, no death expected
			score += 20;
		} else {
			issues.push('Missing death date (likely deceased)');
		}

		if (person.death_place) {
			score += 15;
		} else if (person.death_date?.year) {
			// Only mark as issue if they have a death date but no place
			issues.push('Missing death place');
		} else if (!likelyDeceased) {
			// Living person, no death place expected
			score += 15;
		}

		// Base score is out of 70, normalize to 100
		return {
			score: Math.round((score / 70) * 100),
			issues
		};
	}

	function isOrphaned(person: Person, allFamilies: FamilyDetail[]): boolean {
		// Check if person appears in any family as partner or child
		for (const family of allFamilies) {
			if (family.partner1_id === person.id || family.partner2_id === person.id) {
				return false;
			}
			if (family.children?.some((c) => c.id === person.id)) {
				return false;
			}
		}
		return true;
	}

	function computeAllScores() {
		const currentYear = new Date().getFullYear();

		personsWithScores = persons.map((person) => {
			const { score, issues } = computePersonScore(person);

			// Add orphan check
			if (isOrphaned(person, families)) {
				issues.push('No family connections');
			}

			return {
				...person,
				qualityScore: score,
				issues
			};
		});

		// Sort by score ascending (lowest first)
		personsWithScores.sort((a, b) => a.qualityScore - b.qualityScore);

		// Calculate overall average score
		if (personsWithScores.length > 0) {
			const totalScore = personsWithScores.reduce((sum, p) => sum + p.qualityScore, 0);
			overallScore = Math.round(totalScore / personsWithScores.length);
		}

		// Calculate issue counts
		let missingBirthDate = 0;
		let missingBirthPlace = 0;
		let missingDeathInfo = 0;
		let orphanedPersons = 0;

		for (const person of personsWithScores) {
			if (person.issues.includes('Missing birth date')) missingBirthDate++;
			if (person.issues.includes('Missing birth place')) missingBirthPlace++;
			if (
				person.issues.includes('Missing death date (likely deceased)') ||
				person.issues.includes('Missing death place')
			)
				missingDeathInfo++;
			if (person.issues.includes('No family connections')) orphanedPersons++;
		}

		issuesCounts = [
			{ label: 'Missing birth date', value: missingBirthDate, color: '#ef4444' },
			{ label: 'Missing birth place', value: missingBirthPlace, color: '#f97316' },
			{ label: 'Missing death info', value: missingDeathInfo, color: '#eab308' },
			{ label: 'No family connections', value: orphanedPersons, color: '#8b5cf6' }
		].filter((item) => item.value > 0);
	}

	async function loadData() {
		loading = true;
		error = null;
		try {
			// Fetch all persons (paginated if needed)
			const personResult = await api.listPersons({ limit: 1000 });
			persons = personResult.items;

			// Fetch all families
			const familyResult = await api.listFamilies({ limit: 1000 });
			families = familyResult.items;

			computeAllScores();
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

	// Get top 20 lowest scoring records
	const lowestScoringRecords = $derived(personsWithScores.slice(0, 20));
</script>

<svelte:head>
	<title>Data Quality | My Family</title>
</svelte:head>

<div class="analytics-page">
	<header class="page-header">
		<h1>Data Quality</h1>
	</header>

	{#if loading}
		<div class="loading">Loading data quality metrics...</div>
	{:else if error}
		<div class="error">{error}</div>
	{:else}
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
					<QualityScore score={overallScore} size="large" />
				</div>
				<div class="stat-label">Overall Completeness</div>
			</div>
			<div class="stat-card" class:attention={recordsNeedingAttention > 0}>
				<div class="stat-value">{recordsNeedingAttention.toLocaleString()}</div>
				<div class="stat-label">Records Needing Attention</div>
			</div>
		</section>

		<!-- Quality Issues Chart -->
		{#if issuesCounts.length > 0}
			<section class="section">
				<h2>Quality Issues</h2>
				<div class="chart-container">
					<QualityChart data={issuesCounts} />
				</div>
			</section>
		{/if}

		<!-- Records Needing Attention Table -->
		{#if lowestScoringRecords.length > 0}
			<section class="section">
				<h2>Records Needing Attention</h2>
				<p class="section-description">Persons with lowest quality scores</p>
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
							{#each lowestScoringRecords as person}
								<tr>
									<td>
										<a href="/persons/{person.id}" class="person-link">
											{person.given_name} {person.surname}
										</a>
									</td>
									<td>
										<QualityScore score={person.qualityScore} size="small" />
									</td>
									<td class="issues-cell">
										{#if person.issues.length > 0}
											<ul class="issues-list">
												{#each person.issues as issue}
													<li>{issue}</li>
												{/each}
											</ul>
										{:else}
											<span class="no-issues">No major issues</span>
										{/if}
									</td>
								</tr>
							{/each}
						</tbody>
					</table>
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

	.page-header {
		margin-bottom: 1.5rem;
	}

	.page-header h1 {
		margin: 0;
		font-size: 1.5rem;
		color: #1e293b;
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

	.no-issues {
		color: #22c55e;
		font-size: 0.8125rem;
	}

	.empty-state {
		text-align: center;
		padding: 2rem;
		color: #64748b;
	}
</style>
