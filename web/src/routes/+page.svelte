<script lang="ts">
	import { api, type Person, type FamilyDetail } from '$lib/api/client';
	import PersonCard from '$lib/components/PersonCard.svelte';
	import FamilyCard from '$lib/components/FamilyCard.svelte';
	import DiscoveryFeed from '$lib/components/DiscoveryFeed.svelte';
	import { onboardingState } from '$lib/stores/onboardingSettings.svelte';
	import OnboardingWizard from '$lib/components/onboarding/OnboardingWizard.svelte';
	import { Card, CardHeader, CardContent } from '$lib/components/ui/card';

	let recentPersons: Person[] = $state([]);
	let recentFamilies: FamilyDetail[] = $state([]);
	let stats = $state({ persons: 0, families: 0 });
	let loading = $state(true);
	let showOnboarding = $state(false);
	let hasSuggestions = $state(false);

	async function loadDashboard() {
		loading = true;
		try {
			const [personsRes, familiesRes] = await Promise.all([
				api.listPersons({ limit: 5, sort: 'updated_at', order: 'desc' }),
				api.listFamilies({ limit: 5 })
			]);
			recentPersons = personsRes.items;
			recentFamilies = familiesRes.items;
			stats = {
				persons: personsRes.total,
				families: familiesRes.total
			};
			showOnboarding = stats.persons === 0 && !onboardingState.completed;

			// Check if there are discovery suggestions (only if we have data)
			if (stats.persons > 0) {
				try {
					const discoveryRes = await api.getDiscoveryFeed(1);
					hasSuggestions = discoveryRes.total > 0;
				} catch {
					hasSuggestions = false;
				}
			}
		} catch (e) {
			console.error('Failed to load dashboard:', e);
			showOnboarding = !onboardingState.completed;
		} finally {
			loading = false;
		}
	}

	$effect(() => {
		loadDashboard();
	});
</script>

<svelte:head>
	<title>My Family | Genealogy</title>
</svelte:head>

{#if showOnboarding}
	<OnboardingWizard onComplete={() => { showOnboarding = false; loadDashboard(); }} />
{:else}
<div class="dashboard">
	<section class="hero">
		<h1>My Family</h1>
		<p>Self-hosted genealogy software</p>
	</section>

	{#if loading}
		<div class="loading">Loading...</div>
	{:else}
		<section class="stats">
			<Card class="flex flex-col items-center px-12 py-6">
				<span class="text-3xl font-bold text-foreground">{stats.persons}</span>
				<span class="mt-1 text-sm text-muted-foreground">People</span>
			</Card>
			<Card class="flex flex-col items-center px-12 py-6">
				<span class="text-3xl font-bold text-foreground">{stats.families}</span>
				<span class="mt-1 text-sm text-muted-foreground">Families</span>
			</Card>
		</section>

		{#if hasSuggestions}
			<Card class="mb-8">
				<CardHeader>
					<h2 class="m-0 text-base font-semibold text-foreground">Research Suggestions</h2>
				</CardHeader>
				<CardContent>
					<DiscoveryFeed />
				</CardContent>
			</Card>
		{/if}

		<div class="content-grid">
			<Card>
				<CardHeader class="flex-row items-center justify-between">
					<h2 class="m-0 text-base font-semibold text-foreground">Recent People</h2>
					<a href="/persons" class="text-sm text-primary no-underline hover:underline">View all</a>
				</CardHeader>
				<CardContent>
					{#if recentPersons.length === 0}
						<p class="empty">No people yet. <a href="/import">Import a GEDCOM file</a> to get started.</p>
					{:else}
						<div class="card-list">
							{#each recentPersons as person}
								<PersonCard {person} href="/persons/{person.id}" variant="compact" />
							{/each}
						</div>
					{/if}
				</CardContent>
			</Card>

			<Card>
				<CardHeader class="flex-row items-center justify-between">
					<h2 class="m-0 text-base font-semibold text-foreground">Recent Families</h2>
					<a href="/families" class="text-sm text-primary no-underline hover:underline">View all</a>
				</CardHeader>
				<CardContent>
					{#if recentFamilies.length === 0}
						<p class="empty">No families yet.</p>
					{:else}
						<div class="card-list">
							{#each recentFamilies as family}
								<FamilyCard {family} href="/families/{family.id}" />
							{/each}
						</div>
					{/if}
				</CardContent>
			</Card>
		</div>

		<Card>
			<CardHeader>
				<h2 class="m-0 text-base font-semibold text-foreground">Quick Actions</h2>
			</CardHeader>
			<CardContent>
				<div class="action-buttons">
				<a href="/import" class="action-btn primary">
					<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
						<path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4" />
						<polyline points="17 8 12 3 7 8" />
						<line x1="12" y1="3" x2="12" y2="15" />
					</svg>
					Import GEDCOM
				</a>
				<a href="/persons/add" class="action-btn">
					<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
						<path d="M16 21v-2a4 4 0 0 0-4-4H6a4 4 0 0 0-4 4v2" />
						<circle cx="9" cy="7" r="4" />
						<line x1="19" y1="8" x2="19" y2="14" />
						<line x1="16" y1="11" x2="22" y2="11" />
					</svg>
					Add Person
				</a>
				<a href="/families/add" class="action-btn">
					<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
						<path d="M17 21v-2a4 4 0 0 0-4-4H5a4 4 0 0 0-4 4v2" />
						<circle cx="9" cy="7" r="4" />
						<path d="M23 21v-2a4 4 0 0 0-3-3.87" />
						<path d="M16 3.13a4 4 0 0 1 0 7.75" />
					</svg>
					Add Family
				</a>
				<a href="/persons/quick" class="action-btn accent">
					<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
						<polygon points="13 2 3 14 12 14 11 22 21 10 12 10 13 2" />
					</svg>
					Quick Capture
				</a>
			</div>
			</CardContent>
		</Card>
	{/if}
</div>
{/if}

<style>
	.dashboard {
		max-width: 1200px;
		margin: 0 auto;
		padding: 2rem;
	}

	.hero {
		text-align: center;
		padding: 2rem 0 3rem;
	}

	.hero h1 {
		margin: 0;
		font-size: 2.5rem;
		color: #1e293b;
	}

	.hero p {
		margin: 0.5rem 0 0;
		color: #64748b;
		font-size: 1.125rem;
	}

	.loading {
		text-align: center;
		padding: 3rem;
		color: #64748b;
	}

	.stats {
		display: flex;
		gap: 1rem;
		justify-content: center;
		margin-bottom: 2rem;
	}

	.content-grid {
		display: grid;
		grid-template-columns: repeat(2, 1fr);
		gap: 1.5rem;
		margin-bottom: 2rem;
	}

	@media (max-width: 768px) {
		.content-grid {
			grid-template-columns: 1fr;
		}
	}

	.card-list {
		display: flex;
		flex-direction: column;
		gap: 0.5rem;
	}

	.empty {
		color: #94a3b8;
		font-size: 0.875rem;
		text-align: center;
		padding: 1rem;
	}

	.empty a {
		color: #3b82f6;
	}

	.action-buttons {
		display: flex;
		gap: 1rem;
		flex-wrap: wrap;
	}

	.action-btn {
		display: inline-flex;
		align-items: center;
		gap: 0.5rem;
		padding: 0.75rem 1.25rem;
		border-radius: 8px;
		font-size: 0.875rem;
		font-weight: 500;
		text-decoration: none;
		border: 1px solid #e2e8f0;
		background: white;
		color: #475569;
		transition: all 0.15s;
	}

	.action-btn:hover {
		border-color: #cbd5e1;
		background: #f8fafc;
	}

	.action-btn.primary {
		background: #3b82f6;
		border-color: #3b82f6;
		color: white;
	}

	.action-btn.primary:hover {
		background: #2563eb;
		border-color: #2563eb;
	}

	.action-btn svg {
		width: 1.25rem;
		height: 1.25rem;
	}

	.action-btn.accent {
		background: #f0f9ff;
		border-color: #7dd3fc;
		color: #0369a1;
	}

	.action-btn.accent:hover {
		background: #e0f2fe;
		border-color: #38bdf8;
	}
</style>
