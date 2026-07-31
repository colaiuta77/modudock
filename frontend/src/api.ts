// 인증 토큰을 포함해 ModuDock API를 호출하는 작은 클라이언트다.
const API_URL = import.meta.env.VITE_API_URL || "";

export type Plugin = {
  id: string;
  name: string;
  repository_url: string;
  state: "installing" | "active" | "disabled" | "failed";
  active_release_id: string | null;
};

export type User = {
  id: string;
  username: string;
  role: "admin" | "operator" | "viewer";
  is_active: boolean;
};

export type TaskRun = {
  id: string;
  plugin_id: string | null;
  task_name: string;
  state: string;
  progress: number;
  created_at: string;
  error: string | null;
};

export type Schedule = {
  id: string;
  name: string;
  plugin_id: string;
  task_name: string;
  cron: string;
  timezone: string;
  input: Record<string, unknown>;
  enabled: boolean;
  next_run_at: string;
  last_run_at: string | null;
};

export type LogEntry = {
  id: number;
  level: string;
  message: string;
  plugin_id: string | null;
  task_id: string | null;
  run_id: string | null;
  user_id: string | null;
  timestamp: string;
  context: Record<string, unknown>;
};

export type PluginMenu = {
  plugin_id: string;
  plugin_name: string;
  menu_id: string;
  title: string;
  enabled: boolean;
  position: number;
};

export type Manifest = {
  id: string;
  name: string;
  version: string;
  tasks: Array<{
    id: string;
    title: string;
    input_schema: {
      properties?: Record<string, { type?: string; title?: string; default?: unknown }>;
      required?: string[];
    };
  }>;
  settings_schema: Record<string, unknown>;
  workspace?: {
    type?: string;
    resource?: string;
  };
};

export async function login(username: string, password: string): Promise<string> {
  const response = await fetch(`${API_URL}/api/auth/token`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ username, password }),
  });
  if (!response.ok) throw new Error("로그인에 실패했습니다.");
  return (await response.json()).access_token;
}

export async function api<T>(path: string, token: string, init?: RequestInit): Promise<T> {
  const response = await fetch(`${API_URL}${path}`, {
    ...init,
    headers: {
      "Content-Type": "application/json",
      Authorization: `Bearer ${token}`,
      ...init?.headers,
    },
  });
  if (!response.ok) {
    const error = await response.json().catch(() => ({ detail: response.statusText }));
    throw new Error(error.detail || "요청에 실패했습니다.");
  }
  return response.status === 204 ? (undefined as T) : response.json();
}
