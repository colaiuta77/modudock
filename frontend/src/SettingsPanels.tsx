// 설정 하위의 플러그인·스케줄·계정·로그·메뉴·비밀번호 관리 화면을 제공한다.
import { FormEvent, useCallback, useEffect, useMemo, useState } from "react";
import {
  IconCalendarClock,
  IconKey,
  IconListDetails,
  IconMenu2,
  IconPointFilled,
  IconPlugConnected,
  IconUsers,
  type Icon as TablerIcon,
} from "@tabler/icons-react";
import {
  api,
  LogEntry,
  Manifest,
  Plugin,
  PluginMenu,
  Schedule,
  User,
} from "./api";

type CommonProps = {
  token: string;
  notify: (message: string) => void;
};

export function SettingsHome({
  isAdmin,
  onNavigate,
}: {
  isAdmin: boolean;
  onNavigate: (view: string) => void;
}) {
  const cards: Array<[string, string, string, TablerIcon]> = [
    ["전체 로그", "플랫폼·플러그인·작업 로그를 통합 조회합니다.", "logs", IconListDetails],
    ["비밀번호 변경", "현재 로그인한 계정의 비밀번호를 변경합니다.", "password", IconKey],
  ];
  if (isAdmin) {
    cards.unshift(
      ["플러그인", "Git 플러그인 설치와 활성 상태를 관리합니다.", "plugins", IconPlugConnected],
      ["스케줄", "플러그인 작업의 cron 실행 계획을 편집합니다.", "schedules", IconCalendarClock],
      ["사용자 계정", "관리자와 일반계정을 추가하고 변경합니다.", "users", IconUsers],
      ["메뉴 편집", "플러그인 메뉴의 이름과 노출 순서를 편집합니다.", "menus", IconMenu2],
    );
  }
  return (
    <section className="settings-grid">
      {cards.map(([title, description, destination, CardIcon]) => (
        <button
          className="settings-card"
          key={title}
          onClick={() => onNavigate(`settings/${destination}`)}
        >
          <span className="settings-card-icon"><CardIcon size={22} stroke={1.8} /></span>
          <h2>{title}</h2>
          <p>{description}</p>
          <span className="settings-card-link">관리하기</span>
        </button>
      ))}
    </section>
  );
}

export function PluginsSettings({
  token,
  notify,
  plugins,
  manifests,
  reload,
  openPlugin,
}: CommonProps & {
  plugins: Plugin[];
  manifests: Record<string, Manifest>;
  reload: () => Promise<void>;
  openPlugin: (pluginId: string) => void;
}) {
  const [repository, setRepository] = useState("");

  async function install(event: FormEvent) {
    event.preventDefault();
    try {
      await api("/api/plugins/install", token, {
        method: "POST",
        body: JSON.stringify({ repository_url: repository, ref: "HEAD" }),
      });
      setRepository("");
      notify("플러그인 설치 작업을 시작했습니다.");
      window.setTimeout(() => void reload(), 1500);
    } catch (reason) {
      notify(reason instanceof Error ? reason.message : "설치 요청에 실패했습니다.");
    }
  }

  return (
    <>
      <form className="install-bar" onSubmit={install}>
        <div>
          <p className="eyebrow">INSTALL FROM GIT</p>
          <h2>새 플러그인 연결</h2>
        </div>
        <input
          type="url"
          required
          placeholder="https://github.com/org/plugin.git"
          value={repository}
          onChange={(event) => setRepository(event.target.value)}
        />
        <button className="primary">설치</button>
      </form>
      <section className="panel">
        <div className="panel-title">
          <div>
            <p className="eyebrow">INSTALLED PLUGINS</p>
            <h2>설치된 플러그인</h2>
          </div>
          <span className="count-label">{plugins.length}개</span>
        </div>
        <div className="table-wrap">
          <table>
            <thead>
              <tr>
                <th>이름</th>
                <th>플러그인 ID</th>
                <th>버전</th>
                <th>상태</th>
                <th>저장소</th>
                <th />
              </tr>
            </thead>
            <tbody>
              {plugins.map((plugin) => (
                <tr key={plugin.id}>
                  <td><strong>{plugin.name}</strong></td>
                  <td><code>{plugin.id}</code></td>
                  <td>{manifests[plugin.id]?.version || "-"}</td>
                  <td><span className={`badge ${plugin.state}`}>{plugin.state}</span></td>
                  <td className="truncate-cell">{plugin.repository_url}</td>
                  <td><button className="table-action" onClick={() => openPlugin(plugin.id)}>열기</button></td>
                </tr>
              ))}
            </tbody>
          </table>
          {!plugins.length && <div className="empty">아직 설치된 플러그인이 없습니다.</div>}
        </div>
      </section>
    </>
  );
}

type ScheduleDraft = {
  name: string;
  plugin_id: string;
  task_name: string;
  cron: string;
  timezone: string;
  input: string;
  enabled: boolean;
};

export function SchedulesSettings({
  token,
  notify,
  plugins,
  manifests,
}: CommonProps & { plugins: Plugin[]; manifests: Record<string, Manifest> }) {
  const [schedules, setSchedules] = useState<Schedule[]>([]);
  const [editingId, setEditingId] = useState<string | null>(null);
  const firstPlugin = plugins.find((plugin) => manifests[plugin.id]?.tasks.length);
  const emptyDraft = useMemo<ScheduleDraft>(
    () => ({
      name: "",
      plugin_id: firstPlugin?.id || "",
      task_name: firstPlugin ? manifests[firstPlugin.id]?.tasks[0]?.id || "" : "",
      cron: "0 * * * *",
      timezone: "Asia/Seoul",
      input: "{}",
      enabled: true,
    }),
    [firstPlugin, manifests],
  );
  const [draft, setDraft] = useState<ScheduleDraft>(emptyDraft);

  const load = useCallback(async () => {
    setSchedules(await api<Schedule[]>("/api/schedules", token));
  }, [token]);

  useEffect(() => {
    void load();
  }, [load]);

  useEffect(() => {
    if (!draft.plugin_id && emptyDraft.plugin_id) setDraft(emptyDraft);
  }, [draft.plugin_id, emptyDraft]);

  function selectPlugin(pluginId: string) {
    setDraft({
      ...draft,
      plugin_id: pluginId,
      task_name: manifests[pluginId]?.tasks[0]?.id || "",
    });
  }

  async function submit(event: FormEvent) {
    event.preventDefault();
    try {
      const payload = { ...draft, input: JSON.parse(draft.input) };
      await api(editingId ? `/api/schedules/${editingId}` : "/api/schedules", token, {
        method: editingId ? "PUT" : "POST",
        body: JSON.stringify(payload),
      });
      notify(editingId ? "스케줄을 변경했습니다." : "스케줄을 추가했습니다.");
      setEditingId(null);
      setDraft(emptyDraft);
      await load();
    } catch (reason) {
      notify(reason instanceof Error ? reason.message : "스케줄 저장에 실패했습니다.");
    }
  }

  function edit(schedule: Schedule) {
    setEditingId(schedule.id);
    setDraft({
      name: schedule.name,
      plugin_id: schedule.plugin_id,
      task_name: schedule.task_name,
      cron: schedule.cron,
      timezone: schedule.timezone,
      input: JSON.stringify(schedule.input, null, 2),
      enabled: schedule.enabled,
    });
    window.scrollTo({ top: 0, behavior: "smooth" });
  }

  async function remove(schedule: Schedule) {
    if (!window.confirm(`"${schedule.name}" 스케줄을 삭제할까요?`)) return;
    await api(`/api/schedules/${schedule.id}`, token, { method: "DELETE" });
    notify("스케줄을 삭제했습니다.");
    await load();
  }

  return (
    <>
      <form className="panel form-panel" onSubmit={submit}>
        <div className="panel-title">
          <div>
            <p className="eyebrow">SCHEDULE EDITOR</p>
            <h2>{editingId ? "스케줄 변경" : "새 스케줄"}</h2>
          </div>
        </div>
        <div className="form-grid">
          <label>이름<input required value={draft.name} onChange={(event) => setDraft({ ...draft, name: event.target.value })} /></label>
          <label>플러그인
            <select required value={draft.plugin_id} onChange={(event) => selectPlugin(event.target.value)}>
              <option value="">선택하세요</option>
              {plugins.filter((plugin) => manifests[plugin.id]?.tasks.length).map((plugin) => <option key={plugin.id} value={plugin.id}>{plugin.name}</option>)}
            </select>
          </label>
          <label>작업
            <select required value={draft.task_name} onChange={(event) => setDraft({ ...draft, task_name: event.target.value })}>
              {(manifests[draft.plugin_id]?.tasks || []).map((task) => <option key={task.id} value={task.id}>{task.title}</option>)}
            </select>
          </label>
          <label>Cron 표현식<input required value={draft.cron} onChange={(event) => setDraft({ ...draft, cron: event.target.value })} /></label>
          <label>시간대<input required value={draft.timezone} onChange={(event) => setDraft({ ...draft, timezone: event.target.value })} /></label>
          <label className="check-label"><input type="checkbox" checked={draft.enabled} onChange={(event) => setDraft({ ...draft, enabled: event.target.checked })} /> 활성화</label>
          <label className="full-field">작업 입력 JSON<textarea rows={5} value={draft.input} onChange={(event) => setDraft({ ...draft, input: event.target.value })} /></label>
        </div>
        <div className="form-actions">
          {editingId && <button type="button" className="ghost" onClick={() => { setEditingId(null); setDraft(emptyDraft); }}>취소</button>}
          <button className="primary">저장</button>
        </div>
      </form>
      <section className="panel">
        <div className="panel-title"><div><p className="eyebrow">SCHEDULES</p><h2>등록된 스케줄</h2></div><span className="count-label">{schedules.length}개</span></div>
        <div className="table-wrap">
          <table>
            <thead><tr><th>이름</th><th>작업</th><th>Cron</th><th>시간대</th><th>다음 실행</th><th>상태</th><th /></tr></thead>
            <tbody>{schedules.map((schedule) => (
              <tr key={schedule.id}>
                <td><strong>{schedule.name}</strong></td>
                <td>{schedule.plugin_id} / {schedule.task_name}</td>
                <td><code>{schedule.cron}</code></td>
                <td>{schedule.timezone}</td>
                <td>{new Date(schedule.next_run_at).toLocaleString("ko-KR")}</td>
                <td><span className={`badge ${schedule.enabled ? "active" : "disabled"}`}>{schedule.enabled ? "활성" : "중지"}</span></td>
                <td className="action-cell"><button className="table-action" onClick={() => edit(schedule)}>변경</button><button className="table-action danger" onClick={() => void remove(schedule)}>삭제</button></td>
              </tr>
            ))}</tbody>
          </table>
          {!schedules.length && <div className="empty">등록된 스케줄이 없습니다.</div>}
        </div>
      </section>
    </>
  );
}

export function UsersSettings({ token, notify, currentUser }: CommonProps & { currentUser: User }) {
  const [users, setUsers] = useState<User[]>([]);
  const [drafts, setDrafts] = useState<Record<string, User>>({});
  const [newUser, setNewUser] = useState({ username: "", password: "", role: "viewer" });
  const [resetTarget, setResetTarget] = useState<User | null>(null);
  const [resetPassword, setResetPassword] = useState("");

  const load = useCallback(async () => {
    const data = await api<User[]>("/api/users", token);
    setUsers(data);
    setDrafts(Object.fromEntries(data.map((user) => [user.id, { ...user }])));
  }, [token]);

  useEffect(() => {
    void load();
  }, [load]);

  async function create(event: FormEvent) {
    event.preventDefault();
    try {
      await api("/api/users", token, { method: "POST", body: JSON.stringify(newUser) });
      setNewUser({ username: "", password: "", role: "viewer" });
      notify("사용자 계정을 추가했습니다.");
      await load();
    } catch (reason) {
      notify(reason instanceof Error ? reason.message : "사용자 추가에 실패했습니다.");
    }
  }

  async function save(userId: string) {
    try {
      await api(`/api/users/${userId}`, token, {
        method: "PUT",
        body: JSON.stringify({
          username: drafts[userId].username,
          role: drafts[userId].role,
          is_active: drafts[userId].is_active,
        }),
      });
      notify("사용자 정보를 변경했습니다.");
      await load();
    } catch (reason) {
      notify(reason instanceof Error ? reason.message : "사용자 변경에 실패했습니다.");
    }
  }

  async function remove(user: User) {
    if (!window.confirm(`"${user.username}" 계정을 삭제할까요? 로그인은 즉시 차단됩니다.`)) return;
    try {
      await api(`/api/users/${user.id}`, token, { method: "DELETE" });
      notify("사용자 계정을 비활성화했습니다.");
      await load();
    } catch (reason) {
      notify(reason instanceof Error ? reason.message : "사용자 삭제에 실패했습니다.");
    }
  }

  async function reset(event: FormEvent) {
    event.preventDefault();
    if (!resetTarget) return;
    try {
      await api(`/api/users/${resetTarget.id}/password`, token, {
        method: "PUT",
        body: JSON.stringify({ new_password: resetPassword }),
      });
      notify("사용자 비밀번호를 재설정했습니다.");
      setResetTarget(null);
      setResetPassword("");
    } catch (reason) {
      notify(reason instanceof Error ? reason.message : "비밀번호 재설정에 실패했습니다.");
    }
  }

  return (
    <>
      <form className="panel compact-form" onSubmit={create}>
        <div><p className="eyebrow">ADD ACCOUNT</p><h2>사용자 추가</h2></div>
        <label>사용자 이름<input required minLength={3} value={newUser.username} onChange={(event) => setNewUser({ ...newUser, username: event.target.value })} /></label>
        <label>초기 비밀번호<input required minLength={10} type="password" value={newUser.password} onChange={(event) => setNewUser({ ...newUser, password: event.target.value })} /></label>
        <label>계정 유형
          <select value={newUser.role} onChange={(event) => setNewUser({ ...newUser, role: event.target.value })}>
            <option value="viewer">일반계정</option>
            <option value="admin">관리자</option>
          </select>
        </label>
        <button className="primary">추가</button>
      </form>
      <section className="panel">
        <div className="panel-title"><div><p className="eyebrow">ACCOUNTS</p><h2>사용자 계정 관리</h2></div><span className="count-label">{users.length}명</span></div>
        <div className="table-wrap">
          <table>
            <thead><tr><th>사용자 이름</th><th>계정 유형</th><th>상태</th><th /></tr></thead>
            <tbody>{users.map((user) => {
              const draft = drafts[user.id] || user;
              return (
                <tr key={user.id}>
                  <td><input className="inline-input" value={draft.username} onChange={(event) => setDrafts({ ...drafts, [user.id]: { ...draft, username: event.target.value } })} /></td>
                  <td><select className="inline-input" value={draft.role} onChange={(event) => setDrafts({ ...drafts, [user.id]: { ...draft, role: event.target.value as User["role"] } })}><option value="viewer">일반계정</option><option value="admin">관리자</option>{draft.role === "operator" && <option value="operator">운영자(기존)</option>}</select></td>
                  <td><label className="switch-label"><input type="checkbox" checked={draft.is_active} onChange={(event) => setDrafts({ ...drafts, [user.id]: { ...draft, is_active: event.target.checked } })} /><span>{draft.is_active ? "활성" : "비활성"}</span></label></td>
                  <td className="action-cell"><button className="table-action" onClick={() => void save(user.id)}>저장</button><button className="table-action" onClick={() => setResetTarget(user)}>비밀번호</button><button disabled={user.id === currentUser.id} className="table-action danger" onClick={() => void remove(user)}>삭제</button></td>
                </tr>
              );
            })}</tbody>
          </table>
        </div>
      </section>
      {resetTarget && <div className="modal-backdrop" onClick={() => setResetTarget(null)}>
        <form className="schema-form" onSubmit={reset} onClick={(event) => event.stopPropagation()}>
          <p className="eyebrow">RESET PASSWORD</p>
          <h2>{resetTarget.username} 비밀번호 재설정</h2>
          <label>새 비밀번호<input required minLength={10} type="password" value={resetPassword} onChange={(event) => setResetPassword(event.target.value)} /></label>
          <div className="form-actions"><button type="button" className="ghost" onClick={() => setResetTarget(null)}>취소</button><button className="primary">재설정</button></div>
        </form>
      </div>}
    </>
  );
}

export function LogsSettings({ token, notify }: CommonProps) {
  const [logs, setLogs] = useState<LogEntry[]>([]);
  const [level, setLevel] = useState("");
  const [query, setQuery] = useState("");
  const [autoRefresh, setAutoRefresh] = useState(true);

  const load = useCallback(async () => {
    try {
      const params = new URLSearchParams({ limit: "300" });
      if (level) params.set("level", level);
      if (query) params.set("query", query);
      setLogs(await api<LogEntry[]>(`/api/logs?${params}`, token));
    } catch (reason) {
      notify(reason instanceof Error ? reason.message : "로그 조회에 실패했습니다.");
    }
  }, [level, notify, query, token]);

  useEffect(() => {
    void load();
  }, [load]);

  useEffect(() => {
    if (!autoRefresh) return;
    const timer = window.setInterval(() => void load(), 5000);
    return () => window.clearInterval(timer);
  }, [autoRefresh, load]);

  return (
    <section className="panel log-panel">
      <div className="panel-title"><div><p className="eyebrow">SYSTEM LOG STREAM</p><h2>전체 로그</h2></div><span className="live-indicator"><IconPointFilled size={16} /> {autoRefresh ? "LIVE" : "PAUSED"}</span></div>
      <div className="filter-bar">
        <select value={level} onChange={(event) => setLevel(event.target.value)}><option value="">모든 레벨</option><option value="INFO">INFO</option><option value="WARNING">WARNING</option><option value="ERROR">ERROR</option></select>
        <input placeholder="메시지 검색" value={query} onChange={(event) => setQuery(event.target.value)} />
        <label className="check-label"><input type="checkbox" checked={autoRefresh} onChange={(event) => setAutoRefresh(event.target.checked)} /> 자동 갱신</label>
        <button className="ghost" onClick={() => void load()}>새로고침</button>
      </div>
      <div className="log-viewer">
        {logs.map((entry) => (
          <div className={`log-line ${entry.level.toLowerCase()}`} key={entry.id}>
            <time>{new Date(entry.timestamp).toLocaleString("ko-KR")}</time>
            <b>{entry.level}</b>
            <span className="log-source">
              {entry.plugin_id || (
                typeof entry.context.component === "string"
                  ? `MODUDOCK/${entry.context.component.toUpperCase()}`
                  : "MODUDOCK"
              )}
              {entry.task_id ? ` / ${entry.task_id}` : ""}
            </span>
            <span>{entry.message}</span>
            {entry.run_id && <code>{entry.run_id.slice(0, 8)}</code>}
          </div>
        ))}
        {!logs.length && <div className="empty">조건에 맞는 로그가 없습니다.</div>}
      </div>
    </section>
  );
}

export function MenuSettings({
  token,
  notify,
  menus,
  reload,
}: CommonProps & { menus: PluginMenu[]; reload: () => Promise<void> }) {
  const [drafts, setDrafts] = useState<Record<string, PluginMenu>>({});

  useEffect(() => {
    setDrafts(Object.fromEntries(menus.map((menu) => [`${menu.plugin_id}/${menu.menu_id}`, { ...menu }])));
  }, [menus]);

  async function save(menu: PluginMenu) {
    const key = `${menu.plugin_id}/${menu.menu_id}`;
    const draft = drafts[key];
    try {
      await api(`/api/plugin-menus/${menu.plugin_id}/${menu.menu_id}`, token, {
        method: "PUT",
        body: JSON.stringify({
          title: draft.title,
          enabled: draft.enabled,
          position: draft.position,
        }),
      });
      notify("플러그인 메뉴 설정을 저장했습니다.");
      await reload();
    } catch (reason) {
      notify(reason instanceof Error ? reason.message : "메뉴 저장에 실패했습니다.");
    }
  }

  return (
    <section className="panel">
      <div className="panel-title"><div><p className="eyebrow">PLUGIN NAVIGATION</p><h2>플러그인 메뉴 편집</h2></div><span className="count-label">{menus.length}개</span></div>
      <p className="section-help">플러그인 manifest가 선언한 메뉴의 표시 이름, 노출 여부와 사이드바 순서를 변경합니다.</p>
      <div className="table-wrap">
        <table>
          <thead><tr><th>플러그인</th><th>메뉴 ID</th><th>표시 이름</th><th>순서</th><th>노출</th><th /></tr></thead>
          <tbody>{menus.map((menu) => {
            const key = `${menu.plugin_id}/${menu.menu_id}`;
            const draft = drafts[key] || menu;
            return (
              <tr key={key}>
                <td><strong>{menu.plugin_name}</strong></td>
                <td><code>{menu.menu_id}</code></td>
                <td><input className="inline-input" value={draft.title} onChange={(event) => setDrafts({ ...drafts, [key]: { ...draft, title: event.target.value } })} /></td>
                <td><input className="inline-input order-input" type="number" min={0} value={draft.position} onChange={(event) => setDrafts({ ...drafts, [key]: { ...draft, position: Number(event.target.value) } })} /></td>
                <td><label className="switch-label"><input type="checkbox" checked={draft.enabled} onChange={(event) => setDrafts({ ...drafts, [key]: { ...draft, enabled: event.target.checked } })} /><span>{draft.enabled ? "표시" : "숨김"}</span></label></td>
                <td><button className="table-action" onClick={() => void save(menu)}>저장</button></td>
              </tr>
            );
          })}</tbody>
        </table>
        {!menus.length && <div className="empty">메뉴를 선언한 활성 플러그인이 없습니다.</div>}
      </div>
    </section>
  );
}

export function PasswordSettings({ token, notify }: CommonProps) {
  const [currentPassword, setCurrentPassword] = useState("");
  const [newPassword, setNewPassword] = useState("");
  const [confirmation, setConfirmation] = useState("");

  async function submit(event: FormEvent) {
    event.preventDefault();
    if (newPassword !== confirmation) {
      notify("새 비밀번호 확인이 일치하지 않습니다.");
      return;
    }
    try {
      await api("/api/auth/password", token, {
        method: "PUT",
        body: JSON.stringify({ current_password: currentPassword, new_password: newPassword }),
      });
      setCurrentPassword("");
      setNewPassword("");
      setConfirmation("");
      notify("비밀번호를 변경했습니다.");
    } catch (reason) {
      notify(reason instanceof Error ? reason.message : "비밀번호 변경에 실패했습니다.");
    }
  }

  return (
    <form className="panel password-panel" onSubmit={submit}>
      <p className="eyebrow">ACCOUNT SECURITY</p>
      <h2>내 비밀번호 변경</h2>
      <p className="section-help">기본 관리자 비밀번호를 사용 중이라면 즉시 변경하세요.</p>
      <label>현재 비밀번호<input required type="password" value={currentPassword} onChange={(event) => setCurrentPassword(event.target.value)} /></label>
      <label>새 비밀번호<input required minLength={10} type="password" value={newPassword} onChange={(event) => setNewPassword(event.target.value)} /></label>
      <label>새 비밀번호 확인<input required minLength={10} type="password" value={confirmation} onChange={(event) => setConfirmation(event.target.value)} /></label>
      <div className="form-actions"><button className="primary">비밀번호 변경</button></div>
    </form>
  );
}
