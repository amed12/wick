<script lang="ts">
  import { renderMarkdown } from "@wick-fe/common-md";
  import type { FeatureNode } from "../types.js";

  let {
    node,
    base,
    projectId,
    onClose,
  }: {
    node: FeatureNode | null;
    base: string;
    projectId: string;
    onClose: () => void;
  } = $props();

  const boardHref = (ticketId: string) =>
    `${base}/sessions?project=${encodeURIComponent(projectId)}&ticket=${encodeURIComponent(ticketId)}`;

  /* renderMarkdown has no task-list rule, so a "- [ ]" acceptance line
     would come out as a bullet followed by literal brackets. Swap the
     boxes for the characters a reader expects, keeping the bullet. */
  const checklist = (md: string) =>
    md
      .replace(/^(\s*)[-*]\s+\[\s\]\s+/gm, "$1- ☐ ")
      .replace(/^(\s*)[-*]\s+\[[xX]\]\s+/gm, "$1- ☑ ");

  const statusTone = (status: string) => {
    switch (status) {
      case "done":
        return "bg-green-500/10 text-green-700 dark:text-green-400";
      case "in_progress":
        return "bg-amber-500/10 text-amber-700 dark:text-amber-300";
      case "waiting":
        return "bg-sky-500/10 text-sky-700 dark:text-sky-300";
      default:
        return "bg-white-300 text-black-700 dark:bg-navy-600 dark:text-black-600";
    }
  };
</script>

{#if node}
  <aside class="flex h-full w-[340px] shrink-0 flex-col border-l border-white-300 bg-white-100 dark:border-navy-600 dark:bg-navy-700">
    <div class="flex items-start gap-2 border-b border-white-300 px-4 py-3 dark:border-navy-600">
      <div class="min-w-0 flex-1">
        <p class="text-[11px] font-semibold uppercase tracking-wider text-black-700 dark:text-black-600">Fitur</p>
        <h2 class="mt-0.5 truncate text-sm font-semibold text-black-900 dark:text-white-100">{node.feature.title}</h2>
        <p class="mt-0.5 text-[11px] text-black-700 dark:text-black-600">{node.feature.id} · {node.done}/{node.total} task</p>
      </div>
      <button
        type="button"
        onclick={onClose}
        aria-label="Tutup detail"
        class="flex h-7 w-7 items-center justify-center rounded-md text-black-700 hover:bg-white-200 dark:text-black-600 dark:hover:bg-navy-800"
      >✕</button>
    </div>

    <div class="min-h-0 flex-1 overflow-auto px-4 py-3">
      {#if node.feature.body}
        <div class="mb-4 text-xs leading-6 text-black-800 dark:text-black-600">
          {@html renderMarkdown(node.feature.body)}
        </div>
      {/if}

      {#if node.feature.acceptance}
        <div class="mb-4 rounded-lg border border-white-300 bg-white-200/60 p-3 dark:border-navy-600 dark:bg-navy-800/60">
          <p class="mb-1 text-[11px] font-semibold uppercase tracking-wider text-black-700 dark:text-black-600">Kriteria selesai</p>
          <div class="text-xs leading-6 text-black-800 dark:text-black-600">
            {@html renderMarkdown(checklist(node.feature.acceptance))}
          </div>
        </div>
      {/if}

      <div class="mb-2 flex items-center justify-between">
        <h3 class="text-xs font-semibold text-black-900 dark:text-white-100">Task</h3>
        <a
          href={boardHref(node.feature.id)}
          class="text-[11px] font-medium text-green-700 hover:underline dark:text-green-400"
        >Buka board →</a>
      </div>

      {#if node.tasks.length === 0}
        <p class="text-xs text-black-700 dark:text-black-600">Belum ada task untuk fitur ini.</p>
      {:else}
        <ul class="flex flex-col gap-2">
          {#each node.tasks as task (task.id)}
            <li class="rounded-lg border border-white-300 p-3 dark:border-navy-600">
              <div class="flex items-start gap-2">
                <div class="min-w-0 flex-1">
                  <p class="text-xs font-medium text-black-900 dark:text-white-100">{task.title}</p>
                  <p class="mt-0.5 text-[11px] text-black-700 dark:text-black-600">{task.id}</p>
                </div>
                <span class={"shrink-0 rounded px-1.5 py-0.5 text-[10px] font-medium " + statusTone(task.status)}>
                  {task.status}
                </span>
              </div>
              {#if task.acceptance}
                <div class="mt-2 text-[11px] leading-5 text-black-700 dark:text-black-600">
                  {@html renderMarkdown(checklist(task.acceptance))}
                </div>
              {/if}
              <a
                href={boardHref(task.id)}
                class="mt-2 inline-block text-[11px] font-medium text-green-700 hover:underline dark:text-green-400"
              >Kerjakan dengan agent →</a>
            </li>
          {/each}
        </ul>
      {/if}
    </div>
  </aside>
{/if}
