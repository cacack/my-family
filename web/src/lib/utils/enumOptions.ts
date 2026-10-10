/**
 * Option lists for the form controls whose values are API enums. Each list is
 * typed against the union generated from openapi.yaml, so offering a value the
 * API rejects fails `svelte-check` instead of failing the save.
 */

import type {
	EvidenceType,
	Gender,
	InformantType,
	MediaType,
	NameType,
	RelationType,
	ResearchStatus,
	SourceQuality
} from '$lib/api/client';

export interface EnumOption<T extends string> {
	readonly value: T;
	readonly label: string;
}

export const GENDER_OPTIONS: readonly EnumOption<Gender>[] = [
	{ value: 'unknown', label: 'Unknown' },
	{ value: 'male', label: 'Male' },
	{ value: 'female', label: 'Female' }
];

export const RELATION_TYPE_OPTIONS: readonly EnumOption<RelationType>[] = [
	{ value: 'unknown', label: 'Unknown' },
	{ value: 'marriage', label: 'Marriage' },
	{ value: 'partnership', label: 'Partnership' }
];

export const NAME_TYPE_OPTIONS: readonly EnumOption<NameType>[] = [
	{ value: 'birth', label: 'Birth' },
	{ value: 'married', label: 'Married' },
	{ value: 'aka', label: 'AKA' },
	{ value: 'immigrant', label: 'Immigrant' },
	{ value: 'religious', label: 'Religious' },
	{ value: 'professional', label: 'Professional' }
];

export const RESEARCH_STATUS_OPTIONS: readonly (EnumOption<ResearchStatus> & { readonly description: string })[] = [
	{ value: 'certain', label: 'Certain', description: 'Confirmed with strong evidence' },
	{ value: 'probable', label: 'Probable', description: 'Good supporting evidence' },
	{ value: 'possible', label: 'Possible', description: 'Limited evidence' },
	{ value: 'unknown', label: 'Unknown', description: 'Not yet assessed' }
];

/** The media types an upload offers: the API also accepts audio and video. */
export const UPLOAD_MEDIA_TYPE_OPTIONS: readonly EnumOption<MediaType>[] = [
	{ value: 'photo', label: 'Photo' },
	{ value: 'document', label: 'Document' },
	{ value: 'certificate', label: 'Certificate' }
];

export const SOURCE_QUALITY_OPTIONS: readonly EnumOption<SourceQuality>[] = [
	{ value: 'original', label: 'Original' },
	{ value: 'derivative', label: 'Derivative' },
	{ value: 'authored', label: 'Authored' }
];

export const INFORMANT_TYPE_OPTIONS: readonly EnumOption<InformantType>[] = [
	{ value: 'primary', label: 'Primary' },
	{ value: 'secondary', label: 'Secondary' },
	{ value: 'indeterminate', label: 'Indeterminate' }
];

export const EVIDENCE_TYPE_OPTIONS: readonly EnumOption<EvidenceType>[] = [
	{ value: 'direct', label: 'Direct' },
	{ value: 'indirect', label: 'Indirect' },
	{ value: 'negative', label: 'Negative' }
];
