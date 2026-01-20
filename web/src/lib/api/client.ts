/**
 * My Family Genealogy API Client
 * Types are generated from OpenAPI spec - see types.generated.ts
 * Run `npm run generate:types` after OpenAPI changes
 */

import type { components } from './types.generated';

// Re-export Ahnentafel types from generated file (single source of truth)
export type AhnentafelResponse = components['schemas']['AhnentafelResponse'];
export type AhnentafelEntry = components['schemas']['AhnentafelEntry'];
export type AhnentafelSubject = components['schemas']['AhnentafelSubject'];

const API_BASE = '/api/v1';

// Types based on OpenAPI schemas
export type ResearchStatus = 'certain' | 'probable' | 'possible' | 'unknown';

export interface GenDate {
	raw?: string;
	qualifier?: 'exact' | 'abt' | 'cal' | 'est' | 'bef' | 'aft' | 'bet' | 'from';
	year?: number;
	month?: number;
	day?: number;
	year2?: number;
	month2?: number;
	day2?: number;
}

export interface Person {
	id: string;
	given_name: string;
	surname: string;
	gender?: 'male' | 'female' | 'unknown';
	birth_date?: GenDate;
	birth_place?: string;
	death_date?: GenDate;
	death_place?: string;
	notes?: string;
	research_status?: ResearchStatus;
	version: number;
}

export interface PersonCreate {
	given_name: string;
	surname: string;
	gender?: 'male' | 'female' | 'unknown';
	birth_date?: string;
	birth_place?: string;
	death_date?: string;
	death_place?: string;
	notes?: string;
	research_status?: ResearchStatus;
}

export interface PersonUpdate {
	given_name?: string;
	surname?: string;
	gender?: 'male' | 'female' | 'unknown';
	birth_date?: string;
	birth_place?: string;
	death_date?: string;
	death_place?: string;
	notes?: string;
	research_status?: ResearchStatus;
	version: number;
}

export interface PersonSummary {
	id: string;
	given_name: string;
	surname: string;
	gender?: 'male' | 'female' | 'unknown';
	birth_date?: GenDate;
	death_date?: GenDate;
}

export interface FamilySummary {
	id: string;
	partner1_name?: string;
	partner2_name?: string;
	relationship_type?: string;
}

export interface PersonDetail extends Person {
	families_as_partner?: FamilySummary[];
	family_as_child?: FamilySummary;
}

export interface PersonList {
	items: Person[];
	total: number;
	limit?: number;
	offset?: number;
}

export interface Family {
	id: string;
	partner1_id?: string;
	partner2_id?: string;
	partner1_name?: string;
	partner2_name?: string;
	relationship_type?: 'marriage' | 'partnership' | 'unknown';
	marriage_date?: GenDate;
	marriage_place?: string;
	child_count?: number;
	version: number;
}

export interface FamilyCreate {
	partner1_id?: string;
	partner2_id?: string;
	relationship_type?: 'marriage' | 'partnership' | 'unknown';
	marriage_date?: string;
	marriage_place?: string;
}

export interface FamilyUpdate {
	partner1_id?: string;
	partner2_id?: string;
	relationship_type?: 'marriage' | 'partnership' | 'unknown';
	marriage_date?: string;
	marriage_place?: string;
	version: number;
}

export interface FamilyChild {
	person_id: string;
	relationship_type: 'biological' | 'adopted' | 'foster';
	person?: PersonSummary;
	sequence?: number;
}

export interface FamilyDetail extends Family {
	partner1?: PersonSummary;
	partner2?: PersonSummary;
	children?: FamilyChild[];
}

export interface FamilyList {
	items: FamilyDetail[];
	total: number;
	limit?: number;
	offset?: number;
}

export interface AddChild {
	person_id: string;
	relationship_type?: 'biological' | 'adopted' | 'foster';
	sequence?: number;
}

// Group Sheet types for family group sheet view
export interface GroupSheetCitation {
	id: string;
	source_id: string;
	source_title: string;
	page?: string;
	detail?: string;
}

export interface GroupSheetEvent {
	date?: string;
	place?: string;
	citations?: GroupSheetCitation[];
}

export interface GroupSheetPerson {
	id: string;
	given_name: string;
	surname: string;
	gender?: 'male' | 'female' | 'unknown';
	birth?: GroupSheetEvent;
	death?: GroupSheetEvent;
	father_name?: string;
	father_id?: string;
	mother_name?: string;
	mother_id?: string;
}

export interface GroupSheetChild {
	id: string;
	given_name: string;
	surname: string;
	gender?: 'male' | 'female' | 'unknown';
	relationship_type?: 'biological' | 'adopted' | 'foster';
	sequence?: number;
	birth?: GroupSheetEvent;
	death?: GroupSheetEvent;
	spouse_name?: string;
	spouse_id?: string;
}

export interface FamilyGroupSheet {
	id: string;
	husband?: GroupSheetPerson;
	wife?: GroupSheetPerson;
	marriage?: GroupSheetEvent;
	children?: GroupSheetChild[];
}

export interface PedigreeNode {
	id: string;
	given_name?: string;
	surname?: string;
	birth_date?: GenDate;
	death_date?: GenDate;
	gender?: string;
	father?: PedigreeNode;
	mother?: PedigreeNode;
}

export interface Pedigree {
	root: PedigreeNode;
	generations?: number;
}

// Descendancy chart types
export interface SpouseInfo {
	id: string;
	given_name?: string;
	surname?: string;
	birth_date?: GenDate;
	death_date?: GenDate;
	gender?: string;
	marriage_date?: GenDate;
	marriage_place?: string;
}

export interface DescendancyNode {
	id: string;
	given_name?: string;
	surname?: string;
	birth_date?: GenDate;
	death_date?: GenDate;
	gender?: string;
	spouses?: SpouseInfo[];
	children?: DescendancyNode[];
}

export interface Descendancy {
	root: DescendancyNode;
	generations: number;
	total_descendants: number;
	max_generation: number;
}

// AhnentafelEntry and AhnentafelResponse are imported from types.generated.ts above

export interface SearchResult extends PersonSummary {
	score?: number;
}

export interface SearchResults {
	items: SearchResult[];
	total: number;
	query?: string;
}

export interface ImportWarning {
	line: number;
	record?: string;
	message: string;
}

export interface ImportError {
	line: number;
	record?: string;
	message: string;
}

export interface ImportResult {
	success: boolean;
	persons_imported: number;
	families_imported: number;
	warnings?: ImportWarning[];
	errors?: ImportError[];
}

export interface ApiError {
	code: string;
	message: string;
	details?: Record<string, unknown>;
}

// Source types
export interface Source {
	id: string;
	source_type: string;
	title: string;
	author?: string;
	publisher?: string;
	publish_date?: string;
	url?: string;
	repository_name?: string;
	collection_name?: string;
	call_number?: string;
	notes?: string;
	citation_count: number;
	version: number;
}

export interface SourceDetail extends Source {
	citations?: Citation[];
}

export interface SourceListResponse {
	sources: Source[];
	total: number;
	limit: number;
	offset: number;
}

export interface CreateSourceRequest {
	source_type: string;
	title: string;
	author?: string;
	publisher?: string;
	publish_date?: string;
	url?: string;
	repository_name?: string;
	collection_name?: string;
	call_number?: string;
	notes?: string;
}

export interface UpdateSourceRequest {
	source_type?: string;
	title?: string;
	author?: string;
	publisher?: string;
	publish_date?: string;
	url?: string;
	repository_name?: string;
	collection_name?: string;
	call_number?: string;
	notes?: string;
	version: number;
}

export interface SourceSearchResponse {
	sources: Source[];
	total: number;
	query: string;
}

// Citation types
export interface Citation {
	id: string;
	source_id: string;
	source_title: string;
	fact_type: string;
	fact_owner_id: string;
	page?: string;
	volume?: string;
	source_quality?: string;
	informant_type?: string;
	evidence_type?: string;
	quoted_text?: string;
	analysis?: string;
	template_id?: string;
	version: number;
}

export interface CitationListResponse {
	citations: Citation[];
	total: number;
}

export interface CreateCitationRequest {
	source_id: string;
	fact_type: string;
	fact_owner_id: string;
	page?: string;
	volume?: string;
	source_quality?: string;
	informant_type?: string;
	evidence_type?: string;
	quoted_text?: string;
	analysis?: string;
	template_id?: string;
}

export interface UpdateCitationRequest {
	page?: string;
	volume?: string;
	source_quality?: string;
	informant_type?: string;
	evidence_type?: string;
	quoted_text?: string;
	analysis?: string;
	template_id?: string;
	version: number;
}

// Media types
export interface Media {
	id: string;
	entity_type: string;
	entity_id: string;
	title: string;
	description?: string;
	mime_type: string;
	media_type?: string;
	filename: string;
	file_size: number;
	has_thumbnail: boolean;
	crop_left?: number;
	crop_top?: number;
	crop_width?: number;
	crop_height?: number;
	version: number;
	created_at: string;
	updated_at: string;
}

export interface MediaListResponse {
	items: Media[];
	total: number;
}

// Relationship types
export interface RelationshipPath {
	name?: string;
	pathFromA?: string[];
	pathFromB?: string[];
	commonAncestorId?: string;
	generationDistanceA?: number;
	generationDistanceB?: number;
}

export interface RelationshipResult {
	personA?: Person;
	personB?: Person;
	paths?: RelationshipPath[];
	isRelated?: boolean;
	summary?: string;
}

// Browse types
export interface SurnameIndexResponse {
	items: SurnameEntry[];
	total: number;
	letter_counts?: LetterCount[];
}

export interface SurnameEntry {
	surname: string;
	count: number;
}

export interface LetterCount {
	letter: string;
	count: number;
}

export interface PlaceIndexResponse {
	items: PlaceEntry[];
	total: number;
	breadcrumb?: string[];
}

export interface PlaceEntry {
	name: string;
	full_name: string;
	count: number;
	has_children: boolean;
}

export interface MediaUpdate {
	title?: string;
	description?: string;
	media_type?: string;
	crop_left?: number;
	crop_top?: number;
	crop_width?: number;
	crop_height?: number;
	version: number;
}

// History types
export interface FieldChange {
	old_value?: unknown;
	new_value?: unknown;
}

export interface ChangeEntry {
	id: string;
	timestamp: string;
	entity_type: string;
	entity_id: string;
	entity_name: string;
	action: 'created' | 'updated' | 'deleted';
	changes?: Record<string, FieldChange>;
	user_id?: string;
}

export interface ChangeHistoryResponse {
	items: ChangeEntry[];
	total: number;
	limit: number;
	offset: number;
	has_more: boolean;
}

class ApiClient {
	private async request<T>(
		method: string,
		path: string,
		body?: unknown,
		headers?: Record<string, string>
	): Promise<T> {
		const url = `${API_BASE}${path}`;
		const options: RequestInit = {
			method,
			headers: {
				'Content-Type': 'application/json',
				...headers
			}
		};

		if (body !== undefined) {
			options.body = JSON.stringify(body);
		}

		const response = await fetch(url, options);

		if (!response.ok) {
			const error: ApiError = await response.json().catch(() => ({
				code: 'UNKNOWN_ERROR',
				message: response.statusText
			}));
			throw error;
		}

		if (response.status === 204) {
			return undefined as T;
		}

		return response.json();
	}

	// Person endpoints
	async listPersons(params?: {
		limit?: number;
		offset?: number;
		sort?: 'surname' | 'given_name' | 'birth_date' | 'updated_at';
		order?: 'asc' | 'desc';
		research_status?: ResearchStatus | 'unset';
	}): Promise<PersonList> {
		const searchParams = new URLSearchParams();
		if (params?.limit) searchParams.set('limit', params.limit.toString());
		if (params?.offset) searchParams.set('offset', params.offset.toString());
		if (params?.sort) searchParams.set('sort', params.sort);
		if (params?.order) searchParams.set('order', params.order);
		if (params?.research_status) searchParams.set('research_status', params.research_status);

		const query = searchParams.toString();
		return this.request<PersonList>('GET', `/persons${query ? `?${query}` : ''}`);
	}

	async getPerson(id: string): Promise<PersonDetail> {
		return this.request<PersonDetail>('GET', `/persons/${id}`);
	}

	async createPerson(data: PersonCreate): Promise<Person> {
		return this.request<Person>('POST', '/persons', data);
	}

	async updatePerson(id: string, data: PersonUpdate): Promise<Person> {
		return this.request<Person>('PUT', `/persons/${id}`, data);
	}

	async deletePerson(id: string): Promise<void> {
		return this.request<void>('DELETE', `/persons/${id}`);
	}

	// Family endpoints
	async listFamilies(params?: { limit?: number; offset?: number }): Promise<FamilyList> {
		const searchParams = new URLSearchParams();
		if (params?.limit) searchParams.set('limit', params.limit.toString());
		if (params?.offset) searchParams.set('offset', params.offset.toString());

		const query = searchParams.toString();
		return this.request<FamilyList>('GET', `/families${query ? `?${query}` : ''}`);
	}

	async getFamily(id: string): Promise<FamilyDetail> {
		return this.request<FamilyDetail>('GET', `/families/${id}`);
	}

	async createFamily(data: FamilyCreate): Promise<Family> {
		return this.request<Family>('POST', '/families', data);
	}

	async updateFamily(id: string, data: FamilyUpdate): Promise<Family> {
		return this.request<Family>('PUT', `/families/${id}`, data);
	}

	async deleteFamily(id: string): Promise<void> {
		return this.request<void>('DELETE', `/families/${id}`);
	}

	async addChildToFamily(familyId: string, data: AddChild): Promise<FamilyChild> {
		return this.request<FamilyChild>('POST', `/families/${familyId}/children`, data);
	}

	async removeChildFromFamily(familyId: string, personId: string): Promise<void> {
		return this.request<void>('DELETE', `/families/${familyId}/children/${personId}`);
	}

	async getFamilyGroupSheet(id: string): Promise<FamilyGroupSheet> {
		return this.request<FamilyGroupSheet>('GET', `/families/${id}/group-sheet`);
	}

	// Pedigree endpoint
	async getPedigree(personId: string, generations?: number): Promise<Pedigree> {
		const params = generations ? `?generations=${generations}` : '';
		return this.request<Pedigree>('GET', `/pedigree/${personId}${params}`);
	}

	// Ahnentafel endpoint
	async getAhnentafel(personId: string, generations?: number): Promise<AhnentafelResponse> {
		const params = generations ? `?generations=${generations}` : '';
		return this.request<AhnentafelResponse>('GET', `/ahnentafel/${personId}${params}`);
	}

	async getAhnentafelText(personId: string, generations?: number): Promise<string> {
		const params = new URLSearchParams();
		params.set('format', 'text');
		if (generations) params.set('generations', generations.toString());

		const response = await fetch(`${API_BASE}/ahnentafel/${personId}?${params.toString()}`);

		if (!response.ok) {
			const error: ApiError = await response.json().catch(() => ({
				code: 'UNKNOWN_ERROR',
				message: response.statusText
			}));
			throw error;
		}

		return response.text();
	}

	// Descendancy endpoint
	async getDescendancy(personId: string, generations?: number): Promise<Descendancy> {
		const params = generations ? `?generations=${generations}` : '';
		return this.request<Descendancy>('GET', `/descendancy/${personId}${params}`);
	}

	// Search endpoint
	async searchPersons(params: {
		q: string;
		fuzzy?: boolean;
		limit?: number;
	}): Promise<SearchResults> {
		const searchParams = new URLSearchParams();
		searchParams.set('q', params.q);
		if (params.fuzzy) searchParams.set('fuzzy', 'true');
		if (params.limit) searchParams.set('limit', params.limit.toString());

		return this.request<SearchResults>('GET', `/search?${searchParams.toString()}`);
	}

	// GEDCOM endpoints
	async importGedcom(file: File): Promise<ImportResult> {
		const formData = new FormData();
		formData.append('file', file);

		const response = await fetch(`${API_BASE}/gedcom/import`, {
			method: 'POST',
			body: formData
		});

		if (!response.ok) {
			const error: ApiError = await response.json().catch(() => ({
				code: 'UNKNOWN_ERROR',
				message: response.statusText
			}));
			throw error;
		}

		return response.json();
	}

	async exportGedcom(): Promise<string> {
		const response = await fetch(`${API_BASE}/gedcom/export`);

		if (!response.ok) {
			const error: ApiError = await response.json().catch(() => ({
				code: 'UNKNOWN_ERROR',
				message: response.statusText
			}));
			throw error;
		}

		return response.text();
	}

	async exportTree(): Promise<string> {
		const response = await fetch(`${API_BASE}/export/tree`);

		if (!response.ok) {
			const error: ApiError = await response.json().catch(() => ({
				code: 'UNKNOWN_ERROR',
				message: response.statusText
			}));
			throw error;
		}

		return response.text();
	}

	async exportPersons(format: 'json' | 'csv', fields?: string[]): Promise<string> {
		const params = new URLSearchParams({ format });
		if (fields?.length) params.set('fields', fields.join(','));

		const response = await fetch(`${API_BASE}/export/persons?${params}`);

		if (!response.ok) {
			const error: ApiError = await response.json().catch(() => ({
				code: 'UNKNOWN_ERROR',
				message: response.statusText
			}));
			throw error;
		}

		return response.text();
	}

	async exportFamilies(format: 'json' | 'csv', fields?: string[]): Promise<string> {
		const params = new URLSearchParams({ format });
		if (fields?.length) params.set('fields', fields.join(','));

		const response = await fetch(`${API_BASE}/export/families?${params}`);

		if (!response.ok) {
			const error: ApiError = await response.json().catch(() => ({
				code: 'UNKNOWN_ERROR',
				message: response.statusText
			}));
			throw error;
		}

		return response.text();
	}

	// Source endpoints
	async listSources(params?: {
		limit?: number;
		offset?: number;
		sort?: string;
		order?: 'asc' | 'desc';
		q?: string;
	}): Promise<SourceListResponse> {
		const searchParams = new URLSearchParams();
		if (params?.limit) searchParams.set('limit', params.limit.toString());
		if (params?.offset) searchParams.set('offset', params.offset.toString());
		if (params?.sort) searchParams.set('sort', params.sort);
		if (params?.order) searchParams.set('order', params.order);
		if (params?.q) searchParams.set('q', params.q);

		const query = searchParams.toString();
		return this.request<SourceListResponse>('GET', `/sources${query ? `?${query}` : ''}`);
	}

	async getSource(id: string): Promise<SourceDetail> {
		return this.request<SourceDetail>('GET', `/sources/${id}`);
	}

	async createSource(data: CreateSourceRequest): Promise<Source> {
		return this.request<Source>('POST', '/sources', data);
	}

	async updateSource(id: string, data: UpdateSourceRequest): Promise<Source> {
		return this.request<Source>('PUT', `/sources/${id}`, data);
	}

	async deleteSource(id: string, version: number): Promise<void> {
		return this.request<void>('DELETE', `/sources/${id}?version=${version}`);
	}

	async searchSources(q: string, limit?: number): Promise<SourceSearchResponse> {
		const searchParams = new URLSearchParams();
		searchParams.set('q', q);
		if (limit) searchParams.set('limit', limit.toString());

		return this.request<SourceSearchResponse>('GET', `/sources/search?${searchParams.toString()}`);
	}

	// Citation endpoints
	async getPersonCitations(personId: string): Promise<CitationListResponse> {
		return this.request<CitationListResponse>('GET', `/persons/${personId}/citations`);
	}

	async createCitation(data: CreateCitationRequest): Promise<Citation> {
		return this.request<Citation>('POST', '/citations', data);
	}

	async updateCitation(id: string, data: UpdateCitationRequest): Promise<Citation> {
		return this.request<Citation>('PUT', `/citations/${id}`, data);
	}

	async deleteCitation(id: string, version: number): Promise<void> {
		return this.request<void>('DELETE', `/citations/${id}?version=${version}`);
	}

	// Media endpoints
	async listPersonMedia(
		personId: string,
		params?: { limit?: number; offset?: number }
	): Promise<MediaListResponse> {
		const searchParams = new URLSearchParams();
		if (params?.limit) searchParams.set('limit', params.limit.toString());
		if (params?.offset) searchParams.set('offset', params.offset.toString());

		const query = searchParams.toString();
		return this.request<MediaListResponse>('GET', `/persons/${personId}/media${query ? `?${query}` : ''}`);
	}

	async uploadPersonMedia(
		personId: string,
		file: File,
		title: string,
		description?: string,
		mediaType?: string
	): Promise<Media> {
		const formData = new FormData();
		formData.append('file', file);
		formData.append('title', title);
		if (description) formData.append('description', description);
		if (mediaType) formData.append('media_type', mediaType);

		const response = await fetch(`${API_BASE}/persons/${personId}/media`, {
			method: 'POST',
			body: formData
		});

		if (!response.ok) {
			const error: ApiError = await response.json().catch(() => ({
				code: 'UNKNOWN_ERROR',
				message: response.statusText
			}));
			throw error;
		}

		return response.json();
	}

	async getMedia(id: string): Promise<Media> {
		return this.request<Media>('GET', `/media/${id}`);
	}

	async updateMedia(id: string, data: MediaUpdate): Promise<Media> {
		return this.request<Media>('PUT', `/media/${id}`, data);
	}

	async deleteMedia(id: string, version: number): Promise<void> {
		return this.request<void>('DELETE', `/media/${id}?version=${version}`);
	}

	getMediaContentUrl(id: string): string {
		return `${API_BASE}/media/${id}/content`;
	}

	getMediaThumbnailUrl(id: string): string {
		return `${API_BASE}/media/${id}/thumbnail`;
	}

	// History endpoints
	async getGlobalHistory(params?: {
		entity_type?: string;
		from?: string;
		to?: string;
		limit?: number;
		offset?: number;
	}): Promise<ChangeHistoryResponse> {
		const searchParams = new URLSearchParams();
		if (params?.entity_type) searchParams.set('entity_type', params.entity_type);
		if (params?.from) searchParams.set('from', params.from);
		if (params?.to) searchParams.set('to', params.to);
		if (params?.limit) searchParams.set('limit', params.limit.toString());
		if (params?.offset) searchParams.set('offset', params.offset.toString());

		const query = searchParams.toString();
		return this.request<ChangeHistoryResponse>('GET', `/history${query ? `?${query}` : ''}`);
	}

	async getPersonHistory(
		personId: string,
		params?: { limit?: number; offset?: number }
	): Promise<ChangeHistoryResponse> {
		const searchParams = new URLSearchParams();
		if (params?.limit) searchParams.set('limit', params.limit.toString());
		if (params?.offset) searchParams.set('offset', params.offset.toString());

		const query = searchParams.toString();
		return this.request<ChangeHistoryResponse>(
			'GET',
			`/persons/${personId}/history${query ? `?${query}` : ''}`
		);
	}

	async getFamilyHistory(
		familyId: string,
		params?: { limit?: number; offset?: number }
	): Promise<ChangeHistoryResponse> {
		const searchParams = new URLSearchParams();
		if (params?.limit) searchParams.set('limit', params.limit.toString());
		if (params?.offset) searchParams.set('offset', params.offset.toString());

		const query = searchParams.toString();
		return this.request<ChangeHistoryResponse>(
			'GET',
			`/families/${familyId}/history${query ? `?${query}` : ''}`
		);
	}

	async getSourceHistory(
		sourceId: string,
		params?: { limit?: number; offset?: number }
	): Promise<ChangeHistoryResponse> {
		const searchParams = new URLSearchParams();
		if (params?.limit) searchParams.set('limit', params.limit.toString());
		if (params?.offset) searchParams.set('offset', params.offset.toString());

		const query = searchParams.toString();
		return this.request<ChangeHistoryResponse>(
			'GET',
			`/sources/${sourceId}/history${query ? `?${query}` : ''}`
		);
	}

	// Browse endpoints
	async getSurnameIndex(letter?: string): Promise<SurnameIndexResponse> {
		const params = letter ? `?letter=${encodeURIComponent(letter)}` : '';
		return this.request<SurnameIndexResponse>('GET', `/browse/surnames${params}`);
	}

	async getPersonsBySurname(
		surname: string,
		params?: { limit?: number; offset?: number }
	): Promise<PersonList> {
		const searchParams = new URLSearchParams();
		if (params?.limit) searchParams.set('limit', params.limit.toString());
		if (params?.offset) searchParams.set('offset', params.offset.toString());

		const query = searchParams.toString();
		return this.request<PersonList>(
			'GET',
			`/browse/surnames/${encodeURIComponent(surname)}/persons${query ? `?${query}` : ''}`
		);
	}

	async getPlaceHierarchy(parent?: string): Promise<PlaceIndexResponse> {
		const params = parent ? `?parent=${encodeURIComponent(parent)}` : '';
		return this.request<PlaceIndexResponse>('GET', `/browse/places${params}`);
	}

	async getPersonsByPlace(
		place: string,
		params?: { limit?: number; offset?: number }
	): Promise<PersonList> {
		const searchParams = new URLSearchParams();
		if (params?.limit) searchParams.set('limit', params.limit.toString());
		if (params?.offset) searchParams.set('offset', params.offset.toString());

		const query = searchParams.toString();
		return this.request<PersonList>(
			'GET',
			`/browse/places/${encodeURIComponent(place)}/persons${query ? `?${query}` : ''}`
		);
	}

	// Relationship endpoint
	async getRelationship(personId1: string, personId2: string): Promise<RelationshipResult> {
		return this.request<RelationshipResult>(
			'GET',
			`/relationship/${encodeURIComponent(personId1)}/${encodeURIComponent(personId2)}`
		);
	}
}

export const api = new ApiClient();

// Utility functions for formatting
export function formatGenDate(date?: GenDate): string {
	if (!date) return '';
	if (date.raw) return date.raw;

	const parts: string[] = [];

	if (date.qualifier && date.qualifier !== 'exact') {
		parts.push(date.qualifier.toUpperCase());
	}

	if (date.day) parts.push(date.day.toString());

	if (date.month) {
		const months = [
			'JAN',
			'FEB',
			'MAR',
			'APR',
			'MAY',
			'JUN',
			'JUL',
			'AUG',
			'SEP',
			'OCT',
			'NOV',
			'DEC'
		];
		parts.push(months[date.month - 1]);
	}

	if (date.year) parts.push(date.year.toString());

	if (date.qualifier === 'bet' && date.year2) {
		parts.push('AND');
		if (date.day2) parts.push(date.day2.toString());
		if (date.month2) {
			const months = [
				'JAN',
				'FEB',
				'MAR',
				'APR',
				'MAY',
				'JUN',
				'JUL',
				'AUG',
				'SEP',
				'OCT',
				'NOV',
				'DEC'
			];
			parts.push(months[date.month2 - 1]);
		}
		parts.push(date.year2.toString());
	}

	return parts.join(' ');
}

export function formatPersonName(person: { given_name: string; surname: string }): string {
	return `${person.given_name} ${person.surname}`;
}

export function formatLifespan(person: { birth_date?: GenDate; death_date?: GenDate }): string {
	const birth = person.birth_date?.year;
	const death = person.death_date?.year;

	if (!birth && !death) return '';
	if (birth && !death) return `(b. ${birth})`;
	if (!birth && death) return `(d. ${death})`;
	return `(${birth}–${death})`;
}
