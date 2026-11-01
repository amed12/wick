<script lang="ts">
  import { renderMarkdown } from "@wick-fe/common-md";
  import type { Draft, Plan } from "../types.js";

  let {
    plan,
    draft,
    busy = false,
    onSave,
    onGenerate,
    onApply,
    onDiscard,
  }: {
    plan: Plan | null;
    draft: Draft | null;
    busy?: boolean;
    onSave: (prd: string) => void;
    onGenerate: () => void;
    onApply: () => void;
    onDiscard: () => void;
  } = $props();

  let mode = $state<"preview" | "edit">("preview");
  let text = $state("");

  // The document belongs to the server; re-seed the editor whenever the
  // plan (or its version) changes, but never while the person is typing.
  $effect(() => {
    if (plan) text = plan.prd ?? "";
  });

  const hasPRD = $derived(!!(plan?.prd && plan.prd.trim()));
  const dirty = $derived(plan ? text !== (plan.prd ?? "") : false);
  // With no document yet the editor is the only useful surface, so the
  // "Edit markdown" tab is the active one regardless of the last click.
  const activeMode = $derived(hasPRD ? mode : "edit");

  const tabClass = (active: boolean) =>
    active
      ? "rounded-md bg-white-100 px-3 py-1.5 text-xs font-semibold text-black-900 shadow-sm dark:bg-navy-600 dark:text-white-100"
      : "rounded-md px-3 py-1.5 text-xs font-medium text-black-700 hover:text-black-900 dark:text-black-600 dark:hover:text-white-100";
</script>

<section class="flex min-h-0 flex-1 flex-col">
  <div class="flex flex-wrap items-center gap-2 border-b border-white-300 px-4 py-2 dark:border-navy-600">
    <div class="flex items-center gap-1 rounded-lg bg-white-200 p-0.5 dark:bg-navy-800">
      <button type="button" class={tabClass(activeMode === "preview")} onclick={() => (mode = "preview")}>Preview</button>
      <button type="button" class={tabClass(activeMode === "edit")} onclick={() => (mode = "edit")}>Edit markdown</button>
    </div>
    {#if plan && plan.version > 0}
      <span class="rounded-md bg-white-200 px-2 py-1 text-[11px] font-medium text-black-700 dark:bg-navy-800 dark:text-black-600">
        v{plan.version}
      </span>
    {/if}
    <span class="flex-1"></span>
    {#if dirty}
      <button
        type="button"
        class="rounded-lg bg-green-500 px-3 py-1.5 text-xs font-semibold text-white-100 hover:brightness-95 disabled:opacity-50"
        disabled={busy}
        onclick={() => onSave(text)}>Simpan PRD</button
      >
    {/if}
    <button
      type="button"
      class="rounded-lg border border-white-400 px-3 py-1.5 text-xs font-medium text-black-800 hover:bg-white-200 disabled:opacity-50 dark:border-navy-600 dark:text-black-600 dark:hover:bg-navy-700"
      disabled={busy}
      onclick={onGenerate}
    >
      {busy ? "Menyusun…" : hasPRD ? "Susun ulang" : "✦ Susun PRD"}
    </button>
  </div>

  {#if draft}
    <div class="border-b border-green-500/30 bg-green-500/5 px-4 py-3">
      <p class="text-sm font-semibold text-black-900 dark:text-white-100">
        Draft siap: {draft.title || plan?.title}
      </p>
      <p class="mt-0.5 text-xs text-black-700 dark:text-black-600">
        {draft.summary || "PRD + breakdown fitur dan task"} ·
        {draft.features.length} fitur ·
        {draft.features.reduce((n, f) => n + (f.tasks?.length ?? 0), 0)} task
      </p>
      <div class="mt-2 flex gap-2">
        <button
          type="button"
          class="rounded-lg bg-green-500 px-3 py-1.5 text-xs font-semibold text-white-100 disabled:opacity-50"
          disabled={busy}
          onclick={onApply}>Terapkan ke plan</button
        >
        <button
          type="button"
          class="rounded-lg border border-white-400 px-3 py-1.5 text-xs font-medium text-black-800 dark:border-navy-600 dark:text-black-600"
          onclick={onDiscard}>Buang</button
        >
      </div>
    </div>
  {/if}

  <div class="min-h-0 flex-1 overflow-auto">
    {#if !plan}
      <p class="px-6 py-10 text-sm text-black-700 dark:text-black-600">Pilih atau buat plan dulu.</p>
    {:else if mode === "edit" || !hasPRD}
      <textarea
        class="h-full min-h-[420px] w-full resize-none bg-transparent px-6 py-5 font-mono text-[13px] leading-6 text-black-900 outline-none placeholder:text-black-600 dark:text-white-100 dark:placeholder:text-black-600"
        bind:value={text}
        spellcheck="false"
        placeholder="Tulis PRD di sini, atau tekan ✦ Susun PRD untuk membuat draft dari ide."
      ></textarea>
    {:else}
      <article class="prose-wick mx-auto max-w-3xl px-8 py-7 text-sm leading-7 text-black-900 dark:text-white-100">
        {@html renderMarkdown(plan.prd ?? "")}
      </article>
    {/if}
  </div>
</section>

<style>
  .prose-wick :global(h1) {
    font-size: 1.5rem;
    font-weight: 700;
    margin: 0.25rem 0 1rem;
  }
  .prose-wick :global(h2) {
    font-size: 1.05rem;
    font-weight: 650;
    margin: 1.75rem 0 0.5rem;
    padding-bottom: 0.35rem;
    border-bottom: 1px solid color-mix(in srgb, currentColor 10%, transparent);
  }
  .prose-wick :global(h3) {
    font-size: 0.95rem;
    font-weight: 600;
    margin: 1.1rem 0 0.35rem;
  }
  .prose-wick :global(p) {
    margin: 0.55rem 0;
  }
  .prose-wick :global(ul),
  .prose-wick :global(ol) {
    margin: 0.5rem 0;
    padding-left: 1.25rem;
  }
  .prose-wick :global(ul) {
    list-style: disc;
  }
  .prose-wick :global(ol) {
    list-style: decimal;
  }
  .prose-wick :global(li) {
    margin: 0.2rem 0;
  }
  .prose-wick :global(code) {
    font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
    font-size: 0.85em;
    background: color-mix(in srgb, currentColor 8%, transparent);
    padding: 0.1rem 0.3rem;
    border-radius: 0.25rem;
  }
  .prose-wick :global(pre) {
    margin: 0.75rem 0;
    padding: 0.85rem 1rem;
    overflow: auto;
    border-radius: 0.6rem;
    background: color-mix(in srgb, currentColor 7%, transparent);
  }
  .prose-wick :global(pre code) {
    background: none;
    padding: 0;
  }
  .prose-wick :global(table) {
    width: 100%;
    border-collapse: collapse;
    margin: 0.75rem 0;
    font-size: 0.85em;
  }
  .prose-wick :global(th),
  .prose-wick :global(td) {
    border: 1px solid color-mix(in srgb, currentColor 15%, transparent);
    padding: 0.35rem 0.6rem;
    text-align: left;
  }
  .prose-wick :global(strong) {
    font-weight: 650;
  }
</style>
