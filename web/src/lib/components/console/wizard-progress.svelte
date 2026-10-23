<script lang="ts">
	import { Check } from '@lucide/svelte';
	let {
		steps,
		current,
		onselect,
		disabled = false
	}: {
		steps: readonly { id: number; label: string; description: string }[];
		current: number;
		onselect: (id: number) => void;
		disabled?: boolean;
	} = $props();
</script>

<nav aria-label="Setup progress" class="wizard-progress" style:--step-count={steps.length}>
	<ol>
		{#each steps as step (step.id)}
			<li>
				<button
					type="button"
					class={[
						'wizard-step',
						current === step.id && 'wizard-step-current',
						current > step.id && 'wizard-step-complete'
					]}
					aria-current={current === step.id ? 'step' : undefined}
					{disabled}
					onclick={() => onselect(step.id)}
				>
					<span class="wizard-step-number"
						>{#if current > step.id}<Check class="size-4" />{:else}{String(step.id).padStart(
								2,
								'0'
							)}{/if}</span
					>
					<span
						><span class="block text-xs font-semibold">{step.label}</span><span
							class="mt-1 hidden text-[11px] text-muted-foreground sm:block"
							>{step.description}</span
						></span
					>
				</button>
			</li>
		{/each}
	</ol>
</nav>
