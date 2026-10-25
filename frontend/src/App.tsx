// ModuDock Go의 관리 콘솔을 구성한다.
import {
  useCallback,
  useEffect,
  useRef,
  useState,
  type FormEvent,
} from "react";
import {
  Activity,
  Anchor,
  CalendarClock,
  ChevronDown,
  ChevronLeft,
  ChevronRight,
  CircleCheck,
  CircleAlert,
  History,
  KeyRound,
  LayoutDashboard,
  ListTree,
  LogOut,
  Menu as MenuIcon,
  Puzzle,
  RefreshCw,
  ScrollText,
  Settings,
  ShieldCheck,
  Fingerprint,
  Users,
  Zap,
  type LucideIcon,
} from "lucide-react";
import {
  session,
  signIn,
  signOut,
  completeMFA,
  signInWithPasskey,
  request,
  type User,
  type List,
  type Menu,
  type Status,
} from "./api";
import {
  useLoad,
  useAction,
  ErrorBox,
  Field,
  date,
  type Notify,
} from "./common";
import { Button } from "./components/ui/button";
import { Card } from "./components/ui/card";
import { Input } from "./components/ui/input";
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
  SheetTrigger,
} from "./components/ui/sheet";
import { UsersPage, TokensPage, MenusPage } from "./Settings";
import { SecurityPage, RecoveryCodes } from "./Security";
import { passkeyBrowserSupport, PasskeyCancelled } from "./webauthn";
import { RunsPage, SchedulesPage, LogsPage } from "./Operations";
import { PluginsPage, PluginWorkspace } from "./Plugins";

type NavItem = {
  id: string;
  title: string;
  icon: LucideIcon;
  admin?: boolean;
  operator?: boolean;
};
const navigation: NavItem[] = [
  { id: "overview", title: "개요", icon: LayoutDashboard },
  { id: "runs", title: "작업 이력", icon: History },
];
const settings: NavItem[] = [
  { id: "plugins", title: "플러그인", icon: Puzzle, operator: true },
  { id: "schedules", title: "스케줄", icon: CalendarClock, operator: true },
  { id: "users", title: "사용자 계정", icon: Users, admin: true },
  { id: "tokens", title: "API 토큰", icon: KeyRound, admin: true },
  { id: "menus", title: "메뉴 편집", icon: ListTree, admin: true },
  { id: "logs", title: "전체 로그", icon: ScrollText, admin: true },
  { id: "security", title: "계정 보안", icon: ShieldCheck },
];
const menuIcons: Record<string, LucideIcon> = {
  puzzle: Puzzle,
  activity: Activity,
  calendar: CalendarClock,
  history: History,
  list: ListTree,
  zap: Zap,
};

function Login({ onLogin }: { onLogin: (user: User) => void }) {
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  const [pending, setPending] = useState("");
  const [method, setMethod] = useState<"totp" | "recovery">("totp");
  const [code, setCode] = useState("");
  const [options, setOptions] = useState<{
    passkey_available: boolean;
    passkey_reason: string;
  }>();
  const active = useRef<AbortController | null>(null);
  useEffect(() => {
    const controller = new AbortController();
    request<{ passkey_available: boolean; passkey_reason: string }>(
      "/auth/options",
      { signal: controller.signal },
    )
      .then(setOptions)
      .catch(() => {});
    return () => {
      controller.abort();
      active.current?.abort();
    };
  }, []);
  const passkeyReason =
    passkeyBrowserSupport() || options?.passkey_reason || "";
  async function authenticate(passkey = false) {
    active.current?.abort();
    const controller = new AbortController();
    active.current = controller;
    setBusy(true);
    setError("");
    try {
      if (passkey) {
        const user = await signInWithPasskey(controller.signal);
        if (!controller.signal.aborted) onLogin(user);
      } else if (pending) {
        const user = await completeMFA(method, code, controller.signal);
        if (!controller.signal.aborted) {
          setCode("");
          onLogin(user);
        }
      } else {
        const result = await signIn(username, password, controller.signal);
        if (controller.signal.aborted) return;
        setPassword("");
        if ("mfa_required" in result) {
          setPending(result.expires_at);
          setCode("");
        } else {
          onLogin(result.user);
        }
      }
    } catch (e) {
      if (!controller.signal.aborted && !(e instanceof PasskeyCancelled))
        setError(e instanceof Error ? e.message : "로그인하지 못했습니다.");
    } finally {
      if (!controller.signal.aborted) setBusy(false);
    }
  }
  async function submit(e: FormEvent) {
    e.preventDefault();
    await authenticate();
  }
  return (
    <main className="login-page">
      <div className="login-brand">
        <span className="brand-icon">
          <Anchor />
        </span>
        <h1>
          ModuDock <span>Go</span>
        </h1>
      </div>
      <Card className="login-card">
        <h2>{pending ? "2단계 인증" : "다시 만나 반갑습니다."}</h2>
        <p className="muted">
          {pending
            ? "인증 앱의 6자리 코드 또는 일회용 복구 코드를 입력하세요."
            : "자동화와 플러그인을 한 곳에서 관리하세요."}
        </p>
        <form onSubmit={submit} className="stack">
          {pending ? (
            <>
              <Field label="인증 방법">
                <select
                  disabled={busy}
                  value={method}
                  onChange={(e) => {
                    setMethod(e.target.value as "totp" | "recovery");
                    setCode("");
                    setError("");
                  }}
                >
                  <option value="totp">인증 앱</option>
                  <option value="recovery">복구 코드</option>
                </select>
              </Field>
              <Field label={method === "totp" ? "인증 코드" : "복구 코드"}>
                <Input
                  autoFocus
                  required
                  autoComplete="one-time-code"
                  inputMode={method === "totp" ? "numeric" : "text"}
                  pattern={method === "totp" ? "[0-9]{6}" : undefined}
                  maxLength={method === "totp" ? 6 : 40}
                  value={code}
                  onChange={(e) => setCode(e.target.value)}
                />
              </Field>
              <p className="muted small">
                인증 대기는 5분 후 만료됩니다. 만료되면 취소 후 다시
                로그인하세요.
              </p>
            </>
          ) : (
            <>
              <Field label="사용자 이름">
                <Input
                  required
                  autoComplete="username"
                  value={username}
                  onChange={(e) => setUsername(e.target.value)}
                />
              </Field>
              <Field label="비밀번호">
                <Input
                  required
                  type="password"
                  autoComplete="current-password"
                  value={password}
                  onChange={(e) => setPassword(e.target.value)}
                />
              </Field>
            </>
          )}
          <ErrorBox message={error} />
          <Button disabled={busy}>
            {busy ? "확인 중…" : pending ? "인증 후 로그인" : "로그인"}
          </Button>
          {pending && (
            <Button
              type="button"
              variant="outline"
              onClick={() => {
                active.current?.abort();
                setBusy(true);
                void request("/auth/login/cancel", { method: "POST" })
                  .catch(() => {})
                  .finally(() => {
                    setPending("");
                    setCode("");
                    setPassword("");
                    setError("");
                    setBusy(false);
                  });
              }}
            >
              로그인 취소
            </Button>
          )}
        </form>
        {!pending && (
          <>
            <Button
              variant="outline"
              disabled={busy || !options?.passkey_available || !!passkeyReason}
              onClick={() => void authenticate(true)}
            >
              <Fingerprint />
              Passkey로 로그인
            </Button>
            {passkeyReason && <p className="muted small">{passkeyReason}</p>}
          </>
        )}
        <p className="muted small">
          첫 설치라면 서버에서 관리자 계정을 먼저 생성해 주세요.
        </p>
      </Card>
      <p className="login-foot">작게 설치하고, 필요한 만큼 확장하세요.</p>
    </main>
  );
}

export default function App() {
  const [recoveryCodes, setRecoveryCodes] = useState<string[]>([]);
  const [user, setUser] = useState<User | null>(null),
    [checking, setChecking] = useState(true),
    [view, setView] = useState("overview"),
    [revision, setRevision] = useState(0),
    [drawer, setDrawer] = useState(false),
    [settingsOpen, setSettingsOpen] = useState(true);
  const [collapsed, setCollapsed] = useState(
    () => localStorage.getItem("modudock-go-sidebar-collapsed") === "true",
  );
  const [notice, setNotice] = useState<{
    message: string;
    error: boolean;
  } | null>(null);
  const notify: Notify = useCallback(
    (message, error = false) => setNotice({ message, error }),
    [],
  );
  const changed = useCallback(() => setRevision((n) => n + 1), []);
  useEffect(() => {
    const expired = () => {
      setUser(null);
      setDrawer(false);
      setNotice(null);
      setView("overview");
    };
    window.addEventListener("modudock:unauthorized", expired);
    return () => window.removeEventListener("modudock:unauthorized", expired);
  }, []);
  useEffect(() => {
    let active = true;
    session()
      .then((u) => {
        if (active) setUser(u);
      })
      .catch(() => {})
      .finally(() => {
        if (active) setChecking(false);
      });
    return () => {
      active = false;
    };
  }, []);
  useEffect(() => {
    localStorage.setItem("modudock-go-sidebar-collapsed", String(collapsed));
  }, [collapsed]);
  useEffect(() => {
    const mq = window.matchMedia?.("(min-width: 768px)");
    const close = () => {
      if (mq?.matches) setDrawer(false);
    };
    mq?.addEventListener("change", close);
    return () => mq?.removeEventListener("change", close);
  }, []);
  useEffect(() => {
    if (!user) return;
    const es = new EventSource("/api/v1/events");
    es.addEventListener("change", changed);
    const timer = setInterval(changed, 30000);
    return () => {
      es.close();
      clearInterval(timer);
    };
  }, [user, changed]);
  const action = useAction(notify);
  if (recoveryCodes.length)
    return (
      <RecoveryCodes
        codes={recoveryCodes}
        onClose={() => setRecoveryCodes([])}
      />
    );
  if (checking)
    return (
      <main className="loading-page" aria-busy="true">
        <RefreshCw className="spin" />
        연결 중입니다.
      </main>
    );
  if (!user)
    return (
      <Login
        onLogin={(u) => {
          setUser(u);
          setNotice(null);
          setView("overview");
          changed();
        }}
      />
    );
  return (
    <Console
      user={user}
      view={view}
      revision={revision}
      collapsed={collapsed}
      drawer={drawer}
      settingsOpen={settingsOpen}
      notice={notice}
      notify={notify}
      changed={changed}
      onNavigate={(v) => {
        setView(v);
        setDrawer(false);
        setNotice(null);
      }}
      onCollapse={() => setCollapsed((v) => !v)}
      onDrawer={setDrawer}
      onSettings={() => setSettingsOpen((v) => !v)}
      onDismiss={() => setNotice(null)}
      onSignedOut={() => {
        setUser(null);
        setView("overview");
      }}
      onRecoveryCodes={setRecoveryCodes}
      onLogout={() =>
        void action.perform(async () => {
          await signOut();
          setUser(null);
        }, "")
      }
    />
  );
}

function Console({
  user,
  view,
  revision,
  collapsed,
  drawer,
  settingsOpen,
  notice,
  notify,
  changed,
  onNavigate,
  onCollapse,
  onDrawer,
  onSettings,
  onDismiss,
  onSignedOut,
  onRecoveryCodes,
  onLogout,
}: {
  user: User;
  view: string;
  revision: number;
  collapsed: boolean;
  drawer: boolean;
  settingsOpen: boolean;
  notice: { message: string; error: boolean } | null;
  notify: Notify;
  changed: () => void;
  onNavigate: (view: string) => void;
  onCollapse: () => void;
  onDrawer: (open: boolean) => void;
  onSettings: () => void;
  onDismiss: () => void;
  onSignedOut: () => void;
  onRecoveryCodes: (codes: string[]) => void;
  onLogout: () => void;
}) {
  const status = useLoad<Status>("/status", revision),
    menus = useLoad<List<Menu>>("/menus", revision);
  const allowed = settings.filter(
    (item) =>
      (!item.admin || user.role === "admin") &&
      (!item.operator || user.role !== "viewer"),
  );
  const pluginMenus = menus.data?.items.filter((m) => m.enabled) || [];
  const pluginMenu = pluginMenus.find(
    (m) => view === `plugin:${m.plugin_id}:${m.id}`,
  );
  const title =
    pluginMenu?.title ||
    [...navigation, ...settings].find((n) => n.id === view)?.title ||
    "플러그인";
  const nav = (mobile = false) => (
    <div className="sidebar-inner">
      <button
        className="brand"
        onClick={() => onNavigate("overview")}
        aria-label="ModuDock Go 개요"
      >
        <span className="brand-icon">
          <Anchor />
        </span>
        {(!collapsed || mobile) && (
          <span>
            <strong>
              ModuDock <em>Go</em>
            </strong>
            <small>Automation platform</small>
          </span>
        )}
      </button>
      <nav aria-label={mobile ? "모바일 내비게이션" : "주 내비게이션"}>
        {navigation.map((item) => navButton(item, mobile))}
        {pluginMenus.length > 0 && (
          <>
            <div className="nav-label">
              {!collapsed || mobile ? "플러그인" : "·"}
            </div>
            {pluginMenus.map((m) =>
              navButton(
                {
                  id: `plugin:${m.plugin_id}:${m.id}`,
                  title: m.title,
                  icon: menuIcons[m.icon] || Puzzle,
                },
                mobile,
              ),
            )}
          </>
        )}
        <div className="nav-divider" />
        <button
          className="nav-item"
          onClick={onSettings}
          title="설정"
          aria-expanded={settingsOpen}
        >
          <Settings />
          {(!collapsed || mobile) && (
            <>
              <span>설정</span>
              <ChevronDown
                className={`nav-chevron ${settingsOpen ? "" : "rotated"}`}
              />
            </>
          )}
        </button>
        {settingsOpen && allowed.map((item) => navButton(item, mobile, true))}
      </nav>
      <div className="sidebar-bottom">
        <div className={`connection ${status.error ? "bad" : ""}`}>
          {status.error ? <CircleAlert size={16} /> : <CircleCheck size={16} />}
          <span>{status.error ? "연결 확인 필요" : "연결됨"}</span>
        </div>
        {(!collapsed || mobile) && (
          <div className="account">
            <strong>{user.username}</strong>
            <span>
              {user.role === "admin"
                ? "관리자"
                : user.role === "operator"
                  ? "운영자"
                  : "조회자"}
            </span>
          </div>
        )}
        {!mobile && (
          <button
            className="nav-item"
            onClick={onCollapse}
            aria-label={collapsed ? "사이드바 펼치기" : "사이드바 접기"}
            title={collapsed ? "사이드바 펼치기" : "사이드바 접기"}
          >
            {collapsed ? <ChevronRight /> : <ChevronLeft />}
            {!collapsed && <span>사이드바 접기</span>}
          </button>
        )}
        <button className="nav-item" onClick={onLogout} title="로그아웃">
          <LogOut />
          {(!collapsed || mobile) && <span>로그아웃</span>}
        </button>
      </div>
    </div>
  );
  function navButton(item: NavItem, mobile: boolean, child = false) {
    const Icon = item.icon;
    return (
      <button
        key={item.id}
        className={`nav-item ${view === item.id ? "selected" : ""} ${child ? "nav-child" : ""}`}
        aria-current={view === item.id ? "page" : undefined}
        aria-label={item.title}
        title={item.title}
        onClick={() => onNavigate(item.id)}
      >
        <Icon />
        {(!collapsed || mobile) && <span>{item.title}</span>}
      </button>
    );
  }
  const props = { user, notify, changed };
  return (
    <div className={`app-shell ${collapsed ? "is-collapsed" : ""}`}>
      <aside className="desktop-sidebar">{nav()}</aside>
      <div className="app-main">
        <header className="mobile-header">
          <Sheet open={drawer} onOpenChange={onDrawer}>
            <SheetTrigger asChild>
              <Button size="icon" variant="ghost" aria-label="메뉴 열기">
                <MenuIcon />
              </Button>
            </SheetTrigger>
            <SheetContent side="left" className="mobile-sidebar">
              <SheetHeader className="sr-only">
                <SheetTitle>내비게이션</SheetTitle>
                <SheetDescription>관리 화면을 선택합니다.</SheetDescription>
              </SheetHeader>
              {nav(true)}
            </SheetContent>
          </Sheet>
          <strong>ModuDock Go</strong>
        </header>
        <main className="page">
          <header className="page-header">
            <div>
              <p className="eyebrow">
                MODUDOCK /{" "}
                {view.startsWith("plugin:")
                  ? "PLUGIN"
                  : settings.some((s) => s.id === view)
                    ? "설정"
                    : "WORKSPACE"}
              </p>
              <h1>{title}</h1>
            </div>
            <Button
              variant="outline"
              onClick={() =>
                window.dispatchEvent(new Event("modudock:refresh"))
              }
            >
              <RefreshCw />
              새로고침
            </Button>
          </header>
          {notice && (
            <div
              className={`notice ${notice.error ? "notice-error" : ""}`}
              role={notice.error ? "alert" : "status"}
            >
              <span>{notice.message}</span>
              <button onClick={onDismiss} aria-label="알림 닫기">
                ×
              </button>
            </div>
          )}
          <ErrorBox message={status.error} />
          {view === "overview" && (
            <>
              <div className="hero-strip">
                <div>
                  <span className="eyebrow">YOUR AUTOMATION, ONE PLACE</span>
                  <h2>운영 현황을 한눈에.</h2>
                  <p>작업을 확인하고, 다음 자동화를 준비하세요.</p>
                </div>
                <span className="hero-icon">
                  <Activity />
                </span>
              </div>
              <div className="stats-grid">
                {[
                  {
                    label: "활성 플러그인",
                    value: status.data?.plugins_active,
                    icon: Puzzle,
                  },
                  {
                    label: "대기 중인 작업",
                    value: status.data?.runs_queued,
                    icon: History,
                  },
                  {
                    label: "실행 중인 작업",
                    value: status.data?.runs_running,
                    icon: Activity,
                  },
                  {
                    label: "완료된 작업",
                    value: status.data?.runs_succeeded,
                    icon: CircleCheck,
                  },
                ].map((c) => (
                  <Card className="stat-card" key={c.label}>
                    <div>
                      <span>{c.label}</span>
                      <c.icon />
                    </div>
                    <strong>{c.value ?? "—"}</strong>
                    <small>현재 서버 기준</small>
                  </Card>
                ))}
              </div>
              <div className="shortcuts">
                {[
                  { id: "runs", title: "작업 이력", icon: History },
                  ...allowed.filter((i) =>
                    ["plugins", "schedules", "logs"].includes(i.id),
                  ),
                ].map((i) => (
                  <Button
                    key={i.id}
                    variant="outline"
                    onClick={() => onNavigate(i.id)}
                  >
                    <i.icon />
                    {i.title}
                    <ChevronRight />
                  </Button>
                ))}
              </div>
              <RunsPage {...props} revision={revision} compact />
              <p className="muted small">
                버전 {status.data?.version || "—"} · 시작{" "}
                {date(status.data?.started_at)}
              </p>
            </>
          )}
          {view === "runs" && <RunsPage {...props} revision={revision} />}{" "}
          {view === "plugins" && user.role !== "viewer" && (
            <PluginsPage
              {...props}
              onOpenWorkspace={(pluginId) => onNavigate(`plugin:${pluginId}`)}
            />
          )}{" "}
          {view === "schedules" && user.role !== "viewer" && (
            <SchedulesPage {...props} />
          )}{" "}
          {view === "users" && user.role === "admin" && (
            <UsersPage {...props} />
          )}{" "}
          {view === "tokens" && user.role === "admin" && (
            <TokensPage {...props} />
          )}{" "}
          {view === "menus" && user.role === "admin" && (
            <MenusPage {...props} />
          )}{" "}
          {view === "logs" && user.role === "admin" && <LogsPage {...props} />}{" "}
          {view === "security" && (
            <SecurityPage
              key={user.id}
              {...props}
              onSignedOut={onSignedOut}
              onRecoveryCodes={onRecoveryCodes}
            />
          )}{" "}
          {view.startsWith("plugin:") && (
            <PluginWorkspace
              key={view}
              {...props}
              pluginId={view.split(":")[1]}
              menuId={view.split(":")[2]}
            />
          )}
        </main>
        <footer className="page-footer">
          <span>ModuDock Go</span>
          <a href="/third-party-notices.txt" target="_blank" rel="noreferrer">
            오픈소스 라이선스
          </a>
        </footer>
      </div>
    </div>
  );
}
