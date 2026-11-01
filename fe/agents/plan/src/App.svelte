<script lang="ts">
  import { onMount } from "svelte";
  import { ToastHost } from "@wick-fe/common-ui";
  import { toastError, toastOk } from "@wick-fe/common-stores";
  import PrdPanel from "$lib/components/PrdPanel.svelte";
  import RoadmapCanvas from "$lib/components/RoadmapCanvas.svelte";
  import FeatureDetail from "$lib/components/FeatureDetail.svelte";
  import {
    ApiError,
    addContext,
    applyFeatures,
    createPlan,
    draftFromJob,
    generatePRD,
    getBase,
    getJob,
    getPlan,
    listPlans,
    listProjects,
    removeContext,
    updatePlan,
  } from "$lib/api.js";
  import type { Draft, FeatureNode, Plan, PlanRow, ProjectOption } from "$lib/types.js";

  const base = getBase();

  let projects = $state<ProjectOption[]>([]);
  let projectId = $state("");
  let plans = $state<PlanRow[]>([]);
  let planId = $state("");
  let plan = $state<Plan | null>(null);
  let features = $state<FeatureNode[]>([]);

  let booting = $state(true);
  let loading = $state(false);
  let saving = $state(false);
  let busy = $state(false);
  let error = $state("");

  let tab = $state<"prd" | "roadmap">("prd");
  let selected = $state("");
  let draft = $state<Draft | null>(null);
  let jobID = $state("");
  let pollTimer: ReturnType<typeof setTimeout> | undefined;

  let createOpen = $state(false);
  let newTitle = $state("");
  let newIdea = $state("");
  let creating = $state(false);

  let ctxOpen = $state(false);
  let ctxKind = $state("text");
  let ctxRef = $state("");
  let ctxLabel = $state("");

  const selectedNode = $derived(features.find((f) => f.feature.id === selected) ?? null);
  const activePlan = $derived(plan);
  const project = $derived(projects.find((p) => p.id === projectId) ?? null);

  function fail(e: unknown) {
    const msg = e instanceof ApiError ? e.message : e instanceof Error ? e.message : String(e);
    error = msg;
    toastError(msg);
  }

  function clearError() {
    error = "";
  }

  function syncURL() {
    const url = new URL(window.location.href);
    if (projectId) url.searchParams.set("project", projectId);
    if (planId) url.searchParams.set("plan", planId);
    window.history.replaceState(null, "", url);
  }

  async function loadProjects() {
    try {
      projects = await listProjects();
    } catch (e) {
      fail(e);
      return;
    }
    const wanted = new URLSearchParams(window.location.search).get("project");
    const stored = (() => {
      try {
        return localStorage.getItem("wick.plan.project") ?? "";
      } catch {
        return "";
      }
    })();
    projectId =
      (wanted && projects.some((p) => p.id === wanted) ? wanted : "") ||
      (stored && projects.some((p) => p.id === stored) ? stored : "") ||
      projects[0]?.id ||
      "";
    if (projectId) await loadPlans();
  }

  async function loadPlans() {
    if (!projectId) return;
    loading = true;
    try {
      plans = await listPlans(projectId);
      const wanted = new URLSearchParams(window.location.search).get("plan");
      const next =
        (wanted && plans.some((p) => p.id === wanted) ? wanted : "") ||
        (plans.some((p) => p.id === planId) ? planId : "") ||
        plans[0]?.id ||
        "";
      if (next) await openPlan(next);
      else {
        planId = "";
        plan = null;
        features = [];
        syncURL();
      }
    } catch (e) {
      fail(e);
    } finally {
      loading = false;
    }
  }

  async function openPlan(id: string) {
    if (!projectId) return;
    planId = id;
    selected = "";
    draft = null;
    try {
      const detail = await getPlan(projectId, id);
      plan = detail.plan;
      features = detail.features ?? [];
    } catch (e) {
      fail(e);
      return;
    }
    try {
      localStorage.setItem("wick.plan.project", projectId);
    } catch {
      /* storage unavailable — URL still carries the selection */
    }
    syncURL();
  }

  async function switchProject(id: string) {
    projectId = id;
    planId = "";
    plan = null;
    features = [];
    await loadPlans();
  }

  async function create() {
    if (!projectId || !newTitle.trim()) return;
    creating = true;
    clearError();
    try {
      const p = await createPlan(projectId, { title: newTitle.trim(), idea: newIdea.trim() });
      newTitle = "";
      newIdea = "";
      createOpen = false;
      await loadPlans();
      await openPlan(p.id);
      toastOk("Plan dibuat");
    } catch (e) {
      fail(e);
    } finally {
      creating = false;
    }
  }

  async function savePRD(prd: string) {
    if (!plan) return;
    saving = true;
    clearError();
    try {
      plan = await updatePlan(projectId, plan.id, { prd, note: "manual edit" });
      await loadPlans();
      toastOk(`PRD disimpan (v${plan.version})`);
    } catch (e) {
      fail(e);
    } finally {
      saving = false;
    }
  }

  function stopPolling() {
    if (pollTimer) clearTimeout(pollTimer);
    pollTimer = undefined;
  }

  async function poll(job: string) {
    try {
      const st = await getJob(job);
      if (st.status === "done") {
        const d = draftFromJob(st);
        stopPolling();
        jobID = "";
        busy = false;
        if (!d) {
          fail(new Error("Agent tidak mengembalikan PRD"));
          return;
        }
        draft = d;
        tab = "prd";
        toastOk("Draft PRD siap — periksa lalu terapkan");
        return;
      }
      if (st.status === "failed" || st.status === "canceled") {
        stopPolling();
        jobID = "";
        busy = false;
        fail(new Error(st.error || "Generate gagal"));
        return;
      }
      pollTimer = setTimeout(() => poll(job), 1500);
    } catch (e) {
      stopPolling();
      jobID = "";
      busy = false;
      fail(e);
    }
  }

  async function generate() {
    if (!plan) return;
    if (busy) return;
    clearError();
    busy = true;
    draft = null;
    const context = (plan.context ?? [])
      .map((c) => `[${c.kind}] ${c.label || c.ref}\n${c.ref}`)
      .join("\n\n");
    try {
      jobID = await generatePRD({
        idea: plan.idea || plan.title,
        title: plan.title,
        projectName: project?.name ?? "",
        existingPRD: plan.prd ?? "",
        context,
      });
      poll(jobID);
    } catch (e) {
      busy = false;
      fail(e);
    }
  }

  async function applyDraft() {
    if (!plan || !draft) return;
    busy = true;
    clearError();
    try {
      plan = await updatePlan(projectId, plan.id, {
        prd: draft.prd,
        summary: draft.summary || plan.summary,
        note: "generated",
      });
      if (draft.features.length > 0) {
        const detail = await applyFeatures(projectId, plan.id, draft.features);
        plan = detail.plan;
        features = detail.features ?? [];
      } else {
        await openPlan(plan.id);
      }
      draft = null;
      await loadPlans();
      tab = "roadmap";
      toastOk("Plan diterapkan");
    } catch (e) {
      fail(e);
    } finally {
      busy = false;
    }
  }

  async function setStatus(status: Plan["status"]) {
    if (!plan) return;
    try {
      plan = await updatePlan(projectId, plan.id, { status });
      await loadPlans();
    } catch (e) {
      fail(e);
    }
  }

  async function addCtx() {
    if (!plan || !ctxRef.trim()) return;
    try {
      plan = await addContext(projectId, plan.id, {
        kind: ctxKind,
        ref: ctxRef.trim(),
        label: ctxLabel.trim(),
      });
      ctxRef = "";
      ctxLabel = "";
      ctxOpen = false;
    } catch (e) {
      fail(e);
    }
  }

  async function removeCtx(kind: string, ref: string) {
    if (!plan) return;
    try {
      await removeContext(projectId, plan.id, kind, ref);
      await openPlan(plan.id);
    } catch (e) {
      fail(e);
    }
  }

  onMount(async () => {
    await loadProjects();
    booting = false;
  });

  const tabClass = (active: boolean) =>
    active
      ? "relative px-1 py-3 text-sm font-semibold text-black-900 after:absolute after:inset-x-0 after:-bottom-px after:h-0.5 after:rounded-full after:bg-green-500 dark:text-white-100"
      : "relative px-1 py-3 text-sm font-medium text-black-700 transition-colors hover:text-black-900 dark:text-black-600 dark:hover:text-white-100";

  const statusClass = (s: string) =>
    s === "done"
      ? "bg-green-500/10 text-green-700 dark:text-green-400"
      : s === "active"
        ? "bg-amber-500/10 text-amber-700 dark:text-amber-300"
        : "bg-white-300 text-black-700 dark:bg-navy-600 dark:text-black-600";
</script>

<ToastHost />

<div class="flex h-screen min-h-0 flex-col bg-white-200 text-black-900 dark:bg-navy-800 dark:text-white-100">
  <header class="flex flex-wrap items-center gap-3 border-b border-white-300 bg-white-100 px-4 py-2.5 dark:border-navy-600 dark:bg-navy-700">
    <div class="flex min-w-0 items-center gap-2">
      <span class="hidden text-[11px] font-semibold uppercase tracking-wider text-black-700 sm:inline dark:text-black-600">Plans</span>
      <select
        class="max-w-[220px] rounded-lg border border-white-400 bg-white-100 px-2.5 py-1.5 text-xs font-medium text-black-900 outline-none dark:border-navy-600 dark:bg-navy-800 dark:text-white-100"
        value={projectId}
        onchange={(e) => switchProject((e.currentTarget as HTMLSelectElement).value)}
      >
        {#each projects as p (p.id)}
          <option value={p.id}>{p.name}</option>
        {/each}
      </select>

      <select
        class="max-w-[220px] rounded-lg border border-white-400 bg-white-100 px-2.5 py-1.5 text-xs font-medium text-black-900 outline-none dark:border-navy-600 dark:bg-navy-800 dark:text-white-100"
        value={planId}
        disabled={plans.length === 0}
        onchange={(e) => openPlan((e.currentTarget as HTMLSelectElement).value)}
      >
        {#if plans.length === 0}
          <option value="">Belum ada plan</option>
        {/if}
        {#each plans as p (p.id)}
          <option value={p.id}>Plan {p.number} · {p.title}</option>
        {/each}
      </select>
    </div>

    <button
      type="button"
      class="rounded-lg border border-white-400 px-3 py-1.5 text-xs font-semibold text-black-800 transition-colors hover:border-green-500 hover:text-green-700 dark:border-navy-600 dark:text-black-600 dark:hover:text-green-400"
      onclick={() => (createOpen = true)}>+ Plan baru</button
    >

    {#if plan}
      <button
        type="button"
        class={"rounded-lg px-2.5 py-1.5 text-[11px] font-semibold " + statusClass(plan.status)}
        title="Klik untuk mengubah status plan"
        onclick={() => setStatus(plan.status === "draft" ? "active" : plan.status === "active" ? "done" : "draft")}
      >
        {plan.status}
      </button>
    {/if}

    <span class="flex-1"></span>
    {#if plan}
      <span class="hidden text-[11px] text-black-700 md:inline dark:text-black-600">
        {features.length} fitur · {features.reduce((n, f) => n + f.done, 0)}/{features.reduce((n, f) => n + f.total, 0)} task
      </span>
    {/if}
  </header>

  {#if error}
    <div class="flex items-center gap-3 border-b border-rose-500/30 bg-rose-500/5 px-4 py-2 text-xs text-rose-700 dark:text-rose-400">
      <span class="min-w-0 flex-1 truncate">{error}</span>
      <button type="button" class="font-semibold" onclick={clearError}>Tutup</button>
    </div>
  {/if}

  <div class="flex min-h-0 flex-1">
    <!-- Phases + context rail -->
    <aside class="hidden w-60 shrink-0 flex-col overflow-auto border-r border-white-300 bg-white-100 md:flex dark:border-navy-600 dark:bg-navy-700">
      <div class="px-3 pt-4">
        <p class="px-1 text-[11px] font-semibold uppercase tracking-wider text-black-700 dark:text-black-600">Fase</p>
        <ul class="mt-1.5 flex flex-col gap-0.5">
          {#each plans as p (p.id)}
            <li>
              <button
                type="button"
                onclick={() => openPlan(p.id)}
                class={"w-full rounded-lg px-2.5 py-2 text-left transition-colors " +
                  (p.id === planId
                    ? "bg-green-500/10 text-green-700 dark:text-green-400"
                    : "text-black-800 hover:bg-white-200 dark:text-black-600 dark:hover:bg-navy-800")}
              >
                <span class="block text-xs font-medium">Plan {p.number} · {p.title}</span>
                <span class="mt-0.5 block text-[10px] opacity-70">
                  {p.total > 0 ? `${p.done}/${p.total} task` : "belum dipecah"}
                </span>
              </button>
            </li>
          {/each}
        </ul>
        {#if plans.length === 0 && !booting}
          <p class="px-1 py-2 text-[11px] text-black-700 dark:text-black-600">Belum ada plan di project ini.</p>
        {/if}
      </div>

      <div class="mt-5 border-t border-white-300 px-3 pt-3 dark:border-navy-600">
        <div class="flex items-center justify-between">
          <p class="px-1 text-[11px] font-semibold uppercase tracking-wider text-black-700 dark:text-black-600">Konteks</p>
          <button type="button" class="text-xs font-semibold text-green-700 hover:underline dark:text-green-400" onclick={() => (ctxOpen = !ctxOpen)}>
            {ctxOpen ? "Tutup" : "+ Tambah"}
          </button>
        </div>

        {#if ctxOpen}
          <div class="mt-2 rounded-lg border border-white-300 p-2 dark:border-navy-600">
            <select
              class="mb-1.5 w-full rounded border border-white-400 bg-white-100 px-2 py-1 text-[11px] dark:border-navy-600 dark:bg-navy-800"
              bind:value={ctxKind}
            >
              <option value="text">Teks</option>
              <option value="url">URL</option>
              <option value="file">File</option>
              <option value="note">Catatan</option>
            </select>
            <input
              class="mb-1.5 w-full rounded border border-white-400 bg-white-100 px-2 py-1 text-[11px] dark:border-navy-600 dark:bg-navy-800"
              placeholder="Label (opsional)"
              bind:value={ctxLabel}
            />
            <textarea
              class="mb-1.5 h-20 w-full resize-none rounded border border-white-400 bg-white-100 px-2 py-1 text-[11px] dark:border-navy-600 dark:bg-navy-800"
              placeholder="Path / URL / teks"
              bind:value={ctxRef}
            ></textarea>
            <button
              type="button"
              class="w-full rounded bg-green-500 px-2 py-1 text-[11px] font-semibold text-white-100 disabled:opacity-50"
              disabled={!ctxRef.trim()}
              onclick={addCtx}>Tambahkan</button
            >
          </div>
        {/if}

        <ul class="mt-2 flex flex-col gap-1 pb-4">
          {#each plan?.context ?? [] as c (c.kind + c.ref)}
            <li class="group flex items-start gap-1 rounded px-1 py-1 hover:bg-white-200 dark:hover:bg-navy-800">
              <span class="mt-0.5 rounded bg-white-300 px-1 text-[9px] font-semibold uppercase dark:bg-navy-600">{c.kind}</span>
              <span class="min-w-0 flex-1 truncate text-[11px] text-black-800 dark:text-black-600" title={c.ref}>{c.label || c.ref}</span>
              <button
                type="button"
                class="text-[10px] text-black-700 opacity-0 transition-opacity group-hover:opacity-100 dark:text-black-600"
                onclick={() => removeCtx(c.kind, c.ref)}>✕</button
              >
            </li>
          {/each}
          {#if (plan?.context ?? []).length === 0}
            <li class="px-1 text-[11px] text-black-700 dark:text-black-600">Belum ada konteks.</li>
          {/if}
        </ul>
      </div>
    </aside>

    <main class="flex min-w-0 flex-1 flex-col bg-white-200 dark:bg-navy-800">
      {#if booting}
        <div class="flex flex-1 items-center justify-center text-sm text-black-700 dark:text-black-600">Memuat…</div>
      {:else if !projectId}
        <div class="flex flex-1 flex-col items-center justify-center gap-3 px-6 text-center">
          <h1 class="text-xl font-semibold">Belum ada project</h1>
          <p class="max-w-md text-sm text-black-700 dark:text-black-600">
            Plan hidup di dalam project. Buat project dulu di halaman Projects, lalu kembali ke sini.
          </p>
          <a href={`${base}/projects/new`} class="rounded-lg bg-green-500 px-4 py-2 text-sm font-semibold text-white-100">Buat project</a>
        </div>
      {:else if !plan && !loading}
        <div class="mx-auto flex w-full max-w-2xl flex-1 flex-col justify-center px-6 py-10">
          <h1 class="text-2xl font-semibold tracking-tight">Mau bangun apa?</h1>
          <p class="mt-1 text-sm text-black-700 dark:text-black-600">
            Tulis idenya sekali. Wick menyusun PRD, memecahnya jadi fitur dan task, lalu agent mengerjakannya dari plan yang sama.
          </p>
          <div class="mt-6 rounded-xl border border-white-400 bg-white-100 p-4 dark:border-navy-600 dark:bg-navy-700">
            <input
              class="w-full rounded-lg border border-white-400 bg-white-100 px-3 py-2 text-sm outline-none focus:border-green-500 dark:border-navy-600 dark:bg-navy-800"
              placeholder={'Judul plan, mis. "MVP kasir warung"'}
              bind:value={newTitle}
            />
            <textarea
              class="mt-2 h-32 w-full resize-none rounded-lg border border-white-400 bg-white-100 px-3 py-2 text-sm outline-none focus:border-green-500 dark:border-navy-600 dark:bg-navy-800"
              placeholder="Contoh: aplikasi kasir untuk warung, bisa catat penjualan harian, stok, dan laporan bulanan lewat WhatsApp."
              bind:value={newIdea}
            ></textarea>
            <div class="mt-2 flex justify-end">
              <button
                type="button"
                class="rounded-lg bg-green-500 px-4 py-2 text-sm font-semibold text-white-100 disabled:opacity-50"
                disabled={!newTitle.trim() || creating}
                onclick={create}>{creating ? "Membuat…" : "Buat plan"}</button
              >
            </div>
          </div>
          <p class="mt-3 text-[11px] text-black-700 dark:text-black-600">
            Setelah plan dibuat, tekan <strong>✦ Susun PRD</strong> untuk membuat dokumen dan breakdown-nya.
          </p>
        </div>
      {:else if plan}
        <div class="flex items-center gap-6 border-b border-white-300 bg-white-100 px-4 dark:border-navy-600 dark:bg-navy-700">
          <nav class="flex items-center gap-5" aria-label="Plan sections">
            <button type="button" class={tabClass(tab === "prd")} onclick={() => (tab = "prd")}>PRD</button>
            <button type="button" class={tabClass(tab === "roadmap")} onclick={() => (tab = "roadmap")}>Roadmap</button>
          </nav>
          <span class="flex-1"></span>
          <span class="truncate text-[11px] text-black-700 dark:text-black-600">
            {plan.title}{plan.summary ? ` — ${plan.summary}` : ""}
          </span>
        </div>

        <div class="flex min-h-0 flex-1">
          {#if tab === "prd"}
            <PrdPanel
              {plan}
              {draft}
              busy={busy || saving}
              onSave={savePRD}
              onGenerate={generate}
              onApply={applyDraft}
              onDiscard={() => (draft = null)}
            />
          {:else}
            <div class="min-h-0 flex-1 overflow-auto">
              <RoadmapCanvas {plan} {features} {selected} onselect={(id) => (selected = id)} />
            </div>
          {/if}

          {#if tab === "roadmap" && selectedNode}
            <FeatureDetail node={selectedNode} {base} {projectId} onClose={() => (selected = "")} />
          {/if}
        </div>
      {/if}
    </main>
  </div>
</div>

{#if createOpen}
  <div class="fixed inset-0 z-50 flex items-center justify-center bg-navy-900/40 p-4 backdrop-blur-sm" role="presentation">
    <div class="w-full max-w-lg rounded-2xl border border-white-300 bg-white-100 p-5 shadow-xl dark:border-navy-600 dark:bg-navy-700" role="dialog" aria-modal="true">
      <h2 class="text-sm font-semibold">Plan baru</h2>
      <p class="mt-1 text-xs text-black-700 dark:text-black-600">Satu plan = satu fase. Plan berikutnya mulai dari yang sudah selesai.</p>
      <input
        class="mt-3 w-full rounded-lg border border-white-400 bg-white-100 px-3 py-2 text-sm outline-none focus:border-green-500 dark:border-navy-600 dark:bg-navy-800"
        placeholder="Judul plan"
        bind:value={newTitle}
      />
      <textarea
        class="mt-2 h-28 w-full resize-none rounded-lg border border-white-400 bg-white-100 px-3 py-2 text-sm outline-none focus:border-green-500 dark:border-navy-600 dark:bg-navy-800"
        placeholder="Ide / goal untuk fase ini"
        bind:value={newIdea}
      ></textarea>
      <div class="mt-4 flex justify-end gap-2">
        <button
          type="button"
          class="rounded-lg border border-white-400 px-3 py-1.5 text-xs font-medium dark:border-navy-600"
          onclick={() => (createOpen = false)}>Batal</button
        >
        <button
          type="button"
          class="rounded-lg bg-green-500 px-4 py-1.5 text-xs font-semibold text-white-100 disabled:opacity-50"
          disabled={!newTitle.trim() || creating}
          onclick={create}>{creating ? "Membuat…" : "Buat"}</button
        >
      </div>
    </div>
  </div>
{/if}
