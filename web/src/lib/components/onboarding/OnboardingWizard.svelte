<script lang="ts">
	import { setOnboardingCompleted } from '$lib/stores/onboardingSettings.svelte';
	import WelcomeStep from './WelcomeStep.svelte';
	import ImportStep from './ImportStep.svelte';
	import CreatePersonStep from './CreatePersonStep.svelte';
	import CompletionStep from './CompletionStep.svelte';

	interface Props {
		onComplete: () => void;
	}

	let { onComplete }: Props = $props();

	type WizardStep = 'welcome' | 'import' | 'create-person' | 'completion';
	let currentStep: WizardStep = $state('welcome');
	let completionData = $state<{ personCount?: number; personId?: string; personName?: string }>({});

	function finishOnboarding() {
		setOnboardingCompleted(true);
		onComplete();
	}
</script>

<div class="onboarding-wizard">
	<div class="wizard-container">
		{#if currentStep === 'welcome'}
			<WelcomeStep
				onSelectImport={() => { currentStep = 'import'; }}
				onSelectCreate={() => { currentStep = 'create-person'; }}
				onSkip={finishOnboarding}
			/>
		{:else if currentStep === 'import'}
			<ImportStep
				onComplete={(data) => {
					completionData = { personCount: data.personCount };
					setOnboardingCompleted(true);
					currentStep = 'completion';
				}}
				onBack={() => { currentStep = 'welcome'; }}
			/>
		{:else if currentStep === 'create-person'}
			<CreatePersonStep
				onComplete={(data) => {
					completionData = { personId: data.personId, personName: data.personName };
					setOnboardingCompleted(true);
					currentStep = 'completion';
				}}
				onBack={() => { currentStep = 'welcome'; }}
			/>
		{:else if currentStep === 'completion'}
			<CompletionStep
				{completionData}
				onFinish={finishOnboarding}
			/>
		{/if}
	</div>
</div>

<style>
	.onboarding-wizard {
		min-height: 60vh;
		display: flex;
		align-items: center;
		justify-content: center;
		padding: 2rem;
	}

	.wizard-container {
		width: 100%;
	}
</style>
