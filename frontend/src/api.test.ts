// 실제 API 클라이언트의 CSRF 전달과 오류 응답 처리를 검증한다.
import { afterEach, expect, it, vi } from "vitest";
import { request, signIn } from "./api";

afterEach(() => vi.unstubAllGlobals());

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
