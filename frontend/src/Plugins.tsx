// GitHub 플러그인 설치와 선언형 플러그인 작업 공간을 제공한다.
import { useState } from "react";
import {
  Download,
  ExternalLink,
  Power,
  RotateCcw,
  Settings,
  X,
} from "lucide-react";
import {
  body,
  id,
  request,
  type List,
  type Plugin,
  type Run,
  type Schema,
} from "./api";
import {
  date,
  Empty,
  ErrorBox,
  Field,
  JSONView,
  Pagination,
  Panel,
  State,
  useAction,
  useLoad,
  type PageProps,
} from "./common";
import { Button } from "./components/ui/button";
import { Input } from "./components/ui/input";
import { Tabs, TabsList, TabsTrigger, TabsContent } from "./components/ui/tabs";
import { SchemaForm } from "./SchemaForm";

export function PluginsPage({ user, notify, changed }: PageProps) {
  const [offset, setOffset] = useState(0),
    [selected, setSelected] = useState<string | null>(null);
  const plugins = useLoad<List<Plugin>>(`/plugins?offset=${offset}`);
  const { busy, perform } = useAction(notify);
  return (
    <div className="stack">
      <Panel
        title="GitHub에서 설치"
        description="신뢰하는 저장소의 새 규약 플러그인을 설치합니다. 현재는 공개 GitHub 저장소를 지원합니다."
      >
        <form
          className="install-form"
          onSubmit={async (e) => {
            e.preventDefault();
            const f = new FormData(e.currentTarget);
            if (
              await perform(
                () =>
                  request("/plugins/install", {
                    method: "POST",
                    body: body({
                      repository_url: f.get("repository_url"),
                      ref: f.get("ref"),
                    }),
                  }),
                "플러그인을 설치했습니다.",
              )
            ) {
              plugins.reload();
              changed();
            }
          }}
        >
          <Field label="GitHub 저장소 URL">
            <Input
              name="repository_url"
              type="url"
              required
              placeholder="https://github.com/owner/plugin"
            />
          </Field>
          <Field label="태그 또는 커밋">
            <Input name="ref" required placeholder="v1.0.0" />
          </Field>
          <Button disabled={busy}>
            <Download />
            {busy ? "설치·검증 중…" : "설치"}
          </Button>
        </form>
        <p className="muted small">
          플러그인은 서버 권한으로 별도 실행됩니다. 기존 FlaskFarm 플러그인은
          호환되지 않습니다.
        </p>
      </Panel>
      <Panel title="설치된 플러그인">
        <ErrorBox message={plugins.error} />
        <div className="table-scroll">
          <table>
            <thead>
              <tr>
                <th>플러그인</th>
                <th>버전</th>
                <th>상태</th>
                <th>관리</th>
              </tr>
            </thead>
            <tbody>
              {plugins.data?.items.map((p) => (
                <tr key={p.id}>
                  <td>
                    <strong>{p.name}</strong>
                    <small>{p.id}</small>
                  </td>
                  <td>{p.version}</td>
                  <td>
                    <State value={p.state} />
                  </td>
                  <td>
                    <div className="actions">
                      <Button
                        variant="outline"
                        size="sm"
                        onClick={() =>
                          setSelected(selected === p.id ? null : p.id)
                        }
                      >
                        <Settings />
                        관리
                      </Button>
                      <Button
                        variant="outline"
                        size="sm"
                        disabled={busy}
                        onClick={() => {
                          if (
                            p.state === "active" &&
                            !confirm(
                              "비활성화하면 대기 작업이 취소됩니다. 계속할까요?",
                            )
                          )
                            return;
                          void perform(
                            () =>
                              request(`/plugins/${id(p.id)}/state`, {
                                method: "PUT",
                                body: body({ enabled: p.state !== "active" }),
                              }),
                            "상태를 변경했습니다.",
                          ).then((ok) => {
                            if (ok) {
                              plugins.reload();
                              changed();
                            }
                          });
                        }}
                      >
                        <Power />
                        {p.state === "active" ? "비활성화" : "활성화"}
                      </Button>
                      {/^https:\/\/github\.com\//.test(p.repository_url) && (
                        <a
                          href={p.repository_url}
                          target="_blank"
                          rel="noreferrer"
                          className="external-link"
                          aria-label={`${p.name} 저장소`}
                        >
                          <ExternalLink size={16} />
                        </a>
                      )}
                    </div>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
        {plugins.data?.items.length === 0 && (
          <Empty>
            설치된 플러그인이 없습니다. 위에서 GitHub 저장소를 연결해 주세요.
          </Empty>
        )}
        <Pagination
          offset={offset}
          total={plugins.data?.total || 0}
          onChange={setOffset}
        />
      </Panel>
      {selected && (
        <PluginManager
          key={selected}
          {...{ user, notify, changed }}
          pluginId={selected}
          close={() => setSelected(null)}
          reload={() => {
            plugins.reload();
            changed();
          }}
        />
      )}
    </div>
  );
}

type Release = {
  id: string;
  version: string;
  commit_sha: string;
  data_schema_version: number;
  created_at: string;
};
function PluginManager({
  pluginId,
  notify,
  close,
  reload,
}: PageProps & { pluginId: string; close: () => void; reload: () => void }) {
  const plugin = useLoad<Plugin>(`/plugins/${id(pluginId)}`),
    releases = useLoad<List<Release>>(`/plugins/${id(pluginId)}/releases`);
  const { busy, perform } = useAction(notify);
  const refresh = () => {
    plugin.reload();
    releases.reload();
    reload();
  };
  return (
    <Panel
      title={`${plugin.data?.name || pluginId} 관리`}
      actions={
        <Button
          variant="ghost"
          size="icon"
          onClick={close}
          aria-label="플러그인 관리 닫기"
        >
          <X />
        </Button>
      }
    >
      <ErrorBox message={plugin.error || releases.error} />
      <form
        className="install-form"
        onSubmit={async (e) => {
          e.preventDefault();
          const f = new FormData(e.currentTarget);
          if (
            await perform(
              () =>
                request(`/plugins/${id(pluginId)}/update`, {
                  method: "POST",
                  body: body({ ref: f.get("ref") }),
                }),
              "새 버전을 적용했습니다.",
            )
          )
            refresh();
        }}
      >
        <Field label="업데이트할 태그 또는 커밋">
          <Input name="ref" required placeholder="v1.1.0" />
        </Field>
        <Button disabled={busy}>
          <Download />
          업데이트
        </Button>
      </form>
      <div className="table-scroll">
        <table>
          <thead>
            <tr>
              <th>버전</th>
              <th>커밋</th>
              <th>데이터 형식</th>
              <th>설치일</th>
              <th>전환</th>
            </tr>
          </thead>
          <tbody>
            {releases.data?.items.map((r) => (
              <tr key={r.id}>
                <td>{r.version}</td>
                <td>
                  <code>{r.commit_sha.slice(0, 12)}</code>
                </td>
                <td>{r.data_schema_version}</td>
                <td>{date(r.created_at)}</td>
                <td>
                  <Button
                    size="sm"
                    variant="outline"
                    disabled={busy || plugin.data?.active_release_id === r.id}
                    onClick={() => {
                      if (
                        confirm(
                          `${r.version} 버전으로 전환할까요? 호환되지 않는 데이터 형식은 서버에서 차단합니다.`,
                        )
                      )
                        void perform(
                          () =>
                            request(`/plugins/${id(pluginId)}/activate`, {
                              method: "POST",
                              body: body({ release_id: r.id }),
                            }),
                          "버전을 전환했습니다.",
                        ).then((ok) => {
                          if (ok) refresh();
                        });
                    }}
                  >
                    <RotateCcw />
                    {plugin.data?.active_release_id === r.id
                      ? "현재 버전"
                      : "전환"}
                  </Button>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
      {plugin.data && <PluginSettings plugin={plugin.data} notify={notify} />}
    </Panel>
  );
}

function PluginSettings({
  plugin,
  notify,
}: {
  plugin: Plugin;
  notify: PageProps["notify"];
}) {
  const settings = useLoad<Record<string, unknown>>(
    `/plugins/${id(plugin.id)}/settings`,
  );
  const { busy, perform } = useAction(notify);
  return (
    <section className="stack">
      <h3>플러그인 설정</h3>
      <ErrorBox message={settings.error} />
      {settings.data && (
        <SchemaForm
          key={JSON.stringify(settings.data)}
          schema={plugin.manifest.settings_schema}
          initial={settings.data}
          disabled={busy}
          onSubmit={async (value) => {
            if (
              await perform(() =>
                request(`/plugins/${id(plugin.id)}/settings`, {
                  method: "PUT",
                  body: body(value),
                }),
              )
            )
              settings.reload();
          }}
        />
      )}
    </section>
  );
}

export function PluginWorkspace({
  pluginId,
  menuId,
  user,
  notify,
  changed,
}: PageProps & { pluginId: string; menuId?: string }) {
  const plugin = useLoad<Plugin>(`/plugins/${id(pluginId)}`);
  const [lastRun, setLastRun] = useState<string | null>(null);
  const { busy, perform } = useAction(notify);
  if (!plugin.data)
    return (
      <Panel title="플러그인 작업 공간">
        <ErrorBox message={plugin.error} />
        {plugin.loading && <Empty>불러오는 중입니다.</Empty>}
      </Panel>
    );
  const p = plugin.data,
    resource = p.manifest.menus?.find((m) => m.id === menuId)?.resource;
  return (
    <Panel
      title={p.name}
      description={`${p.id} · ${p.version}`}
      actions={<State value={p.state} />}
    >
      <ErrorBox message={plugin.error} />
      {p.state !== "active" && (
        <ErrorBox message="비활성 플러그인입니다. 관리자에게 활성화를 요청하세요." />
      )}
      <Tabs defaultValue={resource ? "resource" : "tasks"}>
        <TabsList>
          {resource && <TabsTrigger value="resource">데이터</TabsTrigger>}
          <TabsTrigger value="tasks">작업</TabsTrigger>
          {user.role === "admin" && (
            <TabsTrigger value="settings">설정</TabsTrigger>
          )}
        </TabsList>
        {resource && (
          <TabsContent value="resource">
            <ResourcePanel
              plugin={p}
              resourceId={resource}
              {...{ user, notify, changed }}
            />
          </TabsContent>
        )}
        <TabsContent value="tasks">
          <div className="stack">
            {p.manifest.tasks?.map((task) => (
              <section className="edit-panel stack" key={task.id}>
                <h3>{task.title}</h3>
                <p className="muted small">
                  {task.id}
                  {task.timeout_seconds
                    ? ` · 최대 ${task.timeout_seconds}초`
                    : ""}
                </p>
                <SchemaForm
                  schema={task.input_schema}
                  label="작업 실행"
                  disabled={
                    busy || user.role === "viewer" || p.state !== "active"
                  }
                  onSubmit={async (input) => {
                    await perform(async () => {
                      const run = await request<Run>(
                        `/plugins/${id(p.id)}/tasks/${id(task.id)}/runs`,
                        {
                          method: "POST",
                          body: body({ input }),
                        },
                      );
                      setLastRun(run.id);
                      changed();
                    }, "작업을 대기열에 추가했습니다.");
                  }}
                />
              </section>
            ))}
            {!p.manifest.tasks?.length && (
              <Empty>선언된 작업이 없습니다.</Empty>
            )}
            {lastRun && (
              <p className="notice">
                작업을 요청했습니다. 작업 이력에서 진행 상황을 확인하세요.{" "}
                <code>{lastRun}</code>
              </p>
            )}
          </div>
        </TabsContent>
        {user.role === "admin" && (
          <TabsContent value="settings">
            <PluginSettings plugin={p} notify={notify} />
          </TabsContent>
        )}
      </Tabs>
    </Panel>
  );
}

function ResourcePanel({
  plugin,
  resourceId,
  user,
  notify,
  changed,
}: PageProps & { plugin: Plugin; resourceId: string }) {
  const resource = plugin.manifest.resources?.find(
    (r) => r.id === resourceId,
  ) as { id: string; methods: string[]; input_schema?: Schema } | undefined;
  const [revision, setRevision] = useState(0);
  const { busy, perform } = useAction(notify);
  if (!resource)
    return (
      <ErrorBox message="플러그인이 이 메뉴의 데이터 리소스를 선언하지 않았습니다." />
    );
  return (
    <div className="stack">
      {resource.methods.includes("GET") && (
        <ResourceTable
          path={`/plugins/${id(plugin.id)}/resources/${id(resourceId)}`}
          revision={revision}
        />
      )}{" "}
      {user.role !== "viewer" &&
        resource.methods
          .filter((m) => m !== "GET")
          .map((method) => (
            <section className="edit-panel stack" key={method}>
              <h3>
                {method === "POST"
                  ? "추가"
                  : method === "PUT"
                    ? "변경"
                    : "삭제"}
              </h3>
              <SchemaForm
                schema={resource.input_schema}
                disabled={busy || plugin.state !== "active"}
                label={method === "DELETE" ? "삭제" : "전송"}
                onSubmit={async (input) => {
                  if (
                    method === "DELETE" &&
                    !confirm("선택한 데이터를 삭제할까요?")
                  )
                    return;
                  if (
                    await perform(() =>
                      request(
                        `/plugins/${id(plugin.id)}/resources/${id(resourceId)}`,
                        { method, body: body(input) },
                      ),
                    )
                  ) {
                    setRevision((v) => v + 1);
                    changed();
                  }
                }}
              />
            </section>
          ))}
    </div>
  );
}
function ResourceTable({ path, revision }: { path: string; revision: number }) {
  const resource = useLoad<unknown>(path, revision);
  const result = resource.data;
  const rows = Array.isArray(result)
    ? result
    : result &&
        typeof result === "object" &&
        Array.isArray((result as { items?: unknown }).items)
      ? (result as { items: unknown[] }).items
      : null;
  const objectRows = rows?.every(
    (r) => r && typeof r === "object" && !Array.isArray(r),
  )
    ? (rows as Record<string, unknown>[])
    : null;
  const columns = objectRows?.length ? Object.keys(objectRows[0]) : [];
  return (
    <>
      <ErrorBox message={resource.error} />
      {resource.loading && <p className="muted">불러오는 중입니다.</p>}
      {objectRows ? (
        objectRows.length ? (
          <div className="table-scroll">
            <table>
              <thead>
                <tr>
                  {columns.map((c) => (
                    <th key={c}>{c}</th>
                  ))}
                </tr>
              </thead>
              <tbody>
                {objectRows.map((row, i) => (
                  <tr key={i}>
                    {columns.map((c) => (
                      <td key={c} className="wrap">
                        {typeof row[c] === "object"
                          ? JSON.stringify(row[c])
                          : String(row[c] ?? "")}
                      </td>
                    ))}
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        ) : (
          <Empty />
        )
      ) : (
        result !== undefined && <JSONView value={result} />
      )}
    </>
  );
}
