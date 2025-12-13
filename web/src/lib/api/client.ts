/**
 * My Family Genealogy API Client
 * Generated from OpenAPI spec - contracts/openapi.yaml
 */

const API_BASE = '/api/v1';

// Types based on OpenAPI schemas
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
	version: number;
}

export interface PersonSummary {
	id: string;
	given_name: string;
	surname: string;
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
	id: string;
	name: string;
	relationship_type: 'biological' | 'adopted' | 'foster';
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
	}): Promise<PersonList> {
		const searchParams = new URLSearchParams();
		if (params?.limit) searchParams.set('limit', params.limit.toString());
		if (params?.offset) searchParams.set('offset', params.offset.toString());
		if (params?.sort) searchParams.set('sort', params.sort);
		if (params?.order) searchParams.set('order', params.order);

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

	// Pedigree endpoint
	async getPedigree(personId: string, generations?: number): Promise<Pedigree> {
		const params = generations ? `?generations=${generations}` : '';
		return this.request<Pedigree>('GET', `/pedigree/${personId}${params}`);
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
