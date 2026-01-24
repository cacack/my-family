<script lang="ts">
	import { goto } from '$app/navigation';
	import { api, type ImportResult } from '$lib/api/client';
	import { ExportButton } from '$lib/components/export';

	let file: File | null = $state(null);
	let importing = $state(false);
	let result: ImportResult | null = $state(null);
	let error: string | null = $state(null);
	let dragOver = $state(false);

	// Export state
	type EntityType = 'tree' | 'persons' | 'families' | 'sources' | 'events' | 'attributes';
	type ExportFormat = 'json' | 'csv';

	let exportEntityType: EntityType = $state('tree');
	let exportFormat: ExportFormat = $state('json');
	let exporting = $state(false);
	let exportError: string | null = $state(null);

	// Available fields for CSV export
	const personFields = [
		{ id: 'id', label: 'ID' },
		{ id: 'given_name', label: 'Given Name' },
		{ id: 'surname', label: 'Surname' },
		{ id: 'full_name', label: 'Full Name' },
		{ id: 'gender', label: 'Gender' },
		{ id: 'birth_date', label: 'Birth Date' },
		{ id: 'birth_place', label: 'Birth Place' },
		{ id: 'death_date', label: 'Death Date' },
		{ id: 'death_place', label: 'Death Place' },
		{ id: 'notes', label: 'Notes' }
	];

	const familyFields = [
		{ id: 'id', label: 'ID' },
		{ id: 'partner1_name', label: 'Partner 1 Name' },
		{ id: 'partner2_name', label: 'Partner 2 Name' },
		{ id: 'relationship_type', label: 'Relationship Type' },
		{ id: 'marriage_date', label: 'Marriage Date' },
		{ id: 'marriage_place', label: 'Marriage Place' },
		{ id: 'child_count', label: 'Child Count' }
	];

	const sourceFields = [
		{ id: 'id', label: 'ID' },
		{ id: 'source_type', label: 'Type' },
		{ id: 'title', label: 'Title' },
		{ id: 'author', label: 'Author' },
		{ id: 'publisher', label: 'Publisher' },
		{ id: 'publish_date', label: 'Publish Date' },
		{ id: 'url', label: 'URL' },
		{ id: 'repository_name', label: 'Repository' },
		{ id: 'collection_name', label: 'Collection' },
		{ id: 'call_number', label: 'Call Number' },
		{ id: 'citation_count', label: 'Citation Count' },
		{ id: 'notes', label: 'Notes' }
	];

	const eventFields = [
		{ id: 'id', label: 'ID' },
		{ id: 'owner_type', label: 'Owner Type' },
		{ id: 'owner_id', label: 'Owner ID' },
		{ id: 'fact_type', label: 'Event Type' },
		{ id: 'date', label: 'Date' },
		{ id: 'place', label: 'Place' },
		{ id: 'description', label: 'Description' },
		{ id: 'cause', label: 'Cause' },
		{ id: 'age', label: 'Age' },
		{ id: 'research_status', label: 'Research Status' }
	];

	const attributeFields = [
		{ id: 'id', label: 'ID' },
		{ id: 'person_id', label: 'Person ID' },
		{ id: 'fact_type', label: 'Attribute Type' },
		{ id: 'value', label: 'Value' },
		{ id: 'date', label: 'Date' },
		{ id: 'place', label: 'Place' }
	];

	// Selected fields (default all selected)
	let selectedPersonFields: Set<string> = $state(new Set(personFields.map((f) => f.id)));
	let selectedFamilyFields: Set<string> = $state(new Set(familyFields.map((f) => f.id)));
	let selectedSourceFields: Set<string> = $state(new Set(sourceFields.map((f) => f.id)));
	let selectedEventFields: Set<string> = $state(new Set(eventFields.map((f) => f.id)));
	let selectedAttributeFields: Set<string> = $state(new Set(attributeFields.map((f) => f.id)));

	function toggleField(fieldId: string, entityType: EntityType) {
		if (entityType === 'persons') {
			const newSet = new Set(selectedPersonFields);
			if (newSet.has(fieldId)) newSet.delete(fieldId);
			else newSet.add(fieldId);
			selectedPersonFields = newSet;
		} else if (entityType === 'families') {
			const newSet = new Set(selectedFamilyFields);
			if (newSet.has(fieldId)) newSet.delete(fieldId);
			else newSet.add(fieldId);
			selectedFamilyFields = newSet;
		} else if (entityType === 'sources') {
			const newSet = new Set(selectedSourceFields);
			if (newSet.has(fieldId)) newSet.delete(fieldId);
			else newSet.add(fieldId);
			selectedSourceFields = newSet;
		} else if (entityType === 'events') {
			const newSet = new Set(selectedEventFields);
			if (newSet.has(fieldId)) newSet.delete(fieldId);
			else newSet.add(fieldId);
			selectedEventFields = newSet;
		} else if (entityType === 'attributes') {
			const newSet = new Set(selectedAttributeFields);
			if (newSet.has(fieldId)) newSet.delete(fieldId);
			else newSet.add(fieldId);
			selectedAttributeFields = newSet;
		}
	}

	function selectAllFields(entityType: EntityType) {
		if (entityType === 'persons') {
			selectedPersonFields = new Set(personFields.map((f) => f.id));
		} else if (entityType === 'families') {
			selectedFamilyFields = new Set(familyFields.map((f) => f.id));
		} else if (entityType === 'sources') {
			selectedSourceFields = new Set(sourceFields.map((f) => f.id));
		} else if (entityType === 'events') {
			selectedEventFields = new Set(eventFields.map((f) => f.id));
		} else if (entityType === 'attributes') {
			selectedAttributeFields = new Set(attributeFields.map((f) => f.id));
		}
	}

	function selectNoFields(entityType: EntityType) {
		if (entityType === 'persons') {
			selectedPersonFields = new Set();
		} else if (entityType === 'families') {
			selectedFamilyFields = new Set();
		} else if (entityType === 'sources') {
			selectedSourceFields = new Set();
		} else if (entityType === 'events') {
			selectedEventFields = new Set();
		} else if (entityType === 'attributes') {
			selectedAttributeFields = new Set();
		}
	}

	function getFieldsForEntityType(entityType: EntityType) {
		switch (entityType) {
			case 'persons': return personFields;
			case 'families': return familyFields;
			case 'sources': return sourceFields;
			case 'events': return eventFields;
			case 'attributes': return attributeFields;
			default: return [];
		}
	}

	function getSelectedFieldsForEntityType(entityType: EntityType): Set<string> {
		switch (entityType) {
			case 'persons': return selectedPersonFields;
			case 'families': return selectedFamilyFields;
			case 'sources': return selectedSourceFields;
			case 'events': return selectedEventFields;
			case 'attributes': return selectedAttributeFields;
			default: return new Set();
		}
	}

	function getEntityLabel(entityType: EntityType): string {
		switch (entityType) {
			case 'tree': return 'Tree';
			case 'persons': return 'People';
			case 'families': return 'Families';
			case 'sources': return 'Sources';
			case 'events': return 'Events';
			case 'attributes': return 'Attributes';
			default: return '';
		}
	}

	function handleFileSelect(e: Event) {
		const input = e.target as HTMLInputElement;
		if (input.files && input.files.length > 0) {
			file = input.files[0];
			result = null;
			error = null;
		}
	}

	function handleDrop(e: DragEvent) {
		e.preventDefault();
		dragOver = false;
		if (e.dataTransfer?.files && e.dataTransfer.files.length > 0) {
			const droppedFile = e.dataTransfer.files[0];
			if (droppedFile.name.toLowerCase().endsWith('.ged')) {
				file = droppedFile;
				result = null;
				error = null;
			} else {
				error = 'Please select a GEDCOM file (.ged)';
			}
		}
	}

	function handleDragOver(e: DragEvent) {
		e.preventDefault();
		dragOver = true;
	}

	function handleDragLeave() {
		dragOver = false;
	}

	async function importFile() {
		if (!file) return;
		importing = true;
		error = null;
		result = null;

		try {
			result = await api.importGedcom(file);
		} catch (e) {
			error = (e as { message?: string }).message || 'Import failed';
		} finally {
			importing = false;
		}
	}

	function reset() {
		file = null;
		result = null;
		error = null;
	}

	async function exportData() {
		exporting = true;
		exportError = null;

		try {
			let data: string;
			let filename: string;
			let contentType: string;

			if (exportEntityType === 'tree') {
				data = await api.exportTree();
				filename = 'family-tree.json';
				contentType = 'application/json';
			} else if (exportEntityType === 'persons') {
				const fields =
					exportFormat === 'csv' ? Array.from(selectedPersonFields) : undefined;
				data = await api.exportPersons(exportFormat, fields);
				filename = `persons.${exportFormat}`;
				contentType = exportFormat === 'csv' ? 'text/csv' : 'application/json';
			} else if (exportEntityType === 'families') {
				const fields =
					exportFormat === 'csv' ? Array.from(selectedFamilyFields) : undefined;
				data = await api.exportFamilies(exportFormat, fields);
				filename = `families.${exportFormat}`;
				contentType = exportFormat === 'csv' ? 'text/csv' : 'application/json';
			} else if (exportEntityType === 'sources') {
				const fields =
					exportFormat === 'csv' ? Array.from(selectedSourceFields) : undefined;
				data = await api.exportSources(exportFormat, fields);
				filename = `sources.${exportFormat}`;
				contentType = exportFormat === 'csv' ? 'text/csv' : 'application/json';
			} else if (exportEntityType === 'events') {
				const fields =
					exportFormat === 'csv' ? Array.from(selectedEventFields) : undefined;
				data = await api.exportEvents(exportFormat, fields);
				filename = `events.${exportFormat}`;
				contentType = exportFormat === 'csv' ? 'text/csv' : 'application/json';
			} else if (exportEntityType === 'attributes') {
				const fields =
					exportFormat === 'csv' ? Array.from(selectedAttributeFields) : undefined;
				data = await api.exportAttributes(exportFormat, fields);
				filename = `attributes.${exportFormat}`;
				contentType = exportFormat === 'csv' ? 'text/csv' : 'application/json';
			} else {
				throw new Error(`Unknown entity type: ${exportEntityType}`);
			}

			const blob = new Blob([data], { type: contentType });
			const url = URL.createObjectURL(blob);
			const a = document.createElement('a');
			a.href = url;
			a.download = filename;
			a.click();
			URL.revokeObjectURL(url);
		} catch (e) {
			exportError = (e as { message?: string }).message || 'Export failed';
		} finally {
			exporting = false;
		}
	}
</script>

<svelte:head>
	<title>Import GEDCOM | My Family</title>
</svelte:head>

<div class="import-page">
	<header class="page-header">
		<h1>Import & Export</h1>
	</header>

	<div class="content">
		<section class="import-section">
			<h2>Import GEDCOM File</h2>
			<p class="description">
				Import your family tree data from a GEDCOM 5.5 file. This will add all individuals and
				families to your database.
			</p>

			{#if result}
				<div class="result" class:success={result.success}>
					<h3>{result.success ? 'Import Successful!' : 'Import Completed with Issues'}</h3>
					<div class="stats">
						<div class="stat">
							<span class="value">{result.persons_imported}</span>
							<span class="label">People imported</span>
						</div>
						<div class="stat">
							<span class="value">{result.families_imported}</span>
							<span class="label">Families imported</span>
						</div>
					</div>

					{#if result.warnings && result.warnings.length > 0}
						<div class="warnings">
							<h4>Warnings ({result.warnings.length})</h4>
							<ul>
								{#each result.warnings.slice(0, 10) as warning}
									<li>
										Line {warning.line}: {warning.message}
										{#if warning.record} ({warning.record}){/if}
									</li>
								{/each}
								{#if result.warnings.length > 10}
									<li class="more">...and {result.warnings.length - 10} more warnings</li>
								{/if}
							</ul>
						</div>
					{/if}

					{#if result.errors && result.errors.length > 0}
						<div class="errors">
							<h4>Errors ({result.errors.length})</h4>
							<ul>
								{#each result.errors.slice(0, 10) as err}
									<li>
										Line {err.line}: {err.message}
										{#if err.record} ({err.record}){/if}
									</li>
								{/each}
								{#if result.errors.length > 10}
									<li class="more">...and {result.errors.length - 10} more errors</li>
								{/if}
							</ul>
						</div>
					{/if}

					<div class="result-actions">
						<button class="btn" onclick={reset}>Import Another</button>
						<a href="/persons" class="btn btn-primary">View People</a>
					</div>
				</div>
			{:else}
					<div
					class="drop-zone"
					class:drag-over={dragOver}
					ondrop={handleDrop}
					ondragover={handleDragOver}
					ondragleave={handleDragLeave}
					role="region"
					aria-label={file ? `Selected file: ${file.name}` : 'GEDCOM file drop zone - drag and drop files here or use the browse button'}
				>
					{#if file}
						<div class="file-info">
							<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
								<path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z" />
								<polyline points="14 2 14 8 20 8" />
							</svg>
							<span class="file-name">{file.name}</span>
							<span class="file-size">({(file.size / 1024).toFixed(1)} KB)</span>
							<button class="remove-btn" onclick={reset} aria-label="Remove selected file">&times;</button>
						</div>
					{:else}
						<svg class="upload-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
							<path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4" />
							<polyline points="17 8 12 3 7 8" />
							<line x1="12" y1="3" x2="12" y2="15" />
						</svg>
						<p>Drag and drop a GEDCOM file here, or</p>
						<label class="file-label">
							<input type="file" accept=".ged,.gedcom" onchange={handleFileSelect} hidden />
							Browse Files
						</label>
					{/if}
				</div>

				{#if error}
					<p class="error-message" role="alert">{error}</p>
				{/if}

				{#if file}
					<button class="btn btn-primary btn-large" onclick={importFile} disabled={importing}>
						{#if importing}
							<span class="spinner"></span>
							Importing...
						{:else}
							Import File
						{/if}
					</button>
				{/if}
			{/if}
		</section>

		<section class="export-section">
			<h2>Export Data</h2>
			<p class="description">
				Download your family tree data in various formats. GEDCOM files can be imported into other
				genealogy software, while JSON and CSV formats are useful for data analysis.
			</p>

			<!-- GEDCOM Export with Progress Tracking -->
			<div class="export-option">
				<h3>GEDCOM Format</h3>
				<p class="option-description">Standard genealogy format compatible with most software.</p>
				<ExportButton label="Export GEDCOM" showEstimate={true} />
			</div>

			<hr class="divider" />

			<!-- Custom Export -->
			<div class="export-option">
				<h3>Custom Export</h3>
				<p class="option-description">Export specific data types in JSON or CSV format.</p>

				<!-- Entity Type Selector -->
				<div class="form-group">
					<span class="form-label" id="export-entity-type-label">What to export</span>
					<div class="radio-group radio-group-wrap" role="radiogroup" aria-labelledby="export-entity-type-label">
						<label class="radio-label">
							<input
								type="radio"
								name="entityType"
								value="tree"
								bind:group={exportEntityType}
							/>
							<span>Complete Tree</span>
						</label>
						<label class="radio-label">
							<input
								type="radio"
								name="entityType"
								value="persons"
								bind:group={exportEntityType}
							/>
							<span>People</span>
						</label>
						<label class="radio-label">
							<input
								type="radio"
								name="entityType"
								value="families"
								bind:group={exportEntityType}
							/>
							<span>Families</span>
						</label>
						<label class="radio-label">
							<input
								type="radio"
								name="entityType"
								value="sources"
								bind:group={exportEntityType}
							/>
							<span>Sources</span>
						</label>
						<label class="radio-label">
							<input
								type="radio"
								name="entityType"
								value="events"
								bind:group={exportEntityType}
							/>
							<span>Events</span>
						</label>
						<label class="radio-label">
							<input
								type="radio"
								name="entityType"
								value="attributes"
								bind:group={exportEntityType}
							/>
							<span>Attributes</span>
						</label>
					</div>
				</div>

				<!-- Format Selector (only for persons/families) -->
				{#if exportEntityType !== 'tree'}
					<div class="form-group">
						<span class="form-label" id="export-format-label">Format</span>
						<div class="radio-group" role="radiogroup" aria-labelledby="export-format-label">
							<label class="radio-label">
								<input
									type="radio"
									name="format"
									value="json"
									bind:group={exportFormat}
								/>
								<span>JSON</span>
							</label>
							<label class="radio-label">
								<input
									type="radio"
									name="format"
									value="csv"
									bind:group={exportFormat}
								/>
								<span>CSV</span>
							</label>
						</div>
					</div>

					<!-- Field Picker (only for CSV) -->
					{#if exportFormat === 'csv'}
						<div class="form-group">
							<div class="field-picker-header">
								<span class="form-label" id="export-fields-label">Fields to include</span>
								<div class="field-picker-actions">
									<button
										type="button"
										class="btn-text"
										onclick={() => selectAllFields(exportEntityType)}
									>
										Select All
									</button>
									<button
										type="button"
										class="btn-text"
										onclick={() => selectNoFields(exportEntityType)}
									>
										Select None
									</button>
								</div>
							</div>
							<div class="field-picker" role="group" aria-labelledby="export-fields-label">
								{#each getFieldsForEntityType(exportEntityType) as field}
									<label class="checkbox-label">
										<input
											type="checkbox"
											checked={getSelectedFieldsForEntityType(exportEntityType).has(field.id)}
											onchange={() => toggleField(field.id, exportEntityType)}
										/>
										<span>{field.label}</span>
									</label>
								{/each}
							</div>
						</div>
					{/if}
				{/if}

				{#if exportError}
					<p class="error-message" role="alert">{exportError}</p>
				{/if}

				<button class="btn btn-primary" onclick={exportData} disabled={exporting}>
					{#if exporting}
						<span class="spinner"></span>
						Exporting...
					{:else}
						<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
							<path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4" />
							<polyline points="7 10 12 15 17 10" />
							<line x1="12" y1="15" x2="12" y2="3" />
						</svg>
						Export {getEntityLabel(exportEntityType)} as {exportEntityType === 'tree' ? 'JSON' : exportFormat.toUpperCase()}
					{/if}
				</button>
			</div>
		</section>
	</div>
</div>

<style>
	.import-page {
		max-width: 800px;
		margin: 0 auto;
		padding: 1.5rem;
	}

	.page-header h1 {
		margin: 0;
		font-size: 1.5rem;
		color: #1e293b;
	}

	.content {
		display: flex;
		flex-direction: column;
		gap: 2rem;
		margin-top: 1.5rem;
	}

	section {
		background: white;
		border-radius: 12px;
		border: 1px solid #e2e8f0;
		padding: 1.5rem;
	}

	h2 {
		margin: 0 0 0.5rem;
		font-size: 1.125rem;
		color: #1e293b;
	}

	.description {
		margin: 0 0 1.5rem;
		color: #64748b;
		font-size: 0.875rem;
	}

	.drop-zone {
		display: flex;
		flex-direction: column;
		align-items: center;
		justify-content: center;
		padding: 2.5rem;
		border: 2px dashed #cbd5e1;
		border-radius: 8px;
		background: #f8fafc;
		transition: all 0.15s;
	}

	.drop-zone.drag-over {
		border-color: #3b82f6;
		background: #eff6ff;
	}

	.upload-icon {
		width: 3rem;
		height: 3rem;
		color: #94a3b8;
		margin-bottom: 1rem;
	}

	.drop-zone p {
		margin: 0 0 0.75rem;
		color: #64748b;
		font-size: 0.875rem;
	}

	.file-label {
		display: inline-block;
		padding: 0.625rem 1.25rem;
		background: white;
		border: 1px solid #cbd5e1;
		border-radius: 6px;
		font-size: 0.875rem;
		font-weight: 500;
		color: #475569;
		cursor: pointer;
		transition: all 0.15s;
	}

	.file-label:hover {
		background: #f1f5f9;
		border-color: #94a3b8;
	}

	.file-info {
		display: flex;
		align-items: center;
		gap: 0.5rem;
	}

	.file-info svg {
		width: 1.5rem;
		height: 1.5rem;
		color: #3b82f6;
	}

	.file-name {
		font-weight: 500;
		color: #1e293b;
	}

	.file-size {
		color: #64748b;
		font-size: 0.875rem;
	}

	.remove-btn {
		margin-left: 0.5rem;
		width: 1.5rem;
		height: 1.5rem;
		border: none;
		background: #f1f5f9;
		border-radius: 50%;
		font-size: 1rem;
		color: #64748b;
		cursor: pointer;
		line-height: 1;
	}

	.remove-btn:hover {
		background: #e2e8f0;
		color: #1e293b;
	}

	.error-message {
		color: #dc2626;
		font-size: 0.875rem;
		margin: 1rem 0 0;
		text-align: center;
	}

	.btn {
		display: inline-flex;
		align-items: center;
		gap: 0.5rem;
		padding: 0.625rem 1.25rem;
		border: 1px solid #cbd5e1;
		border-radius: 6px;
		background: white;
		font-size: 0.875rem;
		font-weight: 500;
		color: #475569;
		cursor: pointer;
		text-decoration: none;
		transition: all 0.15s;
	}

	.btn:hover {
		background: #f1f5f9;
	}

	.btn svg {
		width: 1.125rem;
		height: 1.125rem;
	}

	.btn-primary {
		background: #3b82f6;
		border-color: #3b82f6;
		color: white;
	}

	.btn-primary:hover {
		background: #2563eb;
	}

	.btn-large {
		width: 100%;
		justify-content: center;
		padding: 0.875rem;
		margin-top: 1rem;
		font-size: 1rem;
	}

	.btn:disabled {
		opacity: 0.7;
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

	.result {
		padding: 1.5rem;
		background: #f0fdf4;
		border: 1px solid #86efac;
		border-radius: 8px;
	}

	.result h3 {
		margin: 0 0 1rem;
		color: #166534;
		font-size: 1.125rem;
	}

	.stats {
		display: flex;
		gap: 2rem;
		margin-bottom: 1rem;
	}

	.stat {
		display: flex;
		flex-direction: column;
	}

	.stat .value {
		font-size: 1.5rem;
		font-weight: 700;
		color: #166534;
	}

	.stat .label {
		font-size: 0.8125rem;
		color: #15803d;
	}

	.warnings,
	.errors {
		margin-top: 1rem;
		padding: 1rem;
		background: white;
		border-radius: 6px;
	}

	.warnings h4,
	.errors h4 {
		margin: 0 0 0.5rem;
		font-size: 0.875rem;
	}

	.warnings h4 {
		color: #a16207;
	}

	.errors h4 {
		color: #dc2626;
	}

	.warnings ul,
	.errors ul {
		margin: 0;
		padding: 0 0 0 1.25rem;
		font-size: 0.8125rem;
	}

	.warnings li {
		color: #854d0e;
	}

	.errors li {
		color: #991b1b;
	}

	.more {
		font-style: italic;
		color: #64748b;
	}

	.result-actions {
		display: flex;
		gap: 0.75rem;
		margin-top: 1.5rem;
	}

	.export-section {
		background: #fafafa;
	}

	.export-option {
		margin-bottom: 1.5rem;
	}

	.export-option:last-child {
		margin-bottom: 0;
	}

	.export-option h3 {
		margin: 0 0 0.25rem;
		font-size: 0.9375rem;
		font-weight: 600;
		color: #1e293b;
	}

	.option-description {
		margin: 0 0 0.75rem;
		color: #64748b;
		font-size: 0.8125rem;
	}

	.divider {
		border: none;
		border-top: 1px solid #e2e8f0;
		margin: 1.5rem 0;
	}

	.form-group {
		margin-bottom: 1rem;
	}

	.form-label {
		display: block;
		margin-bottom: 0.5rem;
		font-size: 0.875rem;
		font-weight: 500;
		color: #374151;
	}

	.radio-group {
		display: flex;
		flex-wrap: wrap;
		gap: 1rem;
	}

	.radio-label {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		cursor: pointer;
		font-size: 0.875rem;
		color: #475569;
	}

	.radio-label input[type='radio'] {
		width: 1rem;
		height: 1rem;
		accent-color: #3b82f6;
	}

	.field-picker-header {
		display: flex;
		justify-content: space-between;
		align-items: center;
		margin-bottom: 0.5rem;
	}

	.field-picker-header .form-label {
		margin-bottom: 0;
	}

	.field-picker-actions {
		display: flex;
		gap: 0.75rem;
	}

	.btn-text {
		background: none;
		border: none;
		padding: 0;
		font-size: 0.8125rem;
		font-weight: 500;
		color: #3b82f6;
		cursor: pointer;
	}

	.btn-text:hover {
		color: #2563eb;
		text-decoration: underline;
	}

	.field-picker {
		display: grid;
		grid-template-columns: repeat(auto-fill, minmax(140px, 1fr));
		gap: 0.5rem;
		padding: 1rem;
		background: white;
		border: 1px solid #e2e8f0;
		border-radius: 6px;
	}

	.checkbox-label {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		cursor: pointer;
		font-size: 0.8125rem;
		color: #475569;
	}

	.checkbox-label input[type='checkbox'] {
		width: 0.875rem;
		height: 0.875rem;
		accent-color: #3b82f6;
	}
</style>
