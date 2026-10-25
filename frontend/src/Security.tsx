// 계정별 TOTP와 Passkey 관리 및 로그아웃 후 복구 코드 일회 표시를 제공한다.
import { useLayoutEffect, useRef, useState, type FormEvent } from "react";
import { Copy, Download, Fingerprint, ShieldCheck, Trash2 } from "lucide-react";
import { APIError, body, id, request as apiRequest } from "./api";
import { ErrorBox, Field, Panel, useLoad, type PageProps } from "./common";
import { Button } from "./components/ui/button";
import { Input } from "./components/ui/input";
import { Badge } from "./components/ui/badge";
import { Card } from "./components/ui/card";
import { PasswordPage } from "./Settings";
import {
  createPasskey,
  getPasskey,
  passkeyBrowserSupport,
  PasskeyCancelled,
} from "./webauthn";

type SecurityStatus = {
  totp_enabled: boolean;
  recovery_codes_remaining: number;
  passkeys: {
    id: string;
    name: string;
    created_at: string;
    last_used_at: string;
  }[];
  passkey_available: boolean;
  passkey_reason: string;
};
type Setup = { secret: string; qr_png: string; expires_at: string };
type Proof = { proof: string; expires_at: string };
type RecoveryResponse = { recovery_codes: string[]; signed_out: true };
export function SecurityPage(
  props: PageProps & {
    onSignedOut: () => void;
    onRecoveryCodes: (codes: string[]) => void;
  },
) {
  const lifecycle = useRef(new AbortController());
  useLayoutEffect(() => {
    const controller = new AbortController();
    lifecycle.current = controller;
    return () => controller.abort();
  }, [props.user.id]);
  function request<T>(path: string, options: RequestInit = {}) {
    return apiRequest<T>(path, {
      ...options,
      signal: lifecycle.current.signal,
    });
  }
  const security = useLoad<SecurityStatus>("/auth/security");
  const [intent, setIntent] = useState("");
  const [proof, setProof] = useState<Proof>();
  const [setup, setSetup] = useState<Setup>();
  const [code, setCode] = useState("");
  const [password, setPassword] = useState("");
  const [factor, setFactor] = useState("");
  const [method, setMethod] = useState<"totp" | "recovery">("totp");
  const [name, setName] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const status = security.data;
  const browserReason = passkeyBrowserSupport();
  const passkeyReason = browserReason || status?.passkey_reason || "";
  function done(codes?: string[]) {
    setSetup(undefined);
    setProof(undefined);
    setPassword("");
    setFactor("");
    setCode("");
    if (codes) props.onRecoveryCodes(codes);
    props.onSignedOut();
  }
  async function run(operation: () => Promise<void>) {
    const signal = lifecycle.current.signal;
    setBusy(true);
    setError("");
    try {
      await operation();
    } catch (e) {
      if (!signal.aborted && !(e instanceof PasskeyCancelled))
        setError(e instanceof Error ? e.message : "처리하지 못했습니다.");
    } finally {
      if (!signal.aborted) setBusy(false);
    }
  }
  function ask(next: string) {
    setError("");
    setPassword("");
    setFactor("");
    setIntent(next);
  }
  function purpose() {
    return intent === "confirm" ? "totp-setup" : intent;
  }
  async function confirm(value: Proof) {
    try {
      const result = await request<RecoveryResponse>("/auth/totp/confirm", {
        method: "POST",
        body: body({ proof: value.proof, code }),
      });
      done(result.recovery_codes);
    } catch (e) {
      if (e instanceof APIError && e.code === "reauth_required") {
        ask("confirm");
        setError(
          "재인증이 만료되었습니다. 다시 인증하면 현재 QR로 계속할 수 있습니다.",
        );
        return;
      }
      throw e;
    }
  }
  async function performIntent(value: Proof) {
    const next = intent;
    setProof(value);
    setPassword("");
    setFactor("");
    setIntent("");
    if (next === "totp-setup") {
      setSetup(
        await request<Setup>("/auth/totp/setup", {
          method: "POST",
          body: body({ proof: value.proof }),
        }),
      );
      setCode("");
    } else if (next === "confirm") {
      await confirm(value);
    } else if (next === "totp-disable") {
      await request("/auth/totp", {
        method: "DELETE",
        body: body({ proof: value.proof }),
      });
      done();
    } else if (next === "recovery-regenerate") {
      const result = await request<RecoveryResponse>("/auth/recovery-codes", {
        method: "POST",
        body: body({ proof: value.proof }),
      });
      done(result.recovery_codes);
    } else if (next === "passkey-register") {
      const options = await request<unknown>("/auth/passkeys/register/begin", {
        method: "POST",
        body: body({ proof: value.proof }),
      });
      const signal = lifecycle.current.signal;
      const credential = await createPasskey(options, signal);
      signal.throwIfAborted();
      await request("/auth/passkeys/register/finish", {
        method: "POST",
        body: body({ proof: value.proof, name, credential }),
      });
      done();
    } else if (next.startsWith("passkey-delete:")) {
      await request(
        `/auth/passkeys/${id(next.slice("passkey-delete:".length))}`,
        { method: "DELETE", body: body({ proof: value.proof }) },
      );
      done();
    }
  }
  async function passwordReauth(e: FormEvent) {
    e.preventDefault();
    await run(async () => {
      const value = await request<Proof>("/auth/reauth", {
        method: "POST",
        body: body({
          purpose: purpose(),
          password,
          method: status?.totp_enabled ? method : undefined,
          code: status?.totp_enabled ? factor : undefined,
        }),
      });
      await performIntent(value);
    });
  }
  async function passkeyReauth() {
    await run(async () => {
      const options = await request<unknown>("/auth/passkeys/reauth/begin", {
        method: "POST",
        body: body({ purpose: purpose() }),
      });
      const signal = lifecycle.current.signal;
      const credential = await getPasskey(options, signal);
      signal.throwIfAborted();
      const value = await request<Proof>("/auth/passkeys/reauth/finish", {
        method: "POST",
        body: body({ credential }),
      });
      await performIntent(value);
    });
  }
  return (
    <div className="stack security-page">
      <p className="muted">
        내 계정의 로그인 수단을 관리합니다. TOTP 변경·복구 코드 재발급·Passkey
        등록/삭제 시 모든 웹 세션이 종료됩니다. API 토큰은 유지되며 별도 폐기가
        필요합니다.
      </p>
      <ErrorBox message={security.error || error} />
      {intent && (
        <Panel
          title="보안 변경 전 재인증"
          description="현재 계정의 인증 수단으로 본인임을 확인하세요. 인증은 5분 동안 유효합니다."
        >
          <form className="stack narrow-form" onSubmit={passwordReauth}>
            <Field label="재인증 비밀번호">
              <Input
                type="password"
                autoComplete="current-password"
                autoFocus
                required
                maxLength={1024}
                value={password}
                onChange={(e) => setPassword(e.target.value)}
              />
            </Field>
            {status?.totp_enabled && (
              <>
                <Field label="추가 인증 방법">
                  <select
                    value={method}
                    onChange={(e) => {
                      setMethod(e.target.value as "totp" | "recovery");
                      setFactor("");
                    }}
                  >
                    <option value="totp">인증 앱</option>
                    <option value="recovery">복구 코드</option>
                  </select>
                </Field>
                <Field
                  label={method === "totp" ? "재인증 코드" : "재인증 복구 코드"}
                >
                  <Input
                    required
                    autoComplete="one-time-code"
                    inputMode={method === "totp" ? "numeric" : "text"}
                    pattern={method === "totp" ? "[0-9]{6}" : undefined}
                    maxLength={method === "totp" ? 6 : 40}
                    value={factor}
                    onChange={(e) => setFactor(e.target.value)}
                  />
                </Field>
              </>
            )}
            <div className="security-actions">
              <Button disabled={busy}>인증 후 계속</Button>
              <Button
                type="button"
                variant="outline"
                disabled={busy}
                onClick={() => {
                  setIntent("");
                  setPassword("");
                  setFactor("");
                }}
              >
                취소
              </Button>
            </div>
          </form>
          {!!status?.passkeys.length && (
            <Button
              variant="outline"
              disabled={busy || !!passkeyReason || !status.passkey_available}
              onClick={() => void passkeyReauth()}
            >
              <Fingerprint />
              Passkey로 재인증
            </Button>
          )}
        </Panel>
      )}
      <Panel
        title="2단계 인증"
        description="TOTP 인증 앱으로 비밀번호 로그인을 보호하세요."
        actions={
          <Badge>{status?.totp_enabled ? "활성화됨" : "비활성화됨"}</Badge>
        }
      >
        {setup ? (
          <div className="stack narrow-form">
            <p>인증 앱으로 QR 코드를 스캔하거나 시크릿 키를 직접 입력하세요.</p>
            <img
              className="totp-qr"
              src={setup.qr_png}
              alt="인증 앱 등록용 QR 코드"
              width="300"
              height="300"
            />
            <Field label="시크릿 키">
              <Input
                className="secret-value"
                value={setup.secret}
                readOnly
                spellCheck={false}
              />
            </Field>
            <Button
              variant="outline"
              disabled={busy}
              onClick={() =>
                void run(async () => {
                  await navigator.clipboard.writeText(setup.secret);
                  props.notify("시크릿 키를 복사했습니다.");
                })
              }
            >
              <Copy />
              시크릿 키 복사
            </Button>
            <p className="muted small">
              등록 대기는 10분 후 만료됩니다. 이 키와 QR 코드를 다른 사람에게
              공유하지 마세요.
            </p>
            <form
              className="stack"
              onSubmit={(e) => {
                e.preventDefault();
                if (!proof || Date.parse(proof.expires_at) <= Date.now()) {
                  ask("confirm");
                  return;
                }
                void run(() => confirm(proof));
              }}
            >
              <Field label="인증 코드">
                <Input
                  required
                  inputMode="numeric"
                  autoComplete="one-time-code"
                  pattern="[0-9]{6}"
                  maxLength={6}
                  placeholder="6자리 코드 입력"
                  value={code}
                  onChange={(e) => setCode(e.target.value)}
                />
              </Field>
              <div className="security-actions">
                <Button disabled={busy || !!intent}>인증 및 활성화</Button>
                <Button
                  type="button"
                  variant="outline"
                  disabled={busy}
                  onClick={() =>
                    void run(async () => {
                      await request("/auth/totp/setup", { method: "DELETE" });
                      setSetup(undefined);
                      setProof(undefined);
                      setCode("");
                      setIntent("");
                    })
                  }
                >
                  등록 취소
                </Button>
              </div>
            </form>
          </div>
        ) : (
          <div className="security-actions">
            <Button
              disabled={!status || busy || !!intent}
              onClick={() => ask("totp-setup")}
            >
              <ShieldCheck />
              {status?.totp_enabled ? "2FA 재설정" : "2FA 설정"}
            </Button>
            {status?.totp_enabled && (
              <Button
                variant="destructive"
                disabled={busy || !!intent}
                onClick={() => {
                  if (
                    window.confirm(
                      "2단계 인증을 해제하면 복구 코드도 삭제됩니다. 계속할까요?",
                    )
                  )
                    ask("totp-disable");
                }}
              >
                2FA 비활성화
              </Button>
            )}
          </div>
        )}
      </Panel>
      <Panel
        title="복구 코드"
        description="인증 앱을 사용할 수 없을 때 비밀번호와 함께 사용하는 일회용 코드입니다."
      >
        <p>복구 코드 {status?.recovery_codes_remaining || 0}개 남음</p>
        <p className="muted small">
          코드는 발급 직후 한 번만 표시됩니다. 재생성하면 기존 코드는 모두
          무효화됩니다.
        </p>
        <Button
          disabled={!status?.totp_enabled || busy || !!intent}
          onClick={() => {
            if (
              window.confirm(
                "기존 복구 코드가 모두 무효화되고 모든 기기에서 로그아웃됩니다. 새로 발급할까요?",
              )
            )
              ask("recovery-regenerate");
          }}
        >
          복구 코드 재생성
        </Button>
      </Panel>
      <Panel
        title="Passkey"
        description="기기의 지문·얼굴 인식 또는 보안 키로 비밀번호 없이 로그인합니다."
      >
        {!status?.totp_enabled && (
          <p className="security-hint">
            Passkey를 등록해도 비밀번호 로그인은 유지됩니다. 비밀번호 경로도
            보호하려면 2단계 인증을 설정하세요.
          </p>
        )}
        {passkeyReason && <p className="muted">{passkeyReason}</p>}
        <form
          className="security-actions"
          onSubmit={(e) => {
            e.preventDefault();
            ask("passkey-register");
          }}
        >
          <Field label="새 Passkey 이름">
            <Input
              required
              maxLength={100}
              value={name}
              onChange={(e) => setName(e.target.value)}
              placeholder="예. 내 휴대전화"
            />
          </Field>
          <Button
            disabled={
              busy ||
              !!intent ||
              !!passkeyReason ||
              !status?.passkey_available ||
              status.passkeys.length >= 10
            }
          >
            <Fingerprint />
            Passkey 추가
          </Button>
        </form>
        <div className="stack">
          {status?.passkeys.map((key) => (
            <form
              className="passkey-row"
              key={key.id}
              onSubmit={(e) => {
                e.preventDefault();
                const fields = new FormData(e.currentTarget);
                void run(async () => {
                  await request(`/auth/passkeys/${id(key.id)}`, {
                    method: "PUT",
                    body: body({ name: fields.get("name") }),
                  });
                  security.reload();
                });
              }}
            >
              <div>
                <Field label={`Passkey 이름 · ${key.name}`}>
                  <Input
                    name="name"
                    defaultValue={key.name}
                    required
                    maxLength={100}
                  />
                </Field>
                <p className="muted small">
                  등록 {new Date(key.created_at).toLocaleDateString()} · 최근
                  사용{" "}
                  {key.last_used_at
                    ? new Date(key.last_used_at).toLocaleString()
                    : "아직 없음"}
                </p>
              </div>
              <div className="security-actions">
                <Button variant="outline" disabled={busy}>
                  이름 저장
                </Button>
                <Button
                  type="button"
                  variant="outline"
                  disabled={busy || !!intent}
                  aria-label={`${key.name} 삭제`}
                  onClick={() => {
                    if (
                      window.confirm(
                        "이 Passkey를 삭제하고 모든 기기에서 로그아웃할까요?",
                      )
                    )
                      ask(`passkey-delete:${key.id}`);
                  }}
                >
                  <Trash2 />
                  삭제
                </Button>
              </div>
            </form>
          ))}
        </div>
      </Panel>
      <PasswordPage {...props} />
    </div>
  );
}

export function RecoveryCodes({
  codes,
  onClose,
}: {
  codes: string[];
  onClose: () => void;
}) {
  const [saved, setSaved] = useState(false);
  const [error, setError] = useState("");
  async function copy() {
    try {
      await navigator.clipboard.writeText(codes.join("\n"));
    } catch {
      setError("복사하지 못했습니다. 다운로드하거나 직접 기록해 주세요.");
    }
  }
  function download() {
    const url = URL.createObjectURL(
      new Blob(
        [
          "ModuDock 복구 코드 — 각각 한 번만 사용할 수 있습니다.\n\n" +
            codes.join("\n"),
        ],
        { type: "text/plain;charset=utf-8" },
      ),
    );
    const link = document.createElement("a");
    link.href = url;
    link.download = "modudock-recovery-codes.txt";
    link.click();
    setTimeout(() => URL.revokeObjectURL(url), 0);
  }
  return (
    <main className="recovery-page">
      <Card className="panel">
        <ShieldCheck size={32} />
        <h1>복구 코드를 안전하게 보관하세요.</h1>
        <p>
          보안 설정이 변경되어 모든 기기에서 로그아웃했습니다. 이 코드는 다시
          조회할 수 없습니다.
        </p>
        <p className="muted">
          비밀번호 관리자 등 안전한 곳에 저장하세요. 화면을 닫거나 새로고침하면
          사라집니다.
        </p>
        <div className="recovery-grid">
          {codes.map((code) => (
            <code key={code}>{code}</code>
          ))}
        </div>
        <ErrorBox message={error} />
        <div className="security-actions">
          <Button variant="outline" onClick={() => void copy()}>
            <Copy />
            전체 복사
          </Button>
          <Button variant="outline" onClick={download}>
            <Download />
            TXT 다운로드
          </Button>
        </div>
        <label className="security-check">
          <input
            type="checkbox"
            checked={saved}
            onChange={(e) => setSaved(e.target.checked)}
          />
          복구 코드를 안전한 곳에 저장했습니다.
        </label>
        <Button disabled={!saved} onClick={onClose}>
          저장 확인 후 로그인
        </Button>
      </Card>
    </main>
  );
}
