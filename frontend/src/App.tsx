// ModuDock의 인증, 내비게이션, 작업 실행 화면을 구성하는 루트 컴포넌트
import { FormEvent, useCallback, useEffect, useMemo, useState } from "react";
import {
  IconActivity,
  IconAnchor,
  IconApps,
  IconCalendarClock,
  IconChevronDown,
  IconChevronRight,
  IconCircleCheck,
  IconClock,
  IconHistory,
  IconKey,
  IconLayoutDashboard,
  IconListDetails,
  IconLoader2,
  IconLogout,
  IconMenu2,
  IconPlayerPlay,
  IconPlugConnected,
  IconPuzzle,
  IconRefresh,
  IconSettings,
  IconUsers,
} from "@tabler/icons-react";

import {
  api,
  login,
  Manifest,
  Plugin,
  PluginMenu,
  TaskRun,
  User,
} from "./api";
import {
  LogsSettings,
  MenuSettings,
  PasswordSettings,
  PluginsSettings,
  SchedulesSettings,
  SettingsHome,
  UsersSettings,
} from "./SettingsPanels";
import { LocalMartSaleWorkspace } from "./LocalMartSaleWorkspace";

type SettingsView =
  | "settings"
  | "settings/plugins"
  | "settings/schedules"
  | "settings/users"
  | "settings/logs"
  | "settings/menus"
  | "settings/password";
type View = "overview" | "runs" | SettingsView | `plugin:${string}`;

const VIEW_TITLES: Record<Exclude<View, `plugin:${string}`>, string> = {
  overview: "개요",
  runs: "작업 이력",
  settings: "설정",
  "settings/plugins": "플러그인",
  "settings/schedules": "스케줄",
  "settings/users": "사용자 계정",
  "settings/logs": "전체 로그",
  "settings/menus": "메뉴 편집",
  "settings/password": "비밀번호 변경",
};

function App() {
  const [token, updateToken] = useState(() => sessionStorage.getItem("modudock-token") || "");
  const [view, setView] = useState<View>("overview");
  const [currentUser, setCurrentUser] = useState<User | null>(null);
  const [plugins, setPlugins] = useState<Plugin[]>([]);
  const [runs, setRuns] = useState<TaskRun[]>([]);
  const [pluginMenus, setPluginMenus] = useState<PluginMenu[]>([]);
  const [manifests, setManifests] = useState<Record<string, Manifest>>({});
  const [notice, setNotice] = useState("");
  const [loading, setLoading] = useState(false);

  const notify = useCallback((message: string) => {
    setNotice(message);
  }, []);

  const logout = useCallback(() => {
    sessionStorage.removeItem("modudock-token");
    updateToken("");
    setCurrentUser(null);
  }, []);

  const refresh = useCallback(async () => {
    if (!token) {
      return;
    }
    setLoading(true);
    try {
      const [me, pluginData, runData, menuData] = await Promise.all([
        api<User>("/api/auth/me", token),
        api<Plugin[]>("/api/plugins", token),
        api<TaskRun[]>("/api/runs", token),
        api<PluginMenu[]>("/api/plugin-menus", token),
      ]);
      setCurrentUser(me);
      setPlugins(pluginData);
      setRuns(runData);
      setPluginMenus(menuData);

      const manifestEntries = await Promise.all(
        pluginData
          .filter((plugin) => plugin.state === "active")
          .map(async (plugin) => {
            try {
              return [
                plugin.id,
                await api<Manifest>(`/api/plugins/${plugin.id}/manifest`, token),
              ] as const;
            } catch {
              return null;
            }
          }),
      );
      setManifests(
        Object.fromEntries(
          manifestEntries.filter(
            (entry): entry is readonly [string, Manifest] => entry !== null,
          ),
        ),
      );
    } catch (error) {
      notify(error instanceof Error ? error.message : "데이터를 불러오지 못했습니다.");
    } finally {
      setLoading(false);
    }
  }, [notify, token]);

  useEffect(() => {
    if (!token) {
      return;
    }
    void refresh();
    const timer = window.setInterval(() => void refresh(), 5000);
    return () => window.clearInterval(timer);
  }, [refresh, token]);

  if (!token) {
    return (
      <Login
        onLogin={(newToken) => {
          sessionStorage.setItem("modudock-token", newToken);
          updateToken(newToken);
        }}
      />
    );
  }

  if (!currentUser) {
    return <div className="app-loading" data-bs-theme="light">ModuDock를 불러오는 중입니다.</div>;
  }

  const isAdmin = currentUser.role === "admin";
  const selectedPluginId = view.startsWith("plugin:") ? view.slice(7) : null;
  const selectedPlugin = plugins.find((plugin) => plugin.id === selectedPluginId);
  const selectedMenu = pluginMenus.find((menu) => menu.plugin_id === selectedPluginId);
  const pageTitle = selectedPluginId
    ? selectedMenu?.title ?? selectedPlugin?.name ?? selectedPluginId
    : VIEW_TITLES[view as Exclude<View, `plugin:${string}`>];
  const settingsOpen = view.startsWith("settings");
  const visibleMenus = pluginMenus
    .filter((menu) => menu.enabled)
    .sort((left, right) => left.position - right.position || left.title.localeCompare(right.title));

  return (
    <div className="shell" data-bs-theme="light">
      <aside>
        <div className="brand">
          <span><IconAnchor size={22} stroke={2.2} /></span>
          <div>
            <strong>ModuDock</strong>
            <small>Automation platform</small>
          </div>
        </div>
        <nav>
          <NavButton
            active={view === "overview"}
            icon={<IconLayoutDashboard />}
            onClick={() => setView("overview")}
          >
            개요
          </NavButton>
          <NavButton
            active={view === "runs"}
            icon={<IconHistory />}
            onClick={() => setView("runs")}
          >
            작업 이력
          </NavButton>

          {visibleMenus.length > 0 && <p className="nav-caption">플러그인</p>}
          {visibleMenus.map((menu) => (
            <NavButton
              key={`${menu.plugin_id}:${menu.menu_id}`}
              active={selectedPluginId === menu.plugin_id}
              className="plugin-nav"
              icon={<IconPuzzle />}
              onClick={() => setView(`plugin:${menu.plugin_id}`)}
            >
              {menu.title}
            </NavButton>
          ))}

          <div className={`nav-group ${settingsOpen ? "open" : ""}`}>
            <NavButton
              active={view === "settings"}
              className="nav-parent"
              icon={<IconSettings />}
              suffix={settingsOpen ? <IconChevronDown /> : <IconChevronRight />}
              onClick={() => setView("settings")}
            >
              설정
            </NavButton>
            {settingsOpen && (
              <div className="subnav">
                {isAdmin && (
                  <>
                    <NavButton
                      active={view === "settings/plugins"}
                      icon={<IconPlugConnected />}
                      onClick={() => setView("settings/plugins")}
                    >
                      플러그인
                    </NavButton>
                    <NavButton
                      active={view === "settings/schedules"}
                      icon={<IconCalendarClock />}
                      onClick={() => setView("settings/schedules")}
                    >
                      스케줄
                    </NavButton>
                    <NavButton
                      active={view === "settings/users"}
                      icon={<IconUsers />}
                      onClick={() => setView("settings/users")}
                    >
                      사용자 계정
                    </NavButton>
                    <NavButton
                      active={view === "settings/menus"}
                      icon={<IconMenu2 />}
                      onClick={() => setView("settings/menus")}
                    >
                      메뉴 편집
                    </NavButton>
                  </>
                )}
                <NavButton
                  active={view === "settings/logs"}
                  icon={<IconListDetails />}
                  onClick={() => setView("settings/logs")}
                >
                  전체 로그
                </NavButton>
                <NavButton
                  active={view === "settings/password"}
                  icon={<IconKey />}
                  onClick={() => setView("settings/password")}
                >
                  비밀번호 변경
                </NavButton>
              </div>
            )}
          </div>
        </nav>
        <div className="aside-footer">
          <div className="system-state">
            <IconCircleCheck size={17} />
            시스템 정상
          </div>
          <div className="account-summary">
            <span>{currentUser.username}</span>
            <small>{isAdmin ? "관리자" : "일반 계정"}</small>
          </div>
          <button className="logout" onClick={logout}>
            <IconLogout size={18} />
            로그아웃
          </button>
        </div>
      </aside>

      <main>
        <header>
          <div>
            <p className="eyebrow">MODUDOCK / {pageTitle.toUpperCase()}</p>
            <h1>{pageTitle}</h1>
          </div>
          <button className="ghost" disabled={loading} onClick={() => void refresh()}>
            {loading ? <IconLoader2 className="spin" size={18} /> : <IconRefresh size={18} />}
            {loading ? "새로고침 중" : "새로고침"}
          </button>
        </header>

        {notice && (
          <div className="notice" onClick={() => setNotice("")}>
            {notice}
          </div>
        )}

        {view === "overview" && (
          <Overview plugins={plugins} runs={runs} pluginMenus={visibleMenus} />
        )}
        {view === "runs" && <RunTable runs={runs} />}
        {view === "settings" && (
          <SettingsHome isAdmin={isAdmin} onNavigate={(next) => setView(next as View)} />
        )}
        {view === "settings/plugins" && isAdmin && (
          <PluginsSettings
            token={token}
            manifests={manifests}
            plugins={plugins}
            reload={refresh}
            notify={notify}
            openPlugin={(pluginId) => setView(`plugin:${pluginId}`)}
          />
        )}
        {view === "settings/schedules" && isAdmin && (
          <SchedulesSettings
            token={token}
            plugins={plugins}
            manifests={manifests}
            notify={notify}
          />
        )}
        {view === "settings/users" && isAdmin && (
          <UsersSettings token={token} currentUser={currentUser} notify={notify} />
        )}
        {view === "settings/menus" && isAdmin && (
          <MenuSettings
            token={token}
            menus={pluginMenus}
            reload={refresh}
            notify={notify}
          />
        )}
        {view === "settings/logs" && <LogsSettings token={token} notify={notify} />}
        {view === "settings/password" && <PasswordSettings token={token} notify={notify} />}
        {selectedPlugin && (
          <PluginWorkspace
            plugin={selectedPlugin}
            manifest={manifests[selectedPlugin.id]}
            token={token}
            canManage={currentUser.role !== "viewer"}
            notify={notify}
            onRun={async (taskName, input) => {
              try {
                await api(`/api/plugins/${selectedPlugin.id}/tasks/${taskName}/runs`, token, {
                  method: "POST",
                  body: JSON.stringify({ input }),
                });
                notify("작업을 요청했습니다.");
                await refresh();
              } catch (error) {
                notify(error instanceof Error ? error.message : "작업 요청에 실패했습니다.");
              }
            }}
          />
        )}
        {selectedPluginId && !selectedPlugin && (
          <EmptyState>선택한 플러그인을 찾을 수 없습니다.</EmptyState>
        )}
      </main>
    </div>
  );
}

function NavButton({
  active,
  children,
  className = "",
  icon,
  onClick,
  suffix,
}: {
  active: boolean;
  children: React.ReactNode;
  className?: string;
  icon?: React.ReactNode;
  onClick: () => void;
  suffix?: React.ReactNode;
}) {
  return (
    <button className={`${active ? "active" : ""} ${className}`} onClick={onClick}>
      {icon && <span className="nav-icon">{icon}</span>}
      <span>{children}</span>
      {suffix && <span className="nav-suffix">{suffix}</span>}
    </button>
  );
}

function Login({ onLogin }: { onLogin: (token: string) => void }) {
  const [username, setUsername] = useState("admin");
  const [password, setPassword] = useState("");
  const [error, setError] = useState("");

  async function submit(event: FormEvent) {
    event.preventDefault();
    try {
      onLogin(await login(username, password));
    } catch (caught) {
      setError(caught instanceof Error ? caught.message : "로그인에 실패했습니다.");
    }
  }

  return (
    <div className="login-page" data-bs-theme="light">
      <form className="login-card" onSubmit={submit}>
        <div className="brand login-brand">
          <span><IconAnchor size={22} stroke={2.2} /></span>
          <div>
            <strong>ModuDock</strong>
            <small>Automation platform</small>
          </div>
        </div>
        <p>플러그인 자동화 플랫폼</p>
        <label>
          사용자 이름
          <input value={username} onChange={(event) => setUsername(event.target.value)} />
        </label>
        <label>
          비밀번호
          <input
            type="password"
            value={password}
            onChange={(event) => setPassword(event.target.value)}
          />
        </label>
        {error && <div className="error">{error}</div>}
        <button type="submit">로그인</button>
      </form>
    </div>
  );
}

function Overview({
  plugins,
  runs,
  pluginMenus,
}: {
  plugins: Plugin[];
  runs: TaskRun[];
  pluginMenus: PluginMenu[];
}) {
  const running = runs.filter((run) => ["queued", "running"].includes(run.state)).length;
  return (
    <>
      <section className="stats">
        <article>
          <div className="stat-heading"><span>활성 플러그인</span><i><IconApps /></i></div>
          <strong>{plugins.filter((plugin) => plugin.state === "active").length}</strong>
          <small>연결된 자동화 모듈</small>
        </article>
        <article>
          <div className="stat-heading"><span>표시 메뉴</span><i><IconMenu2 /></i></div>
          <strong>{pluginMenus.length}</strong>
          <small>사이드바 활성 메뉴</small>
        </article>
        <article>
          <div className="stat-heading"><span>진행 중 작업</span><i><IconActivity /></i></div>
          <strong>{running}</strong>
          <small>대기 및 실행 중</small>
        </article>
        <article>
          <div className="stat-heading"><span>최근 실행</span><i><IconClock /></i></div>
          <strong>{runs.length}</strong>
          <small>기록된 작업 이력</small>
        </article>
      </section>
      <section className="panel">
        <div className="panel-heading">
          <div>
            <p className="eyebrow">RECENT ACTIVITY</p>
            <h2>최근 작업</h2>
          </div>
        </div>
        <RunTable runs={runs.slice(0, 8)} />
      </section>
    </>
  );
}

function PluginWorkspace({
  plugin,
  manifest,
  token,
  canManage,
  notify,
  onRun,
}: {
  plugin: Plugin;
  manifest?: Manifest;
  token: string;
  canManage: boolean;
  notify: (message: string) => void;
  onRun: (taskName: string, input: Record<string, unknown>) => Promise<void>;
}) {
  return (
    <section className="panel">
      <div className="panel-heading">
        <div>
          <p className="eyebrow">PLUGIN WORKSPACE</p>
          <h2>{plugin.name}</h2>
        </div>
        <span className={`badge ${plugin.state}`}>{plugin.state}</span>
      </div>
      {manifest?.workspace?.type === "daily-sale-manager" ? (
        <LocalMartSaleWorkspace
          token={token}
          canManage={canManage}
          resource={manifest.workspace.resource}
          notify={notify}
        />
      ) : !manifest?.tasks?.length ? (
        <EmptyState>이 플러그인에는 실행 가능한 작업이 선언되지 않았습니다.</EmptyState>
      ) : (
        <div className="task-grid">
          {manifest.tasks.map((task) => (
            <TaskLauncher key={task.id} task={task} onRun={onRun} />
          ))}
        </div>
      )}
    </section>
  );
}

function TaskLauncher({
  task,
  onRun,
}: {
  task: Manifest["tasks"][number];
  onRun: (taskName: string, input: Record<string, unknown>) => Promise<void>;
}) {
  const [input, setInput] = useState("{}");
  const [submitting, setSubmitting] = useState(false);
  return (
    <article className="task-card">
      <div>
        <span className="eyebrow">TASK</span>
        <h3>{task.title ?? task.id}</h3>
        <p>{task.id}</p>
      </div>
      <textarea
        rows={5}
        value={input}
        onChange={(event) => setInput(event.target.value)}
        aria-label={`${task.id} 입력 JSON`}
      />
      <button
        disabled={submitting}
        onClick={async () => {
          try {
            setSubmitting(true);
            await onRun(task.id, JSON.parse(input));
          } finally {
            setSubmitting(false);
          }
        }}
      >
        <IconPlayerPlay size={17} />
        {submitting ? "요청 중" : "실행"}
      </button>
    </article>
  );
}

function RunTable({ runs }: { runs: TaskRun[] }) {
  if (!runs.length) {
    return <EmptyState>아직 실행 이력이 없습니다.</EmptyState>;
  }
  return (
    <div className="table-wrap">
      <table>
        <thead>
          <tr>
            <th>플러그인</th>
            <th>작업</th>
            <th>상태</th>
            <th>진행률</th>
            <th>실행 시각</th>
          </tr>
        </thead>
        <tbody>
          {runs.map((run) => (
            <tr key={run.id}>
              <td>{run.plugin_id}</td>
              <td>{run.task_name}</td>
              <td>
                <span className={`badge ${run.state}`}>{run.state}</span>
              </td>
              <td>{run.progress}%</td>
              <td>{new Date(run.created_at).toLocaleString("ko-KR")}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

function EmptyState({ children }: { children: React.ReactNode }) {
  return <div className="empty">{children}</div>;
}

export default App;
