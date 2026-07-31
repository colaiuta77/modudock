// 플러그인, 작업, 스케줄 상태를 한 화면에서 관리하는 콘솔이다.
import { FormEvent, useCallback, useEffect, useState } from "react";
import { api, login, Manifest, Plugin, TaskRun } from "./api";

type View = "overview" | "plugins" | "runs" | "schedules";

function Login({ onLogin }: { onLogin: (token: string) => void }) {
  const [username, setUsername] = useState("admin");
  const [password, setPassword] = useState("");
  const [error, setError] = useState("");

  async function submit(event: FormEvent) {
    event.preventDefault();
    try {
      onLogin(await login(username, password));
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : "로그인에 실패했습니다.");
    }
  }

  return (
    <main className="login-shell">
      <section className="login-card">
        <div className="brand-mark">M</div>
        <p className="eyebrow">AUTOMATION CONTROL PLANE</p>
        <h1>모든 작업이<br />머무는 도크.</h1>
        <p className="muted">플러그인을 연결하고, 예약하고, 실행 이력을 한곳에서 관리하세요.</p>
        <form onSubmit={submit}>
          <label>사용자 이름<input value={username} onChange={(event) => setUsername(event.target.value)} /></label>
          <label>비밀번호<input type="password" value={password} onChange={(event) => setPassword(event.target.value)} /></label>
          {error && <p className="error">{error}</p>}
          <button className="primary" type="submit">콘솔 열기</button>
        </form>
      </section>
    </main>
  );
}

function App() {
  const [token, setToken] = useState(() => sessionStorage.getItem("modudock-token") || "");
  const [view, setView] = useState<View>("overview");
  const [plugins, setPlugins] = useState<Plugin[]>([]);
  const [runs, setRuns] = useState<TaskRun[]>([]);
  const [manifests, setManifests] = useState<Record<string, Manifest>>({});
  const [repository, setRepository] = useState("");
  const [notice, setNotice] = useState("");

  const refresh = useCallback(async () => {
    if (!token) return;
    try {
      const [pluginData, runData] = await Promise.all([
        api<Plugin[]>("/api/plugins", token),
        api<TaskRun[]>("/api/runs", token),
      ]);
      setPlugins(pluginData);
      setRuns(runData);
      const manifestPairs = await Promise.all(
        pluginData.filter((plugin) => plugin.state === "active").map(async (plugin) => [
          plugin.id,
          await api<Manifest>(`/api/plugins/${plugin.id}/manifest`, token),
        ] as const),
      );
      setManifests(Object.fromEntries(manifestPairs));
    } catch (reason) {
      setNotice(reason instanceof Error ? reason.message : "데이터를 불러오지 못했습니다.");
    }
  }, [token]);

  useEffect(() => {
    void refresh();
    const timer = window.setInterval(() => void refresh(), 5000);
    return () => window.clearInterval(timer);
  }, [refresh]);

  function acceptToken(value: string) {
    sessionStorage.setItem("modudock-token", value);
    setToken(value);
  }

  async function install(event: FormEvent) {
    event.preventDefault();
    await api("/api/plugins/install", token, {
      method: "POST",
      body: JSON.stringify({ repository_url: repository, ref: "HEAD" }),
    });
    setRepository("");
    setNotice("설치 작업을 시작했습니다. 잠시 후 목록이 갱신됩니다.");
  }

  async function runTask(pluginId: string, taskId: string, input: Record<string, unknown>) {
    await api(`/api/plugins/${pluginId}/tasks/${taskId}/runs`, token, {
      method: "POST",
      body: JSON.stringify({ input }),
    });
    setNotice(`${taskId} 작업을 대기열에 추가했습니다.`);
    await refresh();
  }

  if (!token) return <Login onLogin={acceptToken} />;
  const activeCount = plugins.filter((plugin) => plugin.state === "active").length;
  const runningCount = runs.filter((run) => ["queued", "running", "cancel_requested"].includes(run.state)).length;
  const successful = runs.filter((run) => run.state === "succeeded").length;
  const successRate = runs.length ? Math.round((successful / runs.length) * 100) : 100;

  return (
    <div className="app-shell">
      <aside>
        <div className="brand"><span className="brand-mark small">M</span><strong>ModuDock</strong></div>
        <nav>
          {([
            ["overview", "개요"],
            ["plugins", "플러그인"],
            ["runs", "작업 이력"],
            ["schedules", "스케줄"],
          ] as Array<[View, string]>).map(([id, label]) => (
            <button className={view === id ? "active" : ""} onClick={() => setView(id)} key={id}>{label}</button>
          ))}
        </nav>
        <div className="system-state"><span className="status-dot" /> 시스템 정상</div>
        <button className="logout" onClick={() => { sessionStorage.clear(); setToken(""); }}>로그아웃</button>
      </aside>
      <main className="content">
        <header>
          <div><p className="eyebrow">MODUDOCK / {view.toUpperCase()}</p><h1>{view === "overview" ? "오늘의 자동화" : { plugins: "플러그인", runs: "작업 이력", schedules: "스케줄" }[view]}</h1></div>
          <button className="ghost" onClick={() => void refresh()}>새로고침</button>
        </header>
        {notice && <div className="notice" onClick={() => setNotice("")}>{notice}</div>}

        {view === "overview" && (
          <>
            <section className="metric-grid">
              <article><span>활성 플러그인</span><strong>{activeCount}</strong><small>연결됨</small></article>
              <article><span>진행 중</span><strong>{runningCount}</strong><small>작업</small></article>
              <article><span>성공률</span><strong>{successRate}%</strong><small>최근 실행</small></article>
            </section>
            <section className="panel">
              <div className="panel-title"><div><p className="eyebrow">RECENT ACTIVITY</p><h2>최근 작업</h2></div><button className="link" onClick={() => setView("runs")}>전체 보기 →</button></div>
              <RunTable runs={runs.slice(0, 6)} />
            </section>
          </>
        )}

        {view === "plugins" && (
          <>
            <form className="install-bar" onSubmit={install}>
              <div><p className="eyebrow">INSTALL FROM GIT</p><h2>새 플러그인 연결</h2></div>
              <input type="url" required placeholder="https://github.com/org/plugin.git" value={repository} onChange={(event) => setRepository(event.target.value)} />
              <button className="primary">설치</button>
            </form>
            <section className="plugin-grid">
              {plugins.map((plugin) => (
                <article className="plugin-card" key={plugin.id}>
                  <div className="plugin-icon">{plugin.name.slice(0, 1)}</div>
                  <div><span className={`badge ${plugin.state}`}>{plugin.state}</span><h2>{plugin.name}</h2><code>{plugin.id}</code></div>
                  <div className="task-list">
                    {(manifests[plugin.id]?.tasks || []).map((task) => (
                      <TaskLauncher
                        key={task.id}
                        task={task}
                        onRun={(input) => runTask(plugin.id, task.id, input)}
                      />
                    ))}
                  </div>
                </article>
              ))}
              {!plugins.length && <div className="empty">아직 설치된 플러그인이 없습니다.</div>}
            </section>
          </>
        )}

        {view === "runs" && <section className="panel"><RunTable runs={runs} /></section>}
        {view === "schedules" && (
          <section className="panel empty-state"><div className="clock">◷</div><h2>스케줄 API가 준비되었습니다.</h2><p>플러그인 작업과 cron 표현식을 API에서 등록할 수 있습니다.<br />관리 폼은 다음 UI 반복에서 연결합니다.</p></section>
        )}
      </main>
    </div>
  );
}

function TaskLauncher({
  task,
  onRun,
}: {
  task: Manifest["tasks"][number];
  onRun: (input: Record<string, unknown>) => Promise<void>;
}) {
  const [open, setOpen] = useState(false);
  const [values, setValues] = useState<Record<string, unknown>>(() =>
    Object.fromEntries(
      Object.entries(task.input_schema.properties || {}).map(([key, property]) => [key, property.default ?? ""]),
    ),
  );

  async function submit(event: FormEvent) {
    event.preventDefault();
    await onRun(values);
    setOpen(false);
  }

  return (
    <>
      <button onClick={() => setOpen(true)}>{task.title}<span>실행 →</span></button>
      {open && <div className="modal-backdrop" onClick={() => setOpen(false)}>
        <form className="schema-form" onSubmit={submit} onClick={(event) => event.stopPropagation()}>
          <p className="eyebrow">SCHEMA GENERATED FORM</p>
          <h2>{task.title}</h2>
          {Object.entries(task.input_schema.properties || {}).map(([key, property]) => (
            <label key={key}>{property.title || key}
              <input
                type={property.type === "number" || property.type === "integer" ? "number" : "text"}
                required={task.input_schema.required?.includes(key)}
                value={String(values[key] ?? "")}
                onChange={(event) => setValues({
                  ...values,
                  [key]: property.type === "number" || property.type === "integer"
                    ? Number(event.target.value)
                    : event.target.value,
                })}
              />
            </label>
          ))}
          {!Object.keys(task.input_schema.properties || {}).length && <p className="muted">입력값이 필요하지 않습니다.</p>}
          <div className="form-actions"><button type="button" className="ghost" onClick={() => setOpen(false)}>취소</button><button className="primary">작업 실행</button></div>
        </form>
      </div>}
    </>
  );
}

function RunTable({ runs }: { runs: TaskRun[] }) {
  return (
    <div className="table-wrap"><table><thead><tr><th>작업</th><th>플러그인</th><th>상태</th><th>진행률</th><th>시작 시각</th></tr></thead>
      <tbody>{runs.map((run) => <tr key={run.id}><td><strong>{run.task_name}</strong><small>{run.id.slice(0, 8)}</small></td><td>{run.plugin_id || "platform"}</td><td><span className={`badge ${run.state}`}>{run.state}</span></td><td><div className="progress"><i style={{ width: `${run.progress}%` }} /></div></td><td>{new Date(run.created_at).toLocaleString("ko-KR")}</td></tr>)}</tbody>
    </table>{!runs.length && <div className="empty">실행 이력이 없습니다.</div>}</div>
  );
}

export default App;
