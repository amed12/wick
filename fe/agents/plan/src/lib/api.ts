import type {
  Draft,
  FeatureSpec,
  GenJob,
  Plan,
  PlanDetail,
  PlanRow,
  ProjectOption,
} from "./types.js";

export class ApiError extends Error {
  constructor(public readonly status: number, message: string) {
    super(message);
  }
}

/** getBase reads the mount path the Go shell put on #app. */
export function getBase(): string {
  return document.getElementById("app")?.dataset.base ?? "";
}

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const resp = await fetch(path, {
    credentials: "same-origin",
    headers: { Accept: "application/json", ...(init?.body ? { "Content-Type": "application/json" } : {}) },
    ...init,
  });
  if (!resp.ok) {
    let message = `HTTP ${resp.status}`;
    try {
      const body = (await resp.json()) as { error?: string };
      if (body?.error) message = body.error;
    } catch {
      /* non-JSON error body — keep the status message */
    }
    throw new ApiError(resp.status, message);
  }
  return resp.json() as Promise<T>;
}

const planPath = (projectID: string, planID?: string) => {
  const base = getBase();
  const root = `${base}/api/projects/${encodeURIComponent(projectID)}/plans`;
  return planID ? `${root}/${encodeURIComponent(planID)}` : root;
};

export function listProjects(): Promise<ProjectOption[]> {
  return request<ProjectOption[]>(`${getBase()}/projects/options`);
}

export async function listPlans(projectID: string): Promise<PlanRow[]> {
  const out = await request<{ plans: PlanRow[] }>(planPath(projectID));
  return out.plans ?? [];
}

export function getPlan(projectID: string, planID: string): Promise<PlanDetail> {
  return request<PlanDetail>(planPath(projectID, planID));
}

export async function createPlan(
  projectID: string,
  input: { title: string; idea?: string },
): Promise<Plan> {
  const out = await request<{ plan: Plan }>(planPath(projectID), {
    method: "POST",
    body: JSON.stringify(input),
  });
  return out.plan;
}

export async function updatePlan(
  projectID: string,
  planID: string,
  patch: Partial<Pick<Plan, "title" | "idea" | "summary" | "status" | "prd">> & { note?: string },
): Promise<Plan> {
  const out = await request<{ plan: Plan }>(planPath(projectID, planID), {
    method: "PATCH",
    body: JSON.stringify(patch),
  });
  return out.plan;
}

export async function deletePlan(projectID: string, planID: string): Promise<void> {
  await request<{ deleted: string }>(planPath(projectID, planID), { method: "DELETE" });
}

export function applyFeatures(
  projectID: string,
  planID: string,
  features: FeatureSpec[],
): Promise<PlanDetail> {
  return request<PlanDetail>(`${planPath(projectID, planID)}/features`, {
    method: "POST",
    body: JSON.stringify({ features }),
  });
}

export async function addContext(
  projectID: string,
  planID: string,
  input: { kind: string; ref: string; label?: string },
): Promise<Plan> {
  const out = await request<{ plan: Plan }>(`${planPath(projectID, planID)}/context`, {
    method: "POST",
    body: JSON.stringify(input),
  });
  return out.plan;
}

export async function removeContext(
  projectID: string,
  planID: string,
  kind: string,
  ref: string,
): Promise<void> {
  const q = new URLSearchParams({ kind, ref });
  await request(`${planPath(projectID, planID)}/context?${q}`, { method: "DELETE" });
}

/**
 * generatePRD starts the one-shot planning job. It goes through the shared
 * aigen queue (root mount, not the agents base) so a burst of clicks waits
 * behind the agent pool instead of forking several CLIs at once.
 */
export async function generatePRD(input: {
  idea: string;
  title?: string;
  projectName?: string;
  existingPRD?: string;
  context?: string;
}): Promise<string> {
  const out = await request<{ id: string }>(`/api/ai-gen`, {
    method: "POST",
    body: JSON.stringify({
      kind: "plan-prd",
      input: {
        text: input.idea,
        fields: {
          idea: input.idea,
          title: input.title ?? "",
          project_name: input.projectName ?? "",
          existing_prd: input.existingPRD ?? "",
          context: input.context ?? "",
        },
      },
    }),
  });
  return out.id;
}

export function getJob(id: string): Promise<GenJob> {
  return request<GenJob>(`/api/ai-gen/${encodeURIComponent(id)}`);
}

/** draftsFromJob narrows the job result to a Draft without trusting it. */
export function draftFromJob(job: GenJob): Draft | null {
  const r = job.result as Draft | undefined;
  if (!r || typeof r.prd !== "string" || !r.prd) return null;
  return { ...r, features: Array.isArray(r.features) ? r.features : [] };
}
