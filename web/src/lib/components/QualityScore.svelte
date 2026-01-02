<script lang="ts">
	interface Props {
		score: number;
		size?: 'small' | 'medium' | 'large';
	}

	let { score, size = 'medium' }: Props = $props();

	const color = $derived(() => {
		if (score <= 40) return '#ef4444'; // Red
		if (score <= 70) return '#eab308'; // Yellow
		return '#22c55e'; // Green
	});

	const dimensions = $derived(() => {
		switch (size) {
			case 'small':
				return { width: 60, height: 8, fontSize: '0.75rem' };
			case 'large':
				return { width: 200, height: 16, fontSize: '1.25rem' };
			default:
				return { width: 120, height: 12, fontSize: '1rem' };
		}
	});
</script>

<div class="quality-score" class:small={size === 'small'} class:large={size === 'large'}>
	<div
		class="progress-bar"
		style:width="{dimensions().width}px"
		style:height="{dimensions().height}px"
	>
		<div
			class="progress-fill"
			style:width="{score}%"
			style:background-color={color()}
		></div>
	</div>
	<span class="score-text" style:font-size={dimensions().fontSize}>{score}%</span>
</div>

<style>
	.quality-score {
		display: inline-flex;
		align-items: center;
		gap: 0.5rem;
	}

	.quality-score.small {
		gap: 0.25rem;
	}

	.quality-score.large {
		gap: 0.75rem;
	}

	.progress-bar {
		background: #e2e8f0;
		border-radius: 999px;
		overflow: hidden;
	}

	.progress-fill {
		height: 100%;
		border-radius: 999px;
		transition: width 0.3s ease;
	}

	.score-text {
		font-weight: 600;
		color: #1e293b;
		min-width: 3em;
	}

	.small .score-text {
		min-width: 2.5em;
	}
</style>
