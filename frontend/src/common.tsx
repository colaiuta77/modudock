// 관리 페이지의 조회 상태와 폼·표 공통 요소를 제공한다.
import { useEffect, useState, type ReactNode } from "react";
import {
  AlertCircle,
  ChevronLeft,
  ChevronRight,
  RefreshCw,
} from "lucide-react";
import { request, type User } from "./api";
import { Button } from "./components/ui/button";
import { Card } from "./components/ui/card";
import { Badge } from "./components/ui/badge";

export type Notify = (message: string, error?: boolean) => void;
export type PageProps = { user: User; notify: Notify; changed: () => void };
export function useLoad<T>(path: string, revision = 0) {
  const [data, setData] = useState<T>();
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(true);
  const [counter, setCounter] = useState(0);
  useEffect(() => {
    const refresh = () => setCounter((n) => n + 1);
    window.addEventListener("modudock:refresh", refresh);
    return () => window.removeEventListener("modudock:refresh", refresh);
  }, []);
  useEffect(() => {
    const controller = new AbortController();
    setLoading(true);
    setError("");
    request<T>(path, { signal: controller.signal })
      .then(setData)
      .catch((e) => {
        if (!controller.signal.aborted) setError(e.message);
      })
      .finally(() => {
        if (!controller.signal.aborted) setLoading(false);
      });
    return () => controller.abort();
  }, [path, revision, counter]);
  return { data, error, loading, reload: () => setCounter((n) => n + 1) };
}
export function useAction(notify: Notify) {
  const [busy, setBusy] = useState(false);
  async function perform(
    action: () => Promise<unknown>,
    message = "저장했습니다.",
  ) {
    setBusy(true);
    try {
      await action();
      if (message) notify(message);
      return true;
    } catch (e) {
      notify(e instanceof Error ? e.message : "처리하지 못했습니다.", true);
      return false;
    } finally {
      setBusy(false);
    }
  }
  return { busy, perform };
}
export function Panel({
  title,
  description,
  actions,
  children,
}: {
  title: string;
  description?: string;
  actions?: ReactNode;
  children: ReactNode;
}) {
  return (
    <Card className="panel">
      <div className="panel-heading">
        <div>
          <h2>{title}</h2>
          {description && <p className="muted">{description}</p>}
        </div>
        {actions}
      </div>
      {children}
    </Card>
  );
}
export function Field({
  label,
  children,
  hint,
}: {
  label: string;
  children: ReactNode;
  hint?: string;
}) {
  return (
    <label className="field">
      <span>{label}</span>
      {children}
      {hint && <small>{hint}</small>}
    </label>
  );
}
export function ErrorBox({ message }: { message: string }) {
  return message ? (
    <div className="error-box" role="alert">
      <AlertCircle size={18} />
      {message}
    </div>
  ) : null;
}
export function Empty({
  children = "표시할 내용이 없습니다.",
}: {
  children?: ReactNode;
}) {
  return <p className="empty">{children}</p>;
}
export function Reload({
  onClick,
  loading = false,
}: {
  onClick: () => void;
  loading?: boolean;
}) {
  return (
    <Button variant="outline" onClick={onClick} disabled={loading}>
      <RefreshCw size={16} className={loading ? "spin" : ""} />
      새로고침
    </Button>
  );
}
export function Pagination({
  offset,
  total,
  onChange,
}: {
  offset: number;
  total: number;
  onChange: (n: number) => void;
}) {
  return (
    <div className="pagination">
      <span>전체 {total}개</span>
      <Button
        variant="outline"
        size="sm"
        aria-label="이전 페이지"
        disabled={offset === 0}
        onClick={() => onChange(Math.max(0, offset - 50))}
      >
        <ChevronLeft />
      </Button>
      <span>{Math.floor(offset / 50) + 1}</span>
      <Button
        variant="outline"
        size="sm"
        aria-label="다음 페이지"
        disabled={offset + 50 >= total}
        onClick={() => onChange(offset + 50)}
      >
        <ChevronRight />
      </Button>
    </div>
  );
}
const states: Record<string, string> = {
  queued: "대기",
  running: "실행 중",
  succeeded: "완료",
  failed: "실패",
  cancelled: "취소",
  interrupted: "중단",
  active: "활성",
  disabled: "비활성",
};
export function State({ value }: { value: string }) {
  return (
    <Badge variant="outline" className={`state state-${value}`}>
      {states[value] || value}
    </Badge>
  );
}
export function date(value: string | null | undefined) {
  return value ? new Date(value).toLocaleString("ko-KR") : "—";
}
export function JSONView({ value }: { value: unknown }) {
  return <pre className="json-view">{JSON.stringify(value, null, 2)}</pre>;
}
