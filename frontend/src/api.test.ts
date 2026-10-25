// 실제 API 클라이언트의 CSRF 전달과 오류 응답 처리를 검증한다.
import { afterEach, expect, it, vi } from "vitest";
import {
  request,
  signIn,
  signOut,
  completeMFA,
  acceptSession,
  changeSecurity,
} from "./api";

it("세션 교체 전에 보낸 요청의 늦은 401은 새 세션을 종료하지 않는다", async () => {
  const expired = vi.fn();
  window.addEventListener("modudock:unauthorized", expired);
  let finish!: (response: Response) => void;
  vi.stubGlobal(
    "fetch",
    () =>
      new Promise<Response>((resolve) => {
        finish = resolve;
      }),
  );
  const pending = request("/status").catch((e) => e);
  acceptSession({
    user: {
      id: "u",
      username: "admin",
      role: "admin",
      is_active: true,
      created_at: "",
    },
    csrf_token: "rotated-csrf",
  });
  finish(
    Response.json(
      { error: { code: "unauthorized", message: "expired" } },
      { status: 401 },
    ),
  );
  await pending;
  expect(expired).not.toHaveBeenCalled();
  let sentToken: string | null = null;
  vi.stubGlobal("fetch", async (_path: string, init?: RequestInit) => {
    sentToken = new Headers(init?.headers).get("X-CSRF-Token");
    return new Response(null, { status: 204 });
  });
  await request("/probe", { method: "POST" });
  expect(sentToken).toBe("rotated-csrf");
  window.removeEventListener("modudock:unauthorized", expired);
});

afterEach(() => vi.unstubAllGlobals());

it.each([200, 500, 401])(
  "보안 변경 응답(%s)보다 먼저 온 배경 401의 처리를 변경 완료까지 기다린다",
  async (status) => {
    let finishRotation!: (response: Response) => void;
    let finishRefresh!: (response: Response) => void;
    const expired = vi.fn();
    window.addEventListener("modudock:unauthorized", expired);
    acceptSession({
      user: {
        id: "u",
        username: "admin",
        role: "admin",
        is_active: true,
        created_at: "",
      },
      csrf_token: "before",
    });
    vi.stubGlobal(
      "fetch",
      (path: string) =>
        new Promise<Response>((resolve) => {
          if (path.endsWith("/confirm")) finishRotation = resolve;
          else finishRefresh = resolve;
        }),
    );
    const rotation = changeSecurity("/auth/totp/confirm", {
      method: "POST",
    }).catch((e) => e);
    const refresh = request("/status").catch((e) => e);
    finishRefresh(
      Response.json(
        { error: { code: "unauthorized", message: "old session" } },
        { status: 401 },
      ),
    );
    await new Promise((resolve) => setTimeout(resolve, 0));
    const earlyEvents = expired.mock.calls.length;
    finishRotation(
      status === 200
        ? Response.json({
            user: {
              id: "u",
              username: "admin",
              role: "admin",
              is_active: true,
              created_at: "",
            },
            csrf_token: "after",
          })
        : Response.json(
            { error: { code: "unauthorized", message: "rotation failed" } },
            { status },
          ),
    );
    await Promise.all([rotation, refresh]);
    window.removeEventListener("modudock:unauthorized", expired);
    expect(earlyEvents).toBe(0);
    if (status === 200) expect(expired).not.toHaveBeenCalled();
    else expect(expired).toHaveBeenCalled();
  },
);

it("취소한 요청의 늦은 401은 새 로그인 상태를 만료시키지 않는다", async () => {
  const expired = vi.fn();
  window.addEventListener("modudock:unauthorized", expired);
  const controller = new AbortController();
  let finish!: (value: Response) => void;
  vi.stubGlobal(
    "fetch",
    () =>
      new Promise<Response>((resolve) => {
        finish = resolve;
      }),
  );
  const result = request("/auth/security", { signal: controller.signal }).catch(
    (e) => e,
  );
  controller.abort();
  finish(
    Response.json(
      { error: { code: "unauthorized", message: "expired" } },
      { status: 401 },
    ),
  );
  expect(await result).toMatchObject({ name: "AbortError" });
  expect(expired).not.toHaveBeenCalled();
  window.removeEventListener("modudock:unauthorized", expired);
});

it("MFA 대기와 코드 오류는 로그인 완료나 세션 만료가 아니다", async () => {
  const expired = vi.fn();
  window.addEventListener("modudock:unauthorized", expired);
  vi.stubGlobal("fetch", async (path: string) =>
    path.endsWith("/login")
      ? Response.json(
          { mfa_required: true, expires_at: "2099-01-01T00:00:00Z" },
          { status: 202 },
        )
      : Response.json(
          { error: { code: "invalid_credentials", message: "잘못된 코드" } },
          { status: 401 },
        ),
  );
  expect(await signIn("admin", "password")).toHaveProperty(
    "mfa_required",
    true,
  );
  await expect(completeMFA("totp", "000000")).rejects.toThrow("잘못된 코드");
  expect(expired).not.toHaveBeenCalled();
  window.removeEventListener("modudock:unauthorized", expired);
});

it("로그인에서 받은 CSRF 토큰을 변경 요청에 전달한다", async () => {
  const calls: Array<{ path: string; init?: RequestInit }> = [];
  vi.stubGlobal("fetch", async (path: string, init?: RequestInit) => {
    calls.push({ path, init });
    return Response.json(
      path.endsWith("login")
        ? {
            user: {
              id: "u",
              username: "admin",
              role: "admin",
              is_active: true,
            },
            csrf_token: "csrf-from-server",
          }
        : { id: "new" },
    );
  });
  await signIn("admin", "a-long-test-password");
  const result = await request<{ id: string }>("/users", {
    method: "POST",
    body: JSON.stringify({ username: "operator" }),
  });
  expect(result.id).toBe("new");
  expect(new Headers(calls[1].init?.headers).get("X-CSRF-Token")).toBe(
    "csrf-from-server",
  );
  expect(calls[1].init?.credentials).toBe("same-origin");
  expect(sessionStorage.length + localStorage.length).toBe(0);
});

it("서버 오류의 설명을 호출자에게 전달한다", async () => {
  vi.stubGlobal("fetch", async () =>
    Response.json(
      { error: { code: "forbidden", message: "권한이 없습니다." } },
      { status: 403 },
    ),
  );
  await expect(request("/users")).rejects.toThrow("권한이 없습니다.");
});

it("본문 없는 삭제 응답을 처리한다", async () => {
  vi.stubGlobal("fetch", async () => new Response(null, { status: 204 }));
  await expect(
    request("/tokens/t", { method: "DELETE" }),
  ).resolves.toBeUndefined();
});

it.each(["/users", "/auth/logout"])(
  "%s의 401 뒤 CSRF 토큰을 재사용하지 않는다",
  async (path) => {
    let sentToken: string | null = null;
    vi.stubGlobal("fetch", async (url: string, init?: RequestInit) => {
      if (url.endsWith("/auth/login"))
        return Response.json({
          user: {
            id: "u",
            username: "admin",
            role: "admin",
            is_active: true,
            created_at: "2026-10-23T00:00:00Z",
          },
          csrf_token: "old-csrf",
        });
      if (url.endsWith(path))
        return Response.json(
          { error: { code: "unauthorized", message: "세션 만료" } },
          { status: 401 },
        );
      sentToken = new Headers(init?.headers).get("X-CSRF-Token");
      return new Response(null, { status: 204 });
    });
    await signIn("admin", "password");
    if (path === "/auth/logout") await signOut().catch(() => {});
    else await request(path).catch(() => {});
    await request("/tokens/t", { method: "DELETE" });
    expect(sentToken).toBeNull();
  },
);
