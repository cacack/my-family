<script lang="ts">
	import { page } from '$app/stores';
	import { api, type AhnentafelResponse, type AhnentafelEntry, formatGenDate } from '$lib/api/client';

	let report: AhnentafelResponse | null = $state(null);
	let error: string | null = $state(null);
	let loading = $state(true);
	let generations = $state(4);
	let exporting = $state(false);

	async function loadReport(personId: string, gens: number) {
		loading = true;
		error = null;
		try {
			report = await api.getAhnentafel(personId, gens);
		} catch (e) {
			error = (e as { message?: string }).message || 'Failed to load Ahnentafel report';
			report = null;
		} finally {
			loading = false;
		}
	}

	function handleGenerationsChange(e: Event) {
		const select = e.target as HTMLSelectElement;
		generations = parseInt(select.value, 10);
		const personId = $page.params.id;
		if (personId) {
			loadReport(personId, generations);
		}
	}

	function handlePrint() {
		window.print();
	}

	async function handleExportText() {
		const personId = $page.params.id;
		if (!personId) return;

		exporting = true;
		try {
			const text = await api.getAhnentafelText(personId, generations);
			const blob = new Blob([text], { type: 'text/plain;charset=utf-8' });
			const url = URL.createObjectURL(blob);
			const link = document.createElement('a');
			link.href = url;
			const subjectName = report?.subject
				? `${report.subject.given_name}_${report.subject.surname}`
				: 'ahnentafel';
			link.download = `${subjectName.replace(/\s+/g, '_')}_ahnentafel.txt`;
			document.body.appendChild(link);
			link.click();
			document.body.removeChild(link);
			URL.revokeObjectURL(url);
		} catch (e) {
			error = (e as { message?: string }).message || 'Failed to export report';
		} finally {
			exporting = false;
		}
	}

	function formatBirthDeath(entry: AhnentafelEntry): string {
		const birth = entry.birth_date ? formatGenDate(entry.birth_date) : '';
		const birthPlace = entry.birth_place || '';
		const death = entry.death_date ? formatGenDate(entry.death_date) : '';
		const deathPlace = entry.death_place || '';

		const birthStr = birth || birthPlace ? `b. ${birth}${birthPlace ? ` ${birthPlace}` : ''}` : '';
		const deathStr = death || deathPlace ? `d. ${death}${deathPlace ? ` ${deathPlace}` : ''}` : '';

		if (birthStr && deathStr) return `${birthStr}; ${deathStr}`;
		return birthStr || deathStr || '';
	}

	function getGenerationLabel(gen: number): string {
		const labels: Record<number, string> = {
			0: 'Subject',
			1: 'Parents',
			2: 'Grandparents',
			3: 'Great-Grandparents',
			4: '2nd Great-Grandparents',
			5: '3rd Great-Grandparents',
			6: '4th Great-Grandparents',
			7: '5th Great-Grandparents',
			8: '6th Great-Grandparents',
			9: '7th Great-Grandparents',
			10: '8th Great-Grandparents'
		};
		return labels[gen] || `${gen - 2}th Great-Grandparents`;
	}

	function groupByGeneration(entries: AhnentafelEntry[]): Map<number, AhnentafelEntry[]> {
		const groups = new Map<number, AhnentafelEntry[]>();
		for (const entry of entries) {
			const existing = groups.get(entry.generation) || [];
			existing.push(entry);
			groups.set(entry.generation, existing);
		}
		return groups;
	}

	$effect(() => {
		const personId = $page.params.id;
		if (personId) {
			loadReport(personId, generations);
		}
	});
</script>

<svelte:head>
	<title>
		{report?.subject ? `${report.subject.given_name} ${report.subject.surname}` : 'Person'} - Ahnentafel Report | My Family
	</title>
</svelte:head>

<div class="ahnentafel-page">
	<header class="page-header no-print">
		<div class="header-left">
			{#if report?.subject}
				<a href="/persons/{report.subject.id}" class="back-link">&larr; Back to {report.subject.given_name} {report.subject.surname}</a>
			{:else}
				<a href="/" class="back-link">&larr; Back</a>
			{/if}
		</div>
		<div class="controls">
			<label>
				Generations:
				<select value={generations} onchange={handleGenerationsChange}>
					{#each Array.from({ length: 9 }, (_, i) => i + 2) as n}
						<option value={n}>{n}</option>
					{/each}
				</select>
			</label>
			<button class="btn" onclick={handlePrint} disabled={loading || !!error}>
				Print
			</button>
			<button class="btn" onclick={handleExportText} disabled={loading || !!error || exporting}>
				{exporting ? 'Exporting...' : 'Export Text'}
			</button>
		</div>
	</header>

	<main class="report-container">
		{#if loading}
			<div class="loading">Loading report...</div>
		{:else if error}
			<div class="error">{error}</div>
		{:else if report}
			<div class="report">
				<div class="report-header">
					<h1>Ahnentafel Report</h1>
					<h2>{report.subject.given_name} {report.subject.surname}</h2>
					<p class="report-meta">
						{report.generations} generations - {report.known_count} of {report.total_count} ancestors known
					</p>
				</div>

				<!-- Desktop table view -->
				<div class="table-view">
					{#each [...groupByGeneration(report.entries)] as [gen, entries]}
						<div class="generation-group" class:gen-even={gen % 2 === 0}>
							<h3 class="generation-header">
								{getGenerationLabel(gen)}
								<span class="generation-count">({entries.filter(e => e.id).length}/{entries.length})</span>
							</h3>
							<table class="ahnentafel-table">
								<thead>
									<tr>
										<th class="col-number">#</th>
										<th class="col-name">Name</th>
										<th class="col-relationship">Relationship</th>
										<th class="col-birth">Birth</th>
										<th class="col-death">Death</th>
									</tr>
								</thead>
								<tbody>
									{#each entries as entry}
										<tr class:unknown={!entry.id} class:male={entry.gender === 'male'} class:female={entry.gender === 'female'}>
											<td class="col-number">{entry.number}</td>
											<td class="col-name">
												{#if entry.id}
													<a href="/persons/{entry.id}" class="person-link">
														{entry.given_name || '?'} {entry.surname || '?'}
													</a>
												{:else}
													<span class="unknown-person">Unknown</span>
												{/if}
											</td>
											<td class="col-relationship">{entry.relationship}</td>
											<td class="col-birth">
												{#if entry.birth_date || entry.birth_place}
													<span class="date">{entry.birth_date ? formatGenDate(entry.birth_date) : ''}</span>
													{#if entry.birth_place}
														<span class="place">{entry.birth_place}</span>
													{/if}
												{:else}
													<span class="unknown-data">-</span>
												{/if}
											</td>
											<td class="col-death">
												{#if entry.death_date || entry.death_place}
													<span class="date">{entry.death_date ? formatGenDate(entry.death_date) : ''}</span>
													{#if entry.death_place}
														<span class="place">{entry.death_place}</span>
													{/if}
												{:else}
													<span class="unknown-data">-</span>
												{/if}
											</td>
										</tr>
									{/each}
								</tbody>
							</table>
						</div>
					{/each}
				</div>

				<!-- Mobile card view -->
				<div class="card-view">
					{#each [...groupByGeneration(report.entries)] as [gen, entries]}
						<div class="generation-group" class:gen-even={gen % 2 === 0}>
							<h3 class="generation-header">
								{getGenerationLabel(gen)}
								<span class="generation-count">({entries.filter(e => e.id).length}/{entries.length})</span>
							</h3>
							<div class="card-list">
								{#each entries as entry}
									<div class="ancestor-card" class:unknown={!entry.id} class:male={entry.gender === 'male'} class:female={entry.gender === 'female'}>
										<div class="card-header">
											<span class="card-number">{entry.number}</span>
											<span class="card-relationship">{entry.relationship}</span>
										</div>
										<div class="card-name">
											{#if entry.id}
												<a href="/persons/{entry.id}" class="person-link">
													{entry.given_name || '?'} {entry.surname || '?'}
												</a>
											{:else}
												<span class="unknown-person">Unknown</span>
											{/if}
										</div>
										{#if entry.id && (entry.birth_date || entry.birth_place || entry.death_date || entry.death_place)}
											<div class="card-details">
												{#if entry.birth_date || entry.birth_place}
													<div class="card-event">
														<span class="event-label">b.</span>
														<span class="event-value">
															{entry.birth_date ? formatGenDate(entry.birth_date) : ''}
															{entry.birth_place || ''}
														</span>
													</div>
												{/if}
												{#if entry.death_date || entry.death_place}
													<div class="card-event">
														<span class="event-label">d.</span>
														<span class="event-value">
															{entry.death_date ? formatGenDate(entry.death_date) : ''}
															{entry.death_place || ''}
														</span>
													</div>
												{/if}
											</div>
										{/if}
									</div>
								{/each}
							</div>
						</div>
					{/each}
				</div>

				<footer class="report-footer print-only">
					<p>Generated on {new Date().toLocaleDateString()}</p>
				</footer>
			</div>
		{:else}
			<div class="empty">No data available.</div>
		{/if}
	</main>
</div>

<style>
	.ahnentafel-page {
		display: flex;
		flex-direction: column;
		min-height: 100vh;
		background: #f8fafc;
	}

	.page-header {
		display: flex;
		justify-content: space-between;
		align-items: center;
		padding: 1rem 1.5rem;
		background: white;
		border-bottom: 1px solid #e2e8f0;
	}

	.header-left {
		display: flex;
		align-items: center;
		gap: 1rem;
	}

	.back-link {
		color: #64748b;
		text-decoration: none;
		font-size: 0.875rem;
	}

	.back-link:hover {
		color: #3b82f6;
	}

	.controls {
		display: flex;
		align-items: center;
		gap: 1rem;
	}

	.controls label {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		font-size: 0.875rem;
		color: #475569;
	}

	.controls select {
		padding: 0.375rem 0.75rem;
		border: 1px solid #cbd5e1;
		border-radius: 6px;
		background: white;
		font-size: 0.875rem;
	}

	.btn {
		padding: 0.5rem 1rem;
		border: 1px solid #cbd5e1;
		border-radius: 6px;
		background: white;
		font-size: 0.875rem;
		cursor: pointer;
		color: #475569;
	}

	.btn:hover:not(:disabled) {
		background: #f1f5f9;
	}

	.btn:disabled {
		opacity: 0.5;
		cursor: not-allowed;
	}

	.report-container {
		flex: 1;
		padding: 1.5rem;
		max-width: 1200px;
		margin: 0 auto;
		width: 100%;
	}

	.loading,
	.error,
	.empty {
		display: flex;
		align-items: center;
		justify-content: center;
		height: 200px;
		color: #64748b;
		font-size: 1rem;
	}

	.error {
		color: #dc2626;
	}

	.report {
		background: white;
		border-radius: 12px;
		border: 1px solid #e2e8f0;
		overflow: hidden;
	}

	.report-header {
		padding: 1.5rem;
		text-align: center;
		border-bottom: 1px solid #e2e8f0;
		background: #f8fafc;
	}

	.report-header h1 {
		margin: 0 0 0.5rem;
		font-size: 1.5rem;
		color: #1e293b;
		font-weight: 600;
	}

	.report-header h2 {
		margin: 0 0 0.5rem;
		font-size: 1.25rem;
		color: #475569;
		font-weight: 500;
	}

	.report-meta {
		margin: 0;
		font-size: 0.875rem;
		color: #64748b;
	}

	.generation-group {
		border-bottom: 1px solid #e2e8f0;
	}

	.generation-group:last-child {
		border-bottom: none;
	}

	.generation-group.gen-even {
		background: #fafafa;
	}

	.generation-header {
		margin: 0;
		padding: 0.75rem 1rem;
		font-size: 0.9375rem;
		font-weight: 600;
		color: #1e293b;
		background: #f1f5f9;
		border-bottom: 1px solid #e2e8f0;
		display: flex;
		align-items: center;
		gap: 0.5rem;
	}

	.generation-count {
		font-size: 0.8125rem;
		font-weight: 400;
		color: #64748b;
	}

	.ahnentafel-table {
		width: 100%;
		border-collapse: collapse;
		font-size: 0.875rem;
	}

	.ahnentafel-table th {
		padding: 0.625rem 0.75rem;
		text-align: left;
		font-weight: 600;
		color: #475569;
		background: #f8fafc;
		border-bottom: 1px solid #e2e8f0;
	}

	.ahnentafel-table td {
		padding: 0.5rem 0.75rem;
		border-bottom: 1px solid #f1f5f9;
		color: #1e293b;
		vertical-align: top;
	}

	.ahnentafel-table tr:last-child td {
		border-bottom: none;
	}

	.col-number {
		width: 50px;
		text-align: center;
		font-weight: 600;
	}

	.col-name {
		min-width: 150px;
	}

	.col-relationship {
		width: 180px;
		color: #64748b;
	}

	.col-birth,
	.col-death {
		width: 200px;
	}

	.person-link {
		color: #2563eb;
		text-decoration: none;
		font-weight: 500;
	}

	.person-link:hover {
		text-decoration: underline;
	}

	.unknown-person {
		color: #94a3b8;
		font-style: italic;
	}

	.unknown-data {
		color: #cbd5e1;
	}

	tr.unknown {
		opacity: 0.7;
	}

	tr.male .col-number {
		color: #3b82f6;
	}

	tr.female .col-number {
		color: #ec4899;
	}

	.date {
		display: block;
	}

	.place {
		display: block;
		font-size: 0.8125rem;
		color: #64748b;
	}

	/* Mobile card view */
	.card-view {
		display: none;
	}

	.card-list {
		display: grid;
		grid-template-columns: repeat(auto-fill, minmax(280px, 1fr));
		gap: 0.75rem;
		padding: 0.75rem;
	}

	.ancestor-card {
		background: white;
		border: 1px solid #e2e8f0;
		border-radius: 8px;
		padding: 0.75rem;
		box-shadow: 0 1px 2px rgba(0, 0, 0, 0.05);
	}

	.ancestor-card.unknown {
		opacity: 0.7;
		background: #f8fafc;
	}

	.ancestor-card.male {
		border-left: 3px solid #3b82f6;
	}

	.ancestor-card.female {
		border-left: 3px solid #ec4899;
	}

	.card-header {
		display: flex;
		justify-content: space-between;
		align-items: center;
		margin-bottom: 0.375rem;
	}

	.card-number {
		font-weight: 700;
		font-size: 0.9375rem;
		color: #64748b;
	}

	.card-relationship {
		font-size: 0.75rem;
		color: #94a3b8;
	}

	.card-name {
		font-size: 1rem;
		font-weight: 500;
		margin-bottom: 0.5rem;
	}

	.card-details {
		font-size: 0.8125rem;
		color: #64748b;
	}

	.card-event {
		display: flex;
		gap: 0.25rem;
		margin-top: 0.25rem;
	}

	.event-label {
		color: #94a3b8;
		flex-shrink: 0;
	}

	.event-value {
		color: #475569;
	}

	.report-footer {
		padding: 1rem;
		text-align: center;
		font-size: 0.75rem;
		color: #94a3b8;
		border-top: 1px solid #e2e8f0;
	}

	.print-only {
		display: none;
	}

	/* Responsive design */
	@media (max-width: 768px) {
		.page-header {
			flex-direction: column;
			gap: 1rem;
			align-items: flex-start;
		}

		.controls {
			width: 100%;
			flex-wrap: wrap;
		}

		.table-view {
			display: none;
		}

		.card-view {
			display: block;
		}
	}

	/* Print styles */
	@media print {
		.no-print {
			display: none !important;
		}

		.print-only {
			display: block !important;
		}

		.ahnentafel-page {
			background: white;
		}

		.report-container {
			padding: 0;
			max-width: none;
		}

		.report {
			border: none;
			border-radius: 0;
		}

		.report-header {
			background: white;
			border-bottom: 2px solid #1e293b;
			padding: 0 0 1rem 0;
			margin-bottom: 1rem;
		}

		.report-header h1 {
			font-size: 18pt;
		}

		.report-header h2 {
			font-size: 14pt;
		}

		.generation-group {
			page-break-inside: avoid;
		}

		.generation-group.gen-even {
			background: white;
		}

		.generation-header {
			background: #f0f0f0;
			font-size: 11pt;
			padding: 0.5rem;
		}

		.ahnentafel-table {
			font-size: 10pt;
		}

		.ahnentafel-table th,
		.ahnentafel-table td {
			padding: 0.25rem 0.5rem;
		}

		.person-link {
			color: black;
			text-decoration: none;
		}

		.card-view {
			display: none !important;
		}

		.table-view {
			display: block !important;
		}

		.place {
			font-size: 9pt;
		}
	}
</style>
