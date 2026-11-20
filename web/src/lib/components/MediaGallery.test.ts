import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent, within } from '@testing-library/svelte';
import MediaGallery from './MediaGallery.svelte';
import * as apiModule from '#lib/api/client.js';
import type { Media } from '#lib/api/client.js';

vi.mock('#lib/api/client.js', async (importOriginal) => {
	const actual = await importOriginal<typeof apiModule>();
	return {
		...actual,
		api: {
			listPersonMedia: vi.fn(),
			getMediaThumbnailUrl: (id: string) => `/api/v1/media/${id}/thumbnail`,
			getMediaContentUrl: (id: string) => `/api/v1/media/${id}/content`
		}
	};
});

function media(overrides: Partial<Media>): Media {
	return {
		id: 'm-1',
		entity_type: 'person',
		entity_id: 'p-1',
		title: 'Portrait',
		mime_type: 'image/jpeg',
		filename: 'portrait.jpg',
		file_size: 2048,
		has_thumbnail: true,
		version: 1,
		created_at: '2026-01-01T00:00:00Z',
		...overrides
	};
}

// A GEDCOM-imported item names its file but stores no content (#1477).
const reference = media({
	id: 'm-ref',
	title: 'Wedding',
	filename: '',
	file_size: 0,
	has_thumbnail: false,
	files: [{ path: 'photos/wedding.jpg', format: 'image/jpeg' }]
});

function list(items: Media[]) {
	vi.mocked(apiModule.api.listPersonMedia).mockResolvedValue({ items, total: items.length });
}

describe('MediaGallery', () => {
	beforeEach(() => {
		vi.clearAllMocks();
	});

	it('shows a reference-only item as a placeholder with its title and path, not an image', async () => {
		list([reference]);
		render(MediaGallery, { props: { personId: 'p-1' } });

		const card = await screen.findByRole('button', { name: /^View Wedding photos\/wedding\.jpg File not stored$/ });
		expect(within(card).getByText('Wedding')).toBeDefined();
		expect(within(card).getByText('photos/wedding.jpg')).toBeDefined();
		expect(within(card).getByText('File not stored')).toBeDefined();
		expect(card.querySelector('img')).toBeNull();
	});

	it('shows an uploaded item with a thumbnail as its image', async () => {
		list([media({})]);
		render(MediaGallery, { props: { personId: 'p-1' } });

		const card = await screen.findByRole('button', { name: 'View Portrait' });
		expect(card.querySelector('img')?.getAttribute('src')).toBe('/api/v1/media/m-1/thumbnail');
		expect(within(card).queryByText('File not stored')).toBeNull();
	});

	it('opens a reference-only item without loading or offering its content', async () => {
		list([reference]);
		render(MediaGallery, { props: { personId: 'p-1' } });

		await fireEvent.click(await screen.findByRole('button', { name: /^View Wedding photos\/wedding\.jpg File not stored$/ }));

		const dialog = await screen.findByRole('dialog');
		expect(within(dialog).getByText('photos/wedding.jpg')).toBeDefined();
		expect(within(dialog).getByText('Not stored (reference only)')).toBeDefined();
		expect(dialog.querySelector('img')).toBeNull();
		expect(dialog.querySelector('a[href*="/content"]')).toBeNull();
	});
});
