/**
 * Presentation helpers for relationship calculator results.
 */
import type { RelationshipPath, RelationshipPathNode } from '$lib/api/client';

export interface ChainNode extends RelationshipPathNode {
	isCommonAncestor: boolean;
}

/**
 * The path as one chain A → … → common ancestor → … → B, each person once.
 * `pathFromA` runs from A up to the ancestor and `pathFromB` from B up to it,
 * so the ancestor ends both lists.
 */
export function relationshipChain(path: RelationshipPath): ChainNode[] {
	const nodes: RelationshipPathNode[] = [];
	for (const node of [...(path.pathFromA ?? []), ...(path.pathFromB ?? []).slice().reverse()]) {
		if (nodes[nodes.length - 1]?.id !== node.id) nodes.push(node);
	}
	return nodes.map((node) => ({ ...node, isCommonAncestor: node.id === path.commonAncestorId }));
}

const GENDERED_ENDINGS: [string, string, string][] = [
	['parent', 'father', 'mother'],
	['child', 'son', 'daughter'],
	['sibling', 'brother', 'sister']
];

/**
 * The relationship name for someone of the given gender, e.g. "parent" →
 * "mother", "great-grand-uncle/aunt" → "great-grand-aunt". Names without a
 * gendered form (cousins) and unknown genders are returned unchanged.
 */
export function genderedRelationship(name: string, gender?: string): string {
	if (gender !== 'male' && gender !== 'female') return name;
	const male = gender === 'male';
	const pair = name.match(/^(.*?)(\w+)\/(\w+)$/);
	if (pair) return pair[1] + (male ? pair[2] : pair[3]);
	for (const [ending, maleForm, femaleForm] of GENDERED_ENDINGS) {
		if (name.endsWith(ending)) return name.slice(0, -ending.length) + (male ? maleForm : femaleForm);
	}
	return name;
}

/** Relationship names that differ from the primary one, each listed once. */
export function additionalRelationships(names: string[]): string[] {
	const [primary, ...rest] = names;
	return [...new Set(rest)].filter((name) => name !== primary);
}
