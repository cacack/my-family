<script lang="ts">
	import { goto } from '$app/navigation';
	import { api, type ImportResult } from '$lib/api/client';

	let file: File | null = $state(null);
	let importing = $state(false);
	let result: ImportResult | null = $state(null);
	let error: string | null = $state(null);
	let dragOver = $state(false);

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
		try {
			const gedcom = await api.exportGedcom();
			const blob = new Blob([gedcom], { type: 'text/plain' });
			const url = URL.createObjectURL(blob);
			const a = document.createElement('a');
			a.href = url;
			a.download = 'export.ged';
			a.click();
			URL.revokeObjectURL(url);
		} catch (e) {
			error = (e as { message?: string }).message || 'Export failed';
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
				<!-- svelte-ignore a11y_no_static_element_interactions -->
				<div
					class="drop-zone"
					class:drag-over={dragOver}
					ondrop={handleDrop}
					ondragover={handleDragOver}
					ondragleave={handleDragLeave}
				>
					{#if file}
						<div class="file-info">
							<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
								<path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z" />
								<polyline points="14 2 14 8 20 8" />
							</svg>
							<span class="file-name">{file.name}</span>
							<span class="file-size">({(file.size / 1024).toFixed(1)} KB)</span>
							<button class="remove-btn" onclick={reset}>&times;</button>
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
					<p class="error-message">{error}</p>
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
				Download all your family tree data as a GEDCOM 5.5 file that can be imported into other
				genealogy software.
			</p>
			<button class="btn" onclick={exportData}>
				<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
					<path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4" />
					<polyline points="7 10 12 15 17 10" />
					<line x1="12" y1="15" x2="12" y2="3" />
				</svg>
				Export GEDCOM
			</button>
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
</style>
