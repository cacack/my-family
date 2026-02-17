<script lang="ts">
	import type { FamilyGroupSheet, GroupSheetPerson, GroupSheetChild, GroupSheetEvent } from '$lib/api/client';

	interface Props {
		data: FamilyGroupSheet;
	}

	let { data }: Props = $props();

	function formatPersonName(person: GroupSheetPerson | GroupSheetChild): string {
		return `${person.given_name} ${person.surname}`;
	}

	function formatEventInfo(event: GroupSheetEvent | undefined): string {
		if (!event) return '';
		if (event.is_negated) return '';
		const parts: string[] = [];
		if (event.date) parts.push(event.date);
		if (event.place) parts.push(event.place);
		return parts.join(' - ');
	}

	function isNegated(event: GroupSheetEvent | undefined): boolean {
		return !!event?.is_negated;
	}

	function hasCitations(event: GroupSheetEvent | undefined): boolean {
		return !!(event?.citations && event.citations.length > 0);
	}
</script>

<div class="group-sheet">
	<header class="sheet-header">
		<h1>Family Group Sheet</h1>
	</header>

	<!-- Husband/Father Section -->
	<section class="person-section husband-section">
		<h2 class="section-title">Husband / Father</h2>
		{#if data.husband}
			<div class="person-info">
				<div class="info-row name-row">
					<span class="label">Name:</span>
					<a href="/persons/{data.husband.id}" class="person-link">
						{formatPersonName(data.husband)}
					</a>
				</div>

				<div class="events-grid">
					<div class="event-row">
						<span class="event-label">Birth:</span>
						<span class="event-value">
							{#if isNegated(data.husband.birth)}
								<span class="negated-event">No birth recorded</span>
							{:else}
								{formatEventInfo(data.husband.birth) || 'Unknown'}
							{/if}
							{#if hasCitations(data.husband.birth)}
								<span class="citation-indicator" title="Has citations">[{data.husband.birth?.citations?.length}]</span>
							{/if}
						</span>
					</div>
					<div class="event-row">
						<span class="event-label">Death:</span>
						<span class="event-value">
							{#if isNegated(data.husband.death)}
								<span class="negated-event">No death recorded</span>
							{:else}
								{formatEventInfo(data.husband.death) || 'Unknown'}
							{/if}
							{#if hasCitations(data.husband.death)}
								<span class="citation-indicator" title="Has citations">[{data.husband.death?.citations?.length}]</span>
							{/if}
						</span>
					</div>
				</div>

				{#if data.husband.father_name || data.husband.mother_name}
					<div class="parents-info">
						<div class="parent-row">
							<span class="label">Father:</span>
							{#if data.husband.father_id}
								<a href="/persons/{data.husband.father_id}">{data.husband.father_name}</a>
							{:else}
								<span>{data.husband.father_name || 'Unknown'}</span>
							{/if}
						</div>
						<div class="parent-row">
							<span class="label">Mother:</span>
							{#if data.husband.mother_id}
								<a href="/persons/{data.husband.mother_id}">{data.husband.mother_name}</a>
							{:else}
								<span>{data.husband.mother_name || 'Unknown'}</span>
							{/if}
						</div>
					</div>
				{/if}
			</div>
		{:else}
			<p class="no-data">No husband/father recorded</p>
		{/if}
	</section>

	<!-- Wife/Mother Section -->
	<section class="person-section wife-section">
		<h2 class="section-title">Wife / Mother</h2>
		{#if data.wife}
			<div class="person-info">
				<div class="info-row name-row">
					<span class="label">Name:</span>
					<a href="/persons/{data.wife.id}" class="person-link">
						{formatPersonName(data.wife)}
					</a>
				</div>

				<div class="events-grid">
					<div class="event-row">
						<span class="event-label">Birth:</span>
						<span class="event-value">
							{#if isNegated(data.wife.birth)}
								<span class="negated-event">No birth recorded</span>
							{:else}
								{formatEventInfo(data.wife.birth) || 'Unknown'}
							{/if}
							{#if hasCitations(data.wife.birth)}
								<span class="citation-indicator" title="Has citations">[{data.wife.birth?.citations?.length}]</span>
							{/if}
						</span>
					</div>
					<div class="event-row">
						<span class="event-label">Death:</span>
						<span class="event-value">
							{#if isNegated(data.wife.death)}
								<span class="negated-event">No death recorded</span>
							{:else}
								{formatEventInfo(data.wife.death) || 'Unknown'}
							{/if}
							{#if hasCitations(data.wife.death)}
								<span class="citation-indicator" title="Has citations">[{data.wife.death?.citations?.length}]</span>
							{/if}
						</span>
					</div>
				</div>

				{#if data.wife.father_name || data.wife.mother_name}
					<div class="parents-info">
						<div class="parent-row">
							<span class="label">Father:</span>
							{#if data.wife.father_id}
								<a href="/persons/{data.wife.father_id}">{data.wife.father_name}</a>
							{:else}
								<span>{data.wife.father_name || 'Unknown'}</span>
							{/if}
						</div>
						<div class="parent-row">
							<span class="label">Mother:</span>
							{#if data.wife.mother_id}
								<a href="/persons/{data.wife.mother_id}">{data.wife.mother_name}</a>
							{:else}
								<span>{data.wife.mother_name || 'Unknown'}</span>
							{/if}
						</div>
					</div>
				{/if}
			</div>
		{:else}
			<p class="no-data">No wife/mother recorded</p>
		{/if}
	</section>

	<!-- Marriage Section -->
	<section class="marriage-section" class:marriage-negated={isNegated(data.marriage)}>
		<h2 class="section-title">Marriage</h2>
		{#if data.marriage}
			{#if isNegated(data.marriage)}
				<p class="negated-event">No marriage recorded</p>
				{#if hasCitations(data.marriage)}
					<div class="citations-row">
						<span class="citation-indicator">[{data.marriage.citations?.length} citation(s)]</span>
					</div>
				{/if}
			{:else}
				<div class="marriage-info">
					<div class="event-row">
						<span class="event-label">Date:</span>
						<span class="event-value">{data.marriage.date || 'Unknown'}</span>
					</div>
					<div class="event-row">
						<span class="event-label">Place:</span>
						<span class="event-value">{data.marriage.place || 'Unknown'}</span>
					</div>
					{#if hasCitations(data.marriage)}
						<div class="citations-row">
							<span class="citation-indicator">[{data.marriage.citations?.length} citation(s)]</span>
						</div>
					{/if}
				</div>
			{/if}
		{:else}
			<p class="no-data">No marriage information recorded</p>
		{/if}
	</section>

	<!-- Children Section -->
	<section class="children-section">
		<h2 class="section-title">Children</h2>
		{#if data.children && data.children.length > 0}
			<table class="children-table">
				<thead>
					<tr>
						<th class="col-num">#</th>
						<th class="col-name">Name</th>
						<th class="col-gender">Sex</th>
						<th class="col-birth">Birth</th>
						<th class="col-death">Death</th>
						<th class="col-spouse">Spouse</th>
					</tr>
				</thead>
				<tbody>
					{#each data.children as child, index}
						<tr>
							<td class="col-num">{child.sequence ?? index + 1}</td>
							<td class="col-name">
								<a href="/persons/{child.id}">{formatPersonName(child)}</a>
								{#if child.relationship_type && child.relationship_type !== 'biological'}
									<span class="relationship-type">({child.relationship_type})</span>
								{/if}
							</td>
							<td class="col-gender">{child.gender === 'male' ? 'M' : child.gender === 'female' ? 'F' : '-'}</td>
							<td class="col-birth">
								{#if isNegated(child.birth)}
									<span class="negated-event">No birth recorded</span>
								{:else}
									{formatEventInfo(child.birth) || '-'}
								{/if}
								{#if hasCitations(child.birth)}
									<span class="citation-indicator">[{child.birth?.citations?.length}]</span>
								{/if}
							</td>
							<td class="col-death">
								{#if isNegated(child.death)}
									<span class="negated-event">No death recorded</span>
								{:else}
									{formatEventInfo(child.death) || '-'}
								{/if}
								{#if hasCitations(child.death)}
									<span class="citation-indicator">[{child.death?.citations?.length}]</span>
								{/if}
							</td>
							<td class="col-spouse">
								{#if child.spouse_id}
									<a href="/persons/{child.spouse_id}">{child.spouse_name}</a>
								{:else if child.spouse_name}
									{child.spouse_name}
								{:else}
									-
								{/if}
							</td>
						</tr>
					{/each}
				</tbody>
			</table>
		{:else}
			<p class="no-data">No children recorded</p>
		{/if}
	</section>

	<!-- Citations Section -->
	{#if (data.husband?.birth?.citations?.length || data.husband?.death?.citations?.length || data.wife?.birth?.citations?.length || data.wife?.death?.citations?.length || data.marriage?.citations?.length || data.children?.some(c => c.birth?.citations?.length || c.death?.citations?.length))}
		<section class="citations-section">
			<h2 class="section-title">Source Citations</h2>
			<ol class="citations-list">
				{#if data.husband?.birth?.citations}
					{#each data.husband.birth.citations as cit}
						<li>
							<a href="/sources/{cit.source_id}">{cit.source_title}</a>
							{#if cit.detail}
								<span class="cit-detail">({cit.detail})</span>
							{/if}
							<span class="cit-context">- {data.husband?.given_name} birth</span>
						</li>
					{/each}
				{/if}
				{#if data.husband?.death?.citations}
					{#each data.husband.death.citations as cit}
						<li>
							<a href="/sources/{cit.source_id}">{cit.source_title}</a>
							{#if cit.detail}
								<span class="cit-detail">({cit.detail})</span>
							{/if}
							<span class="cit-context">- {data.husband?.given_name} death</span>
						</li>
					{/each}
				{/if}
				{#if data.wife?.birth?.citations}
					{#each data.wife.birth.citations as cit}
						<li>
							<a href="/sources/{cit.source_id}">{cit.source_title}</a>
							{#if cit.detail}
								<span class="cit-detail">({cit.detail})</span>
							{/if}
							<span class="cit-context">- {data.wife?.given_name} birth</span>
						</li>
					{/each}
				{/if}
				{#if data.wife?.death?.citations}
					{#each data.wife.death.citations as cit}
						<li>
							<a href="/sources/{cit.source_id}">{cit.source_title}</a>
							{#if cit.detail}
								<span class="cit-detail">({cit.detail})</span>
							{/if}
							<span class="cit-context">- {data.wife?.given_name} death</span>
						</li>
					{/each}
				{/if}
				{#if data.marriage?.citations}
					{#each data.marriage.citations as cit}
						<li>
							<a href="/sources/{cit.source_id}">{cit.source_title}</a>
							{#if cit.detail}
								<span class="cit-detail">({cit.detail})</span>
							{/if}
							<span class="cit-context">- marriage</span>
						</li>
					{/each}
				{/if}
				{#if data.children}
					{#each data.children as child}
						{#if child.birth?.citations}
							{#each child.birth.citations as cit}
								<li>
									<a href="/sources/{cit.source_id}">{cit.source_title}</a>
									{#if cit.detail}
										<span class="cit-detail">({cit.detail})</span>
									{/if}
									<span class="cit-context">- {child.given_name} birth</span>
								</li>
							{/each}
						{/if}
						{#if child.death?.citations}
							{#each child.death.citations as cit}
								<li>
									<a href="/sources/{cit.source_id}">{cit.source_title}</a>
									{#if cit.detail}
										<span class="cit-detail">({cit.detail})</span>
									{/if}
									<span class="cit-context">- {child.given_name} death</span>
								</li>
							{/each}
						{/if}
					{/each}
				{/if}
			</ol>
		</section>
	{/if}
</div>

<style>
	.group-sheet {
		font-family: 'Georgia', 'Times New Roman', serif;
		max-width: 900px;
		margin: 0 auto;
		padding: 1.5rem;
		background: white;
	}

	.sheet-header {
		text-align: center;
		margin-bottom: 2rem;
		padding-bottom: 1rem;
		border-bottom: 2px solid #1e293b;
	}

	.sheet-header h1 {
		margin: 0;
		font-size: 1.75rem;
		font-weight: 600;
		color: #1e293b;
		text-transform: uppercase;
		letter-spacing: 0.1em;
	}

	.section-title {
		font-size: 1rem;
		font-weight: 600;
		color: #1e293b;
		text-transform: uppercase;
		letter-spacing: 0.05em;
		margin: 0 0 1rem;
		padding-bottom: 0.5rem;
		border-bottom: 1px solid #cbd5e1;
	}

	.person-section {
		margin-bottom: 2rem;
		padding: 1rem;
		background: #f8fafc;
		border: 1px solid #e2e8f0;
		border-radius: 8px;
	}

	.person-info {
		display: flex;
		flex-direction: column;
		gap: 0.75rem;
	}

	.info-row,
	.event-row,
	.parent-row {
		display: flex;
		gap: 0.5rem;
		align-items: baseline;
	}

	.name-row {
		font-size: 1.125rem;
	}

	.label,
	.event-label {
		font-weight: 600;
		color: #475569;
		min-width: 60px;
	}

	.person-link {
		color: #1e293b;
		text-decoration: none;
		font-weight: 500;
	}

	.person-link:hover {
		color: #3b82f6;
		text-decoration: underline;
	}

	.events-grid {
		display: grid;
		grid-template-columns: repeat(2, 1fr);
		gap: 0.5rem 2rem;
	}

	.event-value {
		color: #1e293b;
	}

	.parents-info {
		margin-top: 0.5rem;
		padding-top: 0.75rem;
		border-top: 1px dashed #cbd5e1;
	}

	.parent-row a {
		color: #1e293b;
		text-decoration: none;
	}

	.parent-row a:hover {
		color: #3b82f6;
	}

	.citation-indicator {
		color: #3b82f6;
		font-size: 0.75rem;
		font-weight: 500;
		margin-left: 0.25rem;
	}

	.marriage-section {
		margin-bottom: 2rem;
		padding: 1rem;
		background: #fef3c7;
		border: 1px solid #fcd34d;
		border-radius: 8px;
	}

	.marriage-info {
		display: flex;
		flex-direction: column;
		gap: 0.5rem;
	}

	.children-section {
		margin-bottom: 2rem;
	}

	.children-table {
		width: 100%;
		border-collapse: collapse;
		font-size: 0.875rem;
	}

	.children-table th,
	.children-table td {
		padding: 0.625rem 0.75rem;
		text-align: left;
		border: 1px solid #e2e8f0;
	}

	.children-table th {
		background: #f1f5f9;
		font-weight: 600;
		color: #475569;
		text-transform: uppercase;
		font-size: 0.75rem;
		letter-spacing: 0.05em;
	}

	.children-table tbody tr:nth-child(even) {
		background: #f8fafc;
	}

	.children-table a {
		color: #1e293b;
		text-decoration: none;
	}

	.children-table a:hover {
		color: #3b82f6;
	}

	.col-num {
		width: 40px;
		text-align: center;
	}

	.col-gender {
		width: 50px;
		text-align: center;
	}

	.relationship-type {
		color: #94a3b8;
		font-size: 0.75rem;
		font-style: italic;
		margin-left: 0.25rem;
	}

	.citations-section {
		margin-top: 2rem;
		padding-top: 1.5rem;
		border-top: 2px solid #1e293b;
	}

	.citations-list {
		margin: 0;
		padding-left: 1.5rem;
		font-size: 0.875rem;
	}

	.citations-list li {
		margin-bottom: 0.5rem;
	}

	.citations-list a {
		color: #3b82f6;
		text-decoration: none;
	}

	.citations-list a:hover {
		text-decoration: underline;
	}

	.cit-detail {
		color: #64748b;
		font-size: 0.8125rem;
	}

	.cit-context {
		color: #94a3b8;
		font-size: 0.8125rem;
		font-style: italic;
	}

	.no-data {
		color: #94a3b8;
		font-style: italic;
		margin: 0;
	}

	.negated-event {
		color: #94a3b8;
		font-style: italic;
		font-size: 0.8125rem;
	}

	.marriage-negated {
		background: #f8fafc;
		border-color: #e2e8f0;
	}

	/* Print styles */
	@media print {
		.group-sheet {
			font-size: 11pt;
			padding: 0;
			max-width: none;
		}

		.sheet-header h1 {
			font-size: 16pt;
		}

		.section-title {
			font-size: 12pt;
		}

		.person-section,
		.marriage-section {
			background: white !important;
			border: 1px solid #000;
			-webkit-print-color-adjust: exact;
			print-color-adjust: exact;
		}

		.children-table th {
			background: #eee !important;
			-webkit-print-color-adjust: exact;
			print-color-adjust: exact;
		}

		.children-table tbody tr:nth-child(even) {
			background: #f5f5f5 !important;
			-webkit-print-color-adjust: exact;
			print-color-adjust: exact;
		}

		a {
			color: #000 !important;
			text-decoration: none !important;
		}

		.citation-indicator {
			color: #333 !important;
		}

		.person-link,
		.children-table a,
		.citations-list a,
		.parent-row a {
			color: #000 !important;
		}
	}
</style>
