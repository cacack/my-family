import { describe, it, expect } from 'vitest';
import { render, screen } from '@testing-library/svelte';
import ImportProgress from './ImportProgress.svelte';
import type { ImportProgress as ImportProgressType } from '$lib/api/client';

describe('ImportProgress', () => {
	it('renders a determinate progress bar with percent and byte counts', () => {
		const progress: ImportProgressType = {
			bytes_read: 512 * 1024,
			total_bytes: 1024 * 1024,
			percent: 50
		};
		render(ImportProgress, { progress });

		const bar = screen.getByRole('progressbar');
		expect(bar.getAttribute('aria-valuenow')).toBe('50');
		expect(screen.getByText('50%')).toBeTruthy();
		// Human-readable byte sizes for read / total.
		expect(screen.getByText(/512\.0 KB/)).toBeTruthy();
		expect(screen.getByText(/1\.0 MB/)).toBeTruthy();
	});

	it('renders an indeterminate bar when the total size is unknown', () => {
		const progress: ImportProgressType = {
			bytes_read: 4096,
			total_bytes: -1,
			percent: -1
		};
		render(ImportProgress, { progress });

		const bar = screen.getByRole('progressbar');
		// No meaningful percentage is exposed when total is unknown.
		expect(bar.getAttribute('aria-valuenow')).toBeNull();
		expect(bar.getAttribute('aria-label')).toBe('Importing GEDCOM file');
		expect(screen.getByText(/4\.0 KB/)).toBeTruthy();
	});

	it('formats raw byte counts under 1KB', () => {
		const progress: ImportProgressType = {
			bytes_read: 512,
			total_bytes: 800,
			percent: 64
		};
		render(ImportProgress, { progress });
		expect(screen.getByText(/512 B/)).toBeTruthy();
	});
});
