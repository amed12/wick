<script lang="ts">
  import type { FeatureNode, Plan } from "../types.js";

  let {
    plan,
    features,
    selectedId,
    onselect,
  }: {
    plan: Plan | null;
    features: FeatureNode[];
    selectedId: string;
    onselect: (id: string) => void;
  } = $props();

  const statusTone = (status: string) => {
    switch (status) {
      case "done":
        return "bg-green-500";
      case "in_progress":
        return "bg-amber-500";
      case "waiting":
        return "bg-sky-500";
      default:
        return "bg-white-400 dark:bg-navy-600";
    }
  };

  const percent = (n: FeatureNode) => (n.total === 0 ? 0 : Math.round((n.done / n.total) * 100));
</script>

<div class="relative min-h-full px-6 py-6">
  <div class="flex items-start gap-8">
    <div class="w-56 shrink-0">
      <div class="relative rounded-xl border border-green-500/40 bg-green-500/5 p-4">
        <p class="text-[11px] font-semibold uppercase tracking-wider text-green-700 dark:text-green-400">Perencanaan</p>
        <p class="mt-1 line-clamp-2 text-sm font-semibold text-black-900 dark:text-white-100">
          {plan?.title ?? "Plan"}
        </p>
        <p class="mt-1 text-[11px] text-black-700 dark:text-black-600">
          Plan {plan?.number ?? 1} · v{plan?.version ?? 0}
        </p>
        <div class="mt-3 flex items-center justify-between text-[11px] text-black-700 dark:text-black-600">
          <span>{features.length} fitur</span>
          <span>{features.reduce((n, f) => n + f.total, 0)} task</span>
        </div>
      </div>
      <p class="mt-3 px-1 text-[11px] leading-5 text-black-700 dark:text-black-600">
        Klik salah satu kotak untuk melihat detail fitur dan task-nya. Progress diambil langsung dari status ticket.
      </p>
    </div>

    <div class="relative min-w-0 flex-1 pl-6">
      <div class="absolute bottom-4 left-0 top-4 w-px bg-white-400 dark:bg-navy-600"></div>
      <div class="absolute left-0 top-14 h-px w-6 bg-white-400 dark:bg-navy-600"></div>

      {#if features.length === 0}
        <div class="rounded-xl border border-dashed border-white-400 p-6 text-sm text-black-700 dark:border-navy-600 dark:text-black-600">
          Belum ada fitur. Susun PRD lalu terapkan breakdown-nya, atau tambahkan fitur manual.
        </div>
      {/if}

      {#each features as node (node.feature.id)}
        <button
          type="button"
          onclick={() => onselect(node.feature.id)}
          class={"relative mb-3 block w-full rounded-xl border p-4 text-left transition-colors " +
            (selectedId === node.feature.id
              ? "border-green-500 bg-white-100 shadow-sm dark:bg-navy-700"
              : "border-white-400 bg-white-100 hover:border-green-500/50 dark:border-navy-600 dark:bg-navy-700/60")}
        >
          <span class="absolute -left-6 top-8 h-px w-6 bg-white-400 dark:bg-navy-600"></span>
          <span class="absolute -left-[27px] top-[30px] h-1.5 w-1.5 rounded-full bg-green-500"></span>

          <div class="flex items-start gap-3">
            <div class="min-w-0 flex-1">
              <p class="truncate text-sm font-semibold text-black-900 dark:text-white-100">{node.feature.title}</p>
              <p class="mt-0.5 line-clamp-2 text-xs text-black-700 dark:text-black-600">
                {node.feature.body || "Tanpa deskripsi"}
              </p>
            </div>
            <div class="flex shrink-0 items-center gap-2 text-[11px] text-black-700 dark:text-black-600">
              <span class={"h-1.5 w-1.5 rounded-full " + statusTone(node.feature.status)}></span>
              {node.feature.status}
            </div>
          </div>

          {#if node.total > 0}
            <div class="mt-3 flex items-center gap-3">
              <div class="h-1 flex-1 overflow-hidden rounded-full bg-white-300 dark:bg-navy-600">
                <div class="h-full rounded-full bg-green-500" style={`width:${percent(node)}%`}></div>
              </div>
              <span class="text-[11px] font-medium text-black-700 dark:text-black-600">{node.done}/{node.total}</span>
            </div>
          {:else}
            <p class="mt-3 text-[11px] text-black-700 dark:text-black-600">Belum ada task</p>
          {/if}
        </button>
      {/each}
    </div>
  </div>
</div>
