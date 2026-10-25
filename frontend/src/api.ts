// 세션과 CSRF를 사용하는 v1 API 클라이언트를 제공한다.
import { getPasskey } from "./webauthn";
export type User = {
  id: string;
  username: string;
  role: "admin" | "operator" | "viewer";
  is_active: boolean;
  created_at: string;
};
export type List<T> = { items: T[]; total: number };
export type Schema = {
  type?: string;
  title?: string;
  description?: string;
  default?: unknown;
  enum?: unknown[];
  properties?: Record<string, Schema>;
  required?: string[];
  minimum?: number;
  maximum?: number;
  minLength?: number;
  maxLength?: number;
  format?: string;
  writeOnly?: boolean;
  items?: Schema;
};
export type Task = {
  id: string;
  title: string;
  input_schema?: Schema;
  timeout_seconds?: number;
  idempotent?: boolean;
};
export type Manifest = {
  id: string;
  name: string;
  version: string;
  tasks?: Task[];
  resources?: { id: string; methods: string[] }[];
  menus?: { id: string; title: string; icon: string; resource?: string }[];
  settings_schema?: Schema;
  data_schema_version: number;
};
export type Plugin = {
  id: string;
  name: string;
  version: string;
  state: "active" | "disabled";
  repository_url: string;
  active_release_id: string;
  manifest: Manifest;
};
export type Menu = {
  id: string;
  plugin_id: string;
  plugin_name: string;
  title: string;
  icon: string;
  position: number;
  enabled: boolean;
};
export type Run = {
  id: string;
  plugin_id: string;
  release_id: string;
  task_id: string;
  state: string;
  progress: number;
  created_at: string;
  started_at: string | null;
  finished_at: string | null;
  error?: string | null;
  input?: unknown;
  result?: unknown;
};
export type Schedule = {
  id: string;
  name: string;
  plugin_id: string;
  task_id: string;
  cron: string;
  timezone: string;
  input: Record<string, unknown>;
  enabled: boolean;
  next_run_at: string;
  last_run_at: string | null;
};
export type Status = {
  version: string;
  started_at: string;
  plugins_active: number;
  runs_queued: number;
  runs_running: number;
  runs_succeeded: number;
  runs_failed: number;
};
export class APIError extends Error {
  constructor(
    message: string,
    public status: number,
    public code = "",
  ) {
    super(message);
  }
}
let csrfToken = "";

export async function request<T>(
  path: string,
  options: RequestInit = {},
): Promise<T> {
  const headers = new Headers(options.headers);
  if (options.body) headers.set("Content-Type", "application/json");
  if (!["GET", "HEAD"].includes(options.method || "GET") && csrfToken)
    headers.set("X-CSRF-Token", csrfToken);
  const response = await fetch(`/api/v1${path}`, {
    ...options,
    headers,
    credentials: "same-origin",
  });
  if (!response.ok) {
    const data = await response.json().catch(() => null);
    const publicAuth = [
      "/auth/login",
      "/auth/options",
      "/auth/mfa",
      "/auth/passkey/begin",
      "/auth/passkey/finish",
      "/auth/login/cancel",
    ];
    if (
      response.status === 401 &&
      data?.error?.code !== "invalid_credentials" &&
      !publicAuth.includes(path)
    ) {
      csrfToken = "";
      window.dispatchEvent(new Event("modudock:unauthorized"));
    }
    throw new APIError(
      data?.error?.message || `요청에 실패했습니다. (${response.status})`,
      response.status,
      data?.error?.code,
    );
  }
  if (response.status === 204) return undefined as T;
  return response.json() as Promise<T>;
}
export async function session(): Promise<User> {
  const data = await request<{ user: User; csrf_token: string }>("/auth/me");
  csrfToken = data.csrf_token;
  return data.user;
}
export async function signIn(
  username: string,
  password: string,
  signal?: AbortSignal,
): Promise<LoginResult> {
  const data = await request<LoginResult>("/auth/login", {
    method: "POST",
    body: JSON.stringify({ username, password }),
    signal,
  });
  if (signal?.aborted) throw new DOMException("Cancelled", "AbortError");
  csrfToken = "user" in data ? data.csrf_token : "";
  return data;
}
export type LoginResult =
  | { user: User; csrf_token: string }
  | { mfa_required: true; expires_at: string };
type AuthResponse = { user: User; csrf_token: string };
export async function completeMFA(
  method: "totp" | "recovery",
  code: string,
  signal?: AbortSignal,
): Promise<User> {
  const data = await request<AuthResponse>("/auth/mfa", {
    method: "POST",
    body: JSON.stringify({ method, code }),
    signal,
  });
  if (signal?.aborted) throw new DOMException("Cancelled", "AbortError");
  csrfToken = data.csrf_token;
  return data.user;
}
export async function signInWithPasskey(signal?: AbortSignal): Promise<User> {
  const options = await request<unknown>("/auth/passkey/begin", {
    method: "POST",
    body: "{}",
    signal,
  });
  const credential = await getPasskey(options, signal);
  const data = await request<AuthResponse>("/auth/passkey/finish", {
    method: "POST",
    body: JSON.stringify({ credential }),
    signal,
  });
  if (signal?.aborted) throw new DOMException("Cancelled", "AbortError");
  csrfToken = data.csrf_token;
  return data.user;
}
export async function signOut() {
  try {
    await request("/auth/logout", { method: "POST" });
  } catch (error) {
    if (!(error instanceof APIError) || error.status !== 401) throw error;
  }
  csrfToken = "";
}
export const body = (value: unknown) => JSON.stringify(value);
export const id = (value: string) => encodeURIComponent(value);
