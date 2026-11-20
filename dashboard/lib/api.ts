// Typed client for the Mini Vercel API.

export const API_URL =
  process.env.NEXT_PUBLIC_API_URL ??
  (process.env.NODE_ENV === "development" ? "http://localhost:4000" : "");

export type Status = "QUEUED" | "BUILDING" | "UPLOADING" | "READY" | "FAILED" | "CANCELED";

export const ACTIVE: Status[] = ["QUEUED", "BUILDING", "UPLOADING"];
export const isActive = (s: Status) => ACTIVE.includes(s);

export interface Deployment {
  id: string;
  projectId: string;
  source: "git" | "zip";
  trigger: "manual" | "api" | "webhook" | "redeploy";
  branch: string;
  commitSha: string;
  commitMessage: string;
  status: Status;
  error: string;
  attempt: number;
  fileCount: number;
  sizeBytes: number;
  installCommand: string;
  buildCommand: string;
  outputDirectory: string;
  rootDirectory: string;
  createdAt: string;
  startedAt: string | null;
  finishedAt: string | null;
  url: string;
  isProduction: boolean;
  durationMs: number | null;
}

export interface RecentDeployment extends Deployment {
  projectName: string;
  projectSlug: string;
}

export interface Alias {
  hostname: string;
  projectId: string;
  deploymentId: string;
  updatedAt: string;
}

export interface Project {
  id: string;
  name: string;
  slug: string;
  repoUrl: string;
  branch: string;
  framework: string;
  installCommand: string;
  buildCommand: string;
  outputDirectory: string;
  rootDirectory: string;
  createdAt: string;
  updatedAt: string;
  productionUrl: string;
  productionDeployment: Deployment | null;
  latestDeployment: Deployment | null;
  webhookUrl?: string;
  webhookSecret?: string;
  aliases?: Alias[];
}

export interface Preset {
  id: string;
  name: string;
  description: string;
  installCommand: string;
  buildCommand: string;
  outputDirectory: string;
}

export interface EnvVar {
  key: string;
  preview: string;
  updatedAt: string;
}

export interface LogLine {
  seq: number;
  ts: string;
  stream: "stdout" | "stderr" | "system";
  line: string;
}

export interface Stats {
  projects: number;
  deployments: number;
  ready: number;
  failed: number;
  building: number;
  avgBuildSeconds: number;
  totalBytes: number;
}

export interface PlatformConfig {
  rootDomain: string;
  publicScheme: string;
  publicPort: string;
  authRequired: boolean;
  limits: { cpus: number; memoryMB: number; pids: number; timeoutSec: number; image: string };
}

export class ApiError extends Error {
  constructor(public status: number, public code: string, message: string) {
    super(message);
  }
}

const TOKEN_KEY = "mv_token";
export function getToken(): string {
  try {
    return localStorage.getItem(TOKEN_KEY) ?? "";
  } catch {
    return "";
  }
}
export function setToken(t: string) {
  try {
    localStorage.setItem(TOKEN_KEY, t);
  } catch {
    /* ignore */
  }
}

async function request<T>(path: string, init: RequestInit = {}): Promise<T> {
  const headers = new Headers(init.headers);
  const token = getToken();
  if (token) headers.set("Authorization", `Bearer ${token}`);
  if (init.body && !(init.body instanceof FormData)) headers.set("Content-Type", "application/json");
  let res: Response;
  try {
    res = await fetch(`${API_URL}/api${path}`, { ...init, headers });
  } catch {
    throw new ApiError(0, "network", "Cannot reach the API server. Is it running?");
  }
  if (res.status === 204) return undefined as T;
  const body = await res.json().catch(() => null);
  if (!res.ok) {
    throw new ApiError(res.status, body?.error?.code ?? "error", body?.error?.message ?? res.statusText);
  }
  return body as T;
}

export const api = {
  config: () => request<PlatformConfig>("/config"),
  presets: () => request<Preset[]>("/presets"),
  stats: () => request<Stats>("/stats"),
  recent: () => request<RecentDeployment[]>("/deployments"),

  projects: () => request<Project[]>("/projects"),
  project: (id: string) => request<Project>(`/projects/${id}`),
  createProject: (body: Record<string, unknown>) =>
    request<{ project: Project; deployment?: Deployment }>("/projects", { method: "POST", body: JSON.stringify(body) }),
  updateProject: (id: string, body: Record<string, unknown>) =>
    request<Project>(`/projects/${id}`, { method: "PATCH", body: JSON.stringify(body) }),
  deleteProject: (id: string) => request<void>(`/projects/${id}`, { method: "DELETE" }),

  env: (id: string) => request<EnvVar[]>(`/projects/${id}/env`),
  setEnv: (id: string, key: string, value: string) =>
    request<void>(`/projects/${id}/env`, { method: "PUT", body: JSON.stringify({ key, value }) }),
  deleteEnv: (id: string, key: string) =>
    request<void>(`/projects/${id}/env/${encodeURIComponent(key)}`, { method: "DELETE" }),

  deployments: (projectId: string) => request<Deployment[]>(`/projects/${projectId}/deployments`),
  deploy: (projectId: string) =>
    request<Deployment>(`/projects/${projectId}/deployments`, { method: "POST", body: JSON.stringify({}) }),
  deployZip: (projectId: string, file: File) => {
    const fd = new FormData();
    fd.append("file", file);
    return request<Deployment>(`/projects/${projectId}/deployments`, { method: "POST", body: fd });
  },
  deployment: (id: string) =>
    request<{ deployment: Deployment; project: { id: string; name: string; slug: string; repoUrl: string; productionUrl: string } }>(
      `/deployments/${id}`,
    ),
  cancel: (id: string) => request<Deployment>(`/deployments/${id}/cancel`, { method: "POST" }),
  promote: (id: string) =>
    request<{ deployment: Deployment; hostname: string; tookMs: number }>(`/deployments/${id}/promote`, { method: "POST" }),
  redeploy: (id: string) => request<Deployment>(`/deployments/${id}/redeploy`, { method: "POST" }),

  eventsUrl: (id: string) => {
    const t = getToken();
    return `${API_URL}/api/deployments/${id}/events${t ? `?token=${encodeURIComponent(t)}` : ""}`;
  },
};
