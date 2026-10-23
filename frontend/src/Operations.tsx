// 작업 이력·스케줄·플랫폼 로그 운영 화면을 제공한다.
import { useEffect, useState, type FormEvent } from "react";
import { Ban, Plus, Save, Trash2, Pencil, X } from "lucide-react";
import {
  body,
  id,
  request,
  type List,
  type Run,
  type Schedule,
  type Plugin,
} from "./api";
import {
  date,
  Empty,
  ErrorBox,
  Field,
  JSONView,
  Pagination,
  Panel,
  Reload,
  State,
  useAction,
  useLoad,
  type PageProps,
} from "./common";
import { Button } from "./components/ui/button";
import { Input } from "./components/ui/input";
import { Textarea } from "./components/ui/textarea";

export function RunsPage({
  user,
  notify,
  changed,
  revision = 0,
  compact = false,
}: PageProps & { revision?: number; compact?: boolean }) {
  const [offset, setOffset] = useState(0),
    [selected, setSelected] = useState<string | null>(null);
  const runs = useLoad<List<Run>>(
    `/runs?limit=${compact ? 5 : 50}&offset=${offset}`,
    revision,
  );
  const { busy, perform } = useAction(notify);
  return (
    <Panel
      title={compact ? "최근 작업" : "작업 이력"}
      description={
        compact
          ? "가장 최근에 실행한 작업입니다."
          : "대기·실행·완료 상태를 확인하고 필요하면 취소할 수 있습니다."
      }
      actions={<Reload onClick={runs.reload} loading={runs.loading} />}
    >
      <ErrorBox message={runs.error} />
      <div className="table-scroll">
        <table>
          <thead>
            <tr>
              <th>플러그인 / 작업</th>
              <th>상태</th>
              <th>진행률</th>
              <th>요청 시각</th>
              {!compact && <th>관리</th>}
            </tr>
          </thead>
          <tbody>
            {runs.data?.items.map((run) => (
              <tr key={run.id}>
                <td>
                  <button
                    className="text-link"
                    onClick={() => setSelected(run.id)}
                  >
                    {run.plugin_id}
                  </button>
                  <small>{run.task_id}</small>
                </td>
                <td>
                  <State value={run.state} />
                </td>
                <td>
                  <div className="run-progress">
                    <progress value={run.progress} max={100} />
                    <span>{run.progress}%</span>
                  </div>
                </td>
                <td>{date(run.created_at)}</td>
                {!compact && (
                  <td>
                    <Button
                      variant="outline"
                      size="sm"
                      disabled={
                        busy ||
                        user.role === "viewer" ||
                        !["queued", "running"].includes(run.state)
                      }
                      onClick={() => {
                        if (
                          confirm(
                            "이 작업의 취소를 요청할까요? 이미 처리한 외부 작업은 되돌릴 수 없습니다.",
                          )
                        )
                          void perform(
                            () =>
                              request(`/runs/${id(run.id)}/cancel`, {
                                method: "POST",
                              }),
                            "취소를 요청했습니다.",
                          ).then((ok) => {
                            if (ok) {
                              runs.reload();
                              changed();
                            }
                          });
                      }}
                    >
                      <Ban />
                      취소
                    </Button>
                  </td>
                )}
              </tr>
            ))}
          </tbody>
        </table>
      </div>
      {runs.data?.items.length === 0 && (
        <Empty>아직 실행한 작업이 없습니다.</Empty>
      )}
      {!compact && (
        <Pagination
          offset={offset}
          total={runs.data?.total || 0}
          onChange={setOffset}
        />
      )}{" "}
      {selected && (
        <RunDetail
          runId={selected}
          revision={revision}
          close={() => setSelected(null)}
          user={user}
        />
      )}
    </Panel>
  );
}
function RunDetail({
  runId,
  revision,
  close,
  user,
}: {
  runId: string;
  revision: number;
  close: () => void;
  user: PageProps["user"];
}) {
  const run = useLoad<Run>(`/runs/${id(runId)}`, revision);
  return (
    <section className="edit-panel stack">
      <div className="panel-heading">
        <h3>작업 상세</h3>
        <Button
          variant="ghost"
          size="icon"
          onClick={close}
          aria-label="상세 닫기"
        >
          <X />
        </Button>
      </div>
      <ErrorBox message={run.error} />
      {run.data && (
        <>
          <div className="detail-grid">
            <span>작업 ID</span>
            <code>{run.data.id}</code>
            <span>상태</span>
            <State value={run.data.state} />
            <span>실행 시작</span>
            <span>{date(run.data.started_at)}</span>
            <span>실행 종료</span>
            <span>{date(run.data.finished_at)}</span>
          </div>
          {run.data.error && <ErrorBox message={run.data.error} />}{" "}
          {user.role !== "viewer" && (
            <>
              <h4>입력</h4>
              <JSONView value={run.data.input} />
              <h4>결과</h4>
              <JSONView value={run.data.result} />
              <RunLogs runId={runId} revision={revision} />
            </>
          )}
        </>
      )}
    </section>
  );
}
function RunLogs({ runId, revision }: { runId: string; revision: number }) {
  const logs = useLoad<List<Log>>(`/logs?run_id=${id(runId)}`, revision);
  return (
    <>
      <h4>작업 로그</h4>
      <ErrorBox message={logs.error} />
      <LogLines entries={logs.data?.items || []} />
    </>
  );
}

export function SchedulesPage({ notify, changed }: PageProps) {
  const [offset, setOffset] = useState(0),
    [editing, setEditing] = useState<Schedule | null>(null),
    [open, setOpen] = useState(false),
    [pluginId, setPluginId] = useState("");
  const schedules = useLoad<List<Schedule>>(`/schedules?offset=${offset}`),
    plugins = useLoad<List<Plugin>>("/plugins?limit=200");
  const { busy, perform } = useAction(notify);
  const plugin = plugins.data?.items.find((p) => p.id === pluginId);
  async function save(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    const f = new FormData(e.currentTarget);
    if (
      await perform(() =>
        request(editing ? `/schedules/${id(editing.id)}` : "/schedules", {
          method: editing ? "PUT" : "POST",
          body: body({
            name: f.get("name"),
            plugin_id: pluginId,
            task_id: f.get("task_id"),
            cron: f.get("cron"),
            timezone: f.get("timezone"),
            input: JSON.parse(String(f.get("input"))),
            enabled: f.get("enabled") === "on",
          }),
        }),
      )
    ) {
      setOpen(false);
      schedules.reload();
      changed();
    }
  }
  return (
    <Panel
      title="스케줄 관리"
      description="서버가 중지된 동안 놓친 실행은 누적해서 재실행하지 않습니다."
      actions={
        <Button
          onClick={() => {
            setEditing(null);
            setPluginId(
              plugins.data?.items.find((p) => p.state === "active")?.id || "",
            );
            setOpen(true);
          }}
        >
          <Plus />
          스케줄 추가
        </Button>
      }
    >
      <ErrorBox message={schedules.error || plugins.error} />
      {open && (
        <form
          className="edit-panel stack"
          key={editing?.id || "new"}
          onSubmit={save}
        >
          <div className="panel-heading">
            <h3>{editing ? "스케줄 변경" : "새 스케줄"}</h3>
            <Button
              type="button"
              size="icon"
              variant="ghost"
              onClick={() => setOpen(false)}
              aria-label="편집 닫기"
            >
              <X />
            </Button>
          </div>
          <div className="form-grid">
            <Field label="이름">
              <Input
                name="name"
                required
                maxLength={120}
                defaultValue={editing?.name}
              />
            </Field>
            <Field label="플러그인">
              <select
                required
                value={pluginId}
                onChange={(e) => setPluginId(e.target.value)}
              >
                <option value="">선택</option>
                {plugins.data?.items
                  .filter((p) => p.state === "active")
                  .map((p) => (
                    <option key={p.id} value={p.id}>
                      {p.name}
                    </option>
                  ))}
              </select>
            </Field>
            <Field label="작업">
              <select
                name="task_id"
                key={pluginId}
                required
                defaultValue={
                  editing?.plugin_id === pluginId ? editing.task_id : undefined
                }
              >
                {plugin?.manifest.tasks.map((t) => (
                  <option key={t.id} value={t.id}>
                    {t.title}
                  </option>
                ))}
              </select>
            </Field>
            <Field
              label="Cron"
              hint="분 시 일 월 요일. 예를 들어 0 9 * * * 는 매일 오전 9시입니다."
            >
              <Input
                name="cron"
                required
                defaultValue={editing?.cron || "0 9 * * *"}
              />
            </Field>
            <Field label="시간대">
              <Input
                name="timezone"
                required
                defaultValue={editing?.timezone || "Asia/Seoul"}
              />
            </Field>
            <label className="check">
              <input
                name="enabled"
                type="checkbox"
                defaultChecked={editing?.enabled ?? true}
              />
              활성화
            </label>
          </div>
          <Field label="작업 입력 (JSON)">
            <Textarea
              name="input"
              className="json-input"
              required
              defaultValue={JSON.stringify(editing?.input || {}, null, 2)}
              onChange={(e) => {
                try {
                  const v = JSON.parse(e.target.value);
                  e.target.setCustomValidity(
                    v && typeof v === "object" && !Array.isArray(v)
                      ? ""
                      : "JSON 객체가 필요합니다.",
                  );
                } catch {
                  e.target.setCustomValidity("유효한 JSON이 필요합니다.");
                }
              }}
            />
          </Field>
          <div className="form-actions">
            <Button disabled={busy}>
              <Save />
              저장
            </Button>
          </div>
        </form>
      )}
      <div className="table-scroll">
        <table>
          <thead>
            <tr>
              <th>이름 / 작업</th>
              <th>일정</th>
              <th>다음 실행</th>
              <th>상태</th>
              <th>관리</th>
            </tr>
          </thead>
          <tbody>
            {schedules.data?.items.map((s) => (
              <tr key={s.id}>
                <td>
                  <strong>{s.name}</strong>
                  <small>
                    {s.plugin_id} / {s.task_id}
                  </small>
                </td>
                <td>
                  <code>{s.cron}</code>
                  <small>{s.timezone}</small>
                </td>
                <td>{date(s.next_run_at)}</td>
                <td>
                  <State value={s.enabled ? "active" : "disabled"} />
                </td>
                <td>
                  <div className="actions">
                    <Button
                      variant="outline"
                      size="sm"
                      onClick={() => {
                        setEditing(s);
                        setPluginId(s.plugin_id);
                        setOpen(true);
                      }}
                    >
                      <Pencil />
                      변경
                    </Button>
                    <Button
                      variant="ghost"
                      size="icon-sm"
                      aria-label={`${s.name} 삭제`}
                      disabled={busy}
                      onClick={() => {
                        if (confirm(`${s.name} 스케줄을 삭제할까요?`))
                          void perform(
                            () =>
                              request(`/schedules/${id(s.id)}`, {
                                method: "DELETE",
                              }),
                            "삭제했습니다.",
                          ).then((ok) => {
                            if (ok) schedules.reload();
                          });
                      }}
                    >
                      <Trash2 />
                    </Button>
                  </div>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
      {schedules.data?.items.length === 0 && <Empty />}
      <Pagination
        offset={offset}
        total={schedules.data?.total || 0}
        onChange={setOffset}
      />
    </Panel>
  );
}

type Log = {
  timestamp: string;
  level: string;
  message: string;
  component: string;
  plugin_id?: string;
  task_id?: string;
  run_id?: string;
  user_id?: string;
  request_id?: string;
};
function LogLines({ entries }: { entries: Log[] }) {
  return (
    <div className="log-lines" role="log" aria-label="로그 내용">
      {entries.length === 0 ? (
        <Empty>조건에 맞는 로그가 없습니다.</Empty>
      ) : (
        entries.map((l, i) => (
          <div
            className={`log-line log-${l.level}`}
            key={`${l.timestamp}:${i}`}
          >
            <time>{date(l.timestamp)}</time>
            <b>{l.level}</b>
            <span className="log-source">{l.plugin_id || l.component}</span>
            <span>{l.message}</span>
          </div>
        ))
      )}
    </div>
  );
}
export function LogsPage(_props: PageProps) {
  const [q, setQ] = useState(""),
    [query, setQuery] = useState(""),
    [level, setLevel] = useState(""),
    [auto, setAuto] = useState(true),
    [offset, setOffset] = useState(0),
    [revision, setRevision] = useState(0);
  const logs = useLoad<List<Log>>(
    `/logs?level=${level}&q=${encodeURIComponent(query)}&offset=${offset}`,
    revision,
  );
  useEffect(() => {
    if (!auto) return;
    const timer = setInterval(() => setRevision((n) => n + 1), 5000);
    return () => clearInterval(timer);
  }, [auto]);
  return (
    <Panel
      title="전체 로그"
      description="플랫폼 이벤트와 작업 로그를 확인합니다."
      actions={<Reload onClick={logs.reload} loading={logs.loading} />}
    >
      <form
        className="log-filters"
        onSubmit={(e) => {
          e.preventDefault();
          setQuery(q);
          setOffset(0);
        }}
      >
        <Field label="레벨">
          <select
            value={level}
            onChange={(e) => {
              setLevel(e.target.value);
              setOffset(0);
            }}
          >
            <option value="">모든 레벨</option>
            {["DEBUG", "INFO", "WARN", "ERROR"].map((v) => (
              <option key={v}>{v}</option>
            ))}
          </select>
        </Field>
        <Field label="메시지 검색">
          <Input
            value={q}
            onChange={(e) => setQ(e.target.value)}
            placeholder="검색어"
          />
        </Field>
        <Button variant="outline">검색</Button>
        <label className="check">
          <input
            type="checkbox"
            checked={auto}
            onChange={(e) => setAuto(e.target.checked)}
          />
          자동 갱신
        </label>
      </form>
      <ErrorBox message={logs.error} />
      <LogLines entries={logs.data?.items || []} />
      <Pagination
        offset={offset}
        total={logs.data?.total || 0}
        onChange={setOffset}
      />
    </Panel>
  );
}
