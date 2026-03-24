<script lang="ts">
	import type { HTMLSelectAttributes } from "svelte/elements";
	import { cn, type WithElementRef } from "$lib/utils.js";

	type Props = WithElementRef<HTMLSelectAttributes>;

	let {
		ref = $bindable(null),
		value = $bindable(),
		children,
		class: className,
		"data-slot": dataSlot = "select",
		...restProps
	}: Props = $props();
</script>

<select
	bind:this={ref}
	data-slot={dataSlot}
	class={cn(
		"dark:bg-input/30 border-input focus-visible:border-ring focus-visible:ring-ring/50 aria-invalid:ring-destructive/20 dark:aria-invalid:ring-destructive/40 aria-invalid:border-destructive dark:aria-invalid:border-destructive/50 h-9 rounded-md border bg-transparent px-3 py-1 text-base shadow-xs transition-[color,box-shadow] focus-visible:ring-3 aria-invalid:ring-3 md:text-sm w-full min-w-0 outline-none disabled:pointer-events-none disabled:cursor-not-allowed disabled:opacity-50",
		className
	)}
	bind:value
	{...restProps}
>
	{@render children?.()}
</select>
