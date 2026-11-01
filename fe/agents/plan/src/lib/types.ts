export interface ProjectOption {
  id: string;
  name: string;
  path?: string;
}

export interface PlanVersion {
  number: number;
  prd: string;
  note?: string;
  created_at: string;
}

export interface PlanContext {
  kind: string;
  ref: string;
  label?: string;
  added_at: string;
}

export interface Plan {
  id: string;
  project_id: string;
  number: number;
  title: string;
  idea?: string;
  prd?: string;
  status: "draft" | "active" | "done";
  version: number;
  versions?: PlanVersion[];
  features?: string[];
  context?: PlanContext[];
  summary?: string;
  created_at: string;
  updated_at: string;
}

export interface Ticket {
  id: string;
  project_id: string;
  title: string;
  body?: string;
  status: string;
  kind?: string;
  parent_id?: string;
  plan_id?: string;
  acceptance?: string;
  depends_on?: string[];
  assignee?: string;
  created_at: string;
  updated_at: string;
}

export interface FeatureNode {
  feature: Ticket;
  tasks: Ticket[];
  done: number;
  total: number;
}

export interface PlanRow extends Plan {
  feature_count: number;
  done: number;
  total: number;
}

export interface PlanDetail {
  plan: Plan;
  features: FeatureNode[];
}

export interface TaskSpec {
  title: string;
  body?: string;
  acceptance?: string;
  depends_on?: string[];
}

export interface FeatureSpec {
  title: string;
  body?: string;
  acceptance?: string;
  depends_on?: string[];
  tasks?: TaskSpec[];
}

export interface Draft {
  title: string;
  summary: string;
  prd: string;
  features: FeatureSpec[];
}

export interface GenJob {
  id: string;
  status: "queued" | "working" | "done" | "failed" | "canceled";
  position?: number;
  /** The provider answer; narrow it with draftFromJob before use. */
  result?: unknown;
  error?: string;
}
