// 사용자·API 토큰·메뉴·비밀번호 설정 화면을 제공한다.
import { useState, type FormEvent } from "react";
import { Plus, Save, Trash2, Copy, Pencil, X } from "lucide-react";
import { request, body, id, type List, type User, type Menu } from "./api";
import {
  useLoad,
  useAction,
  Panel,
  Field,
  ErrorBox,
  Empty,
  Pagination,
  date,
  type PageProps,
} from "./common";
import { Button } from "./components/ui/button";
import { Input } from "./components/ui/input";
import { Textarea } from "./components/ui/textarea";

export function UsersPage({ user, notify }: PageProps) {
  const [offset, setOffset] = useState(0),
    [editing, setEditing] = useState<User | null>(null),
    [open, setOpen] = useState(false);
  const users = useLoad<List<User>>(`/users?offset=${offset}`);
  const { busy, perform } = useAction(notify);
  async function save(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    const fields = new FormData(e.currentTarget);
    const payload: Record<string, unknown> = {
      username: fields.get("username"),
      role: fields.get("role"),
    };
    if (editing) payload.is_active = fields.get("is_active") === "on";
    if (fields.get("password")) payload.password = fields.get("password");
    if (
      await perform(() =>
        request(editing ? `/users/${id(editing.id)}` : "/users", {
          method: editing ? "PUT" : "POST",
          body: body(payload),
        }),
      )
    ) {
      setOpen(false);
      setEditing(null);
      users.reload();
    }
  }
  return (
    <Panel
      title="사용자 계정"
      description="계정과 운영 권한을 관리합니다. 마지막 관리자는 삭제할 수 없습니다."
      actions={
        <Button
          onClick={() => {
            setEditing(null);
            setOpen(true);
          }}
        >
          <Plus />
          계정 추가
        </Button>
      }
    >
      <ErrorBox message={users.error} />
      {open && (
        <form
          className="edit-panel stack"
          onSubmit={save}
          key={editing?.id || "new"}
        >
          <div className="panel-heading">
            <h3>{editing ? "계정 변경" : "새 계정"}</h3>
            <Button
              type="button"
              variant="ghost"
              size="icon"
              aria-label="편집 닫기"
              onClick={() => setOpen(false)}
            >
              <X />
            </Button>
          </div>
          <div className="form-grid">
            <Field label="사용자 이름">
              <Input
                name="username"
                required
                minLength={2}
                maxLength={64}
                defaultValue={editing?.username}
              />
            </Field>
            <Field label={editing ? "새 비밀번호 (선택)" : "초기 비밀번호"}>
              <Input
                name="password"
                type="password"
                autoComplete="new-password"
                required={!editing}
                minLength={12}
                maxLength={128}
              />
            </Field>
            <Field label="역할">
              <select name="role" defaultValue={editing?.role || "viewer"}>
                <option value="viewer">조회자</option>
                <option value="operator">운영자</option>
                <option value="admin">관리자</option>
              </select>
            </Field>
            {editing && (
              <label className="check">
                <input
                  name="is_active"
                  type="checkbox"
                  defaultChecked={editing.is_active}
                />
                활성 계정
              </label>
            )}
          </div>
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
              <th>이름</th>
              <th>역할</th>
              <th>상태</th>
              <th>생성일</th>
              <th>관리</th>
            </tr>
          </thead>
          <tbody>
            {users.data?.items.map((u) => (
              <tr key={u.id}>
                <td>
                  <strong>{u.username}</strong>
                  {u.id === user.id && (
                    <small className="muted"> (내 계정)</small>
                  )}
                </td>
                <td>{u.role}</td>
                <td>{u.is_active ? "활성" : "비활성"}</td>
                <td>{date(u.created_at)}</td>
                <td>
                  <div className="actions">
                    <Button
                      variant="outline"
                      size="sm"
                      onClick={() => {
                        setEditing(u);
                        setOpen(true);
                      }}
                    >
                      <Pencil />
                      변경
                    </Button>
                    <Button
                      variant="ghost"
                      size="icon-sm"
                      disabled={busy || u.id === user.id}
                      aria-label={`${u.username} 삭제`}
                      onClick={() => {
                        if (confirm(`${u.username} 계정을 삭제할까요?`))
                          void perform(
                            () =>
                              request(`/users/${id(u.id)}`, {
                                method: "DELETE",
                              }),
                            "삭제했습니다.",
                          ).then((ok) => {
                            if (ok) users.reload();
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
      {users.data?.items.length === 0 && <Empty />}
      <Pagination
        offset={offset}
        total={users.data?.total || 0}
        onChange={setOffset}
      />
    </Panel>
  );
}

type Token = {
  id: string;
  name: string;
  scopes: string[];
  expires_at: string;
  created_at: string;
};
const scopes = [
  "status:read",
  "plugins:read",
  "plugins:write",
  "runs:read",
  "runs:write",
  "schedules:read",
  "schedules:write",
  "logs:read",
  "users:write",
  "tokens:write",
];
export function TokensPage({ notify }: PageProps) {
  const [offset, setOffset] = useState(0),
    [token, setToken] = useState("");
  const tokens = useLoad<List<Token>>(`/tokens?offset=${offset}`);
  const { busy, perform } = useAction(notify);
  async function create(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    const form = e.currentTarget,
      fields = new FormData(form);
    await perform(async () => {
      const result = await request<{ token: string }>("/tokens", {
        method: "POST",
        body: body({
          name: fields.get("name"),
          scopes: fields.getAll("scope"),
          expires_at: new Date(String(fields.get("expires_at"))).toISOString(),
        }),
      });
      setToken(result.token);
      form.reset();
      tokens.reload();
    }, "토큰을 생성했습니다. 지금 안전한 곳에 보관하세요.");
  }
  return (
    <Panel
      title="외부 API 토큰"
      description="외부 앱에 필요한 권한만 부여하세요. 토큰 원문은 생성 직후 한 번만 표시합니다."
    >
      <ErrorBox message={tokens.error} />
      {token && (
        <div className="token-result stack">
          <strong>새 API 토큰</strong>
          <Textarea
            readOnly
            aria-label="새 API 토큰"
            value={token}
            className="json-input"
            onFocus={(e) => e.currentTarget.select()}
          />
          <div className="actions">
            <Button
              variant="outline"
              disabled={!navigator.clipboard}
              title={
                !navigator.clipboard
                  ? "HTTP 접속에서는 토큰을 선택해 직접 복사하세요."
                  : undefined
              }
              onClick={() =>
                void perform(
                  () => navigator.clipboard.writeText(token),
                  "클립보드에 복사했습니다.",
                )
              }
            >
              <Copy />
              복사
            </Button>
            <Button variant="ghost" onClick={() => setToken("")}>
              표시 닫기
            </Button>
          </div>
        </div>
      )}
      <form onSubmit={create} className="edit-panel stack">
        <div className="form-grid">
          <Field label="토큰 이름">
            <Input
              name="name"
              required
              maxLength={100}
              placeholder="외부 자동화 서버"
            />
          </Field>
          <Field label="만료 시각">
            <Input name="expires_at" type="datetime-local" required />
          </Field>
        </div>
        <fieldset>
          <legend>허용 권한</legend>
          <div className="scope-grid">
            {scopes.map((scope) => (
              <label className="check" key={scope}>
                <input type="checkbox" name="scope" value={scope} />
                {scope}
              </label>
            ))}
          </div>
        </fieldset>
        <div className="form-actions">
          <Button disabled={busy}>
            <Plus />
            토큰 생성
          </Button>
        </div>
      </form>
      <div className="table-scroll">
        <table>
          <thead>
            <tr>
              <th>이름</th>
              <th>허용 권한</th>
              <th>만료</th>
              <th>관리</th>
            </tr>
          </thead>
          <tbody>
            {tokens.data?.items.map((t) => (
              <tr key={t.id}>
                <td>{t.name}</td>
                <td className="wrap">{t.scopes.join(", ")}</td>
                <td>{date(t.expires_at)}</td>
                <td>
                  <Button
                    variant="outline"
                    size="sm"
                    disabled={busy}
                    onClick={() => {
                      if (
                        confirm(
                          `${t.name} 토큰을 폐기할까요? 연결된 앱은 인증할 수 없게 됩니다.`,
                        )
                      )
                        void perform(
                          () =>
                            request(`/tokens/${id(t.id)}`, {
                              method: "DELETE",
                            }),
                          "토큰을 폐기했습니다.",
                        ).then((ok) => {
                          if (ok) tokens.reload();
                        });
                    }}
                  >
                    <Trash2 />
                    폐기
                  </Button>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
      <Pagination
        offset={offset}
        total={tokens.data?.total || 0}
        onChange={setOffset}
      />
    </Panel>
  );
}

export function PasswordPage({
  notify,
  onSignedOut,
}: PageProps & { onSignedOut: () => void }) {
  const { busy, perform } = useAction(notify);
  return (
    <Panel
      title="비밀번호 변경"
      description="변경하면 모든 로그인 세션과 API 토큰이 폐기됩니다."
    >
      <form
        className="stack narrow-form"
        onSubmit={async (e) => {
          e.preventDefault();
          const fields = new FormData(e.currentTarget);
          if (fields.get("new_password") !== fields.get("confirm_password")) {
            notify("새 비밀번호가 일치하지 않습니다.", true);
            return;
          }
          if (
            await perform(
              () =>
                request("/auth/password", {
                  method: "PUT",
                  body: body({
                    current_password: fields.get("current_password"),
                    new_password: fields.get("new_password"),
                  }),
                }),
              "비밀번호를 변경했습니다. 다시 로그인해 주세요.",
            )
          )
            onSignedOut();
        }}
      >
        <Field label="현재 비밀번호">
          <Input
            name="current_password"
            type="password"
            autoComplete="current-password"
            required
          />
        </Field>
        <Field label="새 비밀번호">
          <Input
            name="new_password"
            type="password"
            autoComplete="new-password"
            required
            minLength={12}
            maxLength={128}
          />
        </Field>
        <Field label="새 비밀번호 확인">
          <Input
            name="confirm_password"
            type="password"
            autoComplete="new-password"
            required
            minLength={12}
            maxLength={128}
          />
        </Field>
        <Button disabled={busy}>
          <Save />
          비밀번호 변경
        </Button>
      </form>
    </Panel>
  );
}

export function MenusPage({ notify, changed }: PageProps) {
  const menus = useLoad<List<Menu>>("/menus");
  return (
    <Panel
      title="플러그인 메뉴 편집"
      description="사이드바에서 보이는 이름과 순서, 표시 여부를 변경합니다."
    >
      <ErrorBox message={menus.error} />
      <div className="stack">
        {menus.data?.items.map((m) => (
          <MenuEditor
            key={`${m.plugin_id}:${m.id}:${m.title}:${m.position}:${m.enabled}`}
            menu={m}
            notify={notify}
            saved={() => {
              menus.reload();
              changed();
            }}
          />
        ))}
      </div>
      {menus.data?.items.length === 0 && (
        <Empty>플러그인을 설치하면 메뉴를 편집할 수 있습니다.</Empty>
      )}
    </Panel>
  );
}
function MenuEditor({
  menu,
  notify,
  saved,
}: {
  menu: Menu;
  notify: PageProps["notify"];
  saved: () => void;
}) {
  const { busy, perform } = useAction(notify);
  return (
    <form
      className="menu-editor"
      onSubmit={async (e) => {
        e.preventDefault();
        const f = new FormData(e.currentTarget);
        if (
          await perform(() =>
            request(`/menus/${id(menu.plugin_id)}/${id(menu.id)}`, {
              method: "PUT",
              body: body({
                title: f.get("title"),
                position: Number(f.get("position")),
                enabled: f.get("enabled") === "on",
              }),
            }),
          )
        )
          saved();
      }}
    >
      <div>
        <strong>{menu.plugin_name}</strong>
        <small>{menu.id}</small>
      </div>
      <Field label="표시 이름">
        <Input
          name="title"
          required
          defaultValue={menu.title}
          maxLength={100}
        />
      </Field>
      <Field label="순서">
        <Input
          name="position"
          type="number"
          defaultValue={menu.position}
          min={0}
          max={9999}
        />
      </Field>
      <label className="check">
        <input type="checkbox" name="enabled" defaultChecked={menu.enabled} />
        표시
      </label>
      <Button disabled={busy}>
        <Save />
        저장
      </Button>
    </form>
  );
}
