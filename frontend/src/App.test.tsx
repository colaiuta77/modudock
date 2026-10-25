// 관리 콘솔의 로그인과 역할별 메뉴 및 반응형 내비게이션을 검증한다.
import {
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import App from "./App";

const admin = {
  id: "u",
  username: "admin",
  role: "admin",
  is_active: true,
  created_at: "2026-10-23T00:00:00Z",
};
beforeEach(() => {
  vi.stubGlobal(
    "EventSource",
    class {
      addEventListener() {}
      close() {}
    },
  );
  vi.stubGlobal("matchMedia", () => ({
    matches: true,
    addEventListener() {},
    removeEventListener() {},
  }));
});
afterEach(() => vi.unstubAllGlobals());

it("MFA 로그인 대기에서는 콘솔을 열지 않고 코드를 요청한다", async () => {
  vi.stubGlobal("fetch", async (path: string) =>
    path.endsWith("/auth/login")
      ? Response.json(
          { mfa_required: true, expires_at: "2099-01-01T00:00:00Z" },
          { status: 202 },
        )
      : path.endsWith("/auth/options")
        ? Response.json({ passkey_available: false, passkey_reason: "" })
        : Response.json(
            { error: { code: "unauthorized", message: "로그인 필요" } },
            { status: 401 },
          ),
  );
  render(<App />);
  await userEvent.type(await screen.findByLabelText("사용자 이름"), "admin");
  await userEvent.type(screen.getByLabelText("비밀번호"), "password");
  await userEvent.click(screen.getByRole("button", { name: "로그인" }));
  expect(await screen.findByLabelText("인증 코드")).toBeTruthy();
  expect(screen.queryByRole("navigation")).toBeNull();
  expect(screen.queryByLabelText("비밀번호")).toBeNull();
});

function serve(
  role = "admin",
  reply?: (path: string, init?: RequestInit) => Response | undefined,
) {
  vi.stubGlobal("fetch", async (path: string, init?: RequestInit) => {
    const response = reply?.(path, init);
    if (response) return response;
    if (path.endsWith("/auth/me"))
      return Response.json({ user: { ...admin, role }, csrf_token: "csrf" });
    if (path.endsWith("/status"))
      return Response.json({
        version: "dev",
        started_at: "2026-10-23T00:00:00Z",
        plugins_active: 0,
        runs_queued: 0,
        runs_running: 0,
        runs_succeeded: 0,
        runs_failed: 0,
      });
    return Response.json({ items: [], total: 0 });
  });
}
it("보안 변경 후 설정 화면에서 복구 코드를 저장하고 로그인을 유지한다", async () => {
  serve("admin", (path) =>
    path.endsWith("/auth/security")
      ? Response.json({
          totp_enabled: false,
          recovery_codes_remaining: 0,
          passkeys: [],
          passkey_available: false,
          passkey_reason: "",
        })
      : path.endsWith("/auth/reauth")
        ? Response.json({ proof: "p", expires_at: "2099-01-01T00:00:00Z" })
        : path.endsWith("/auth/totp/setup")
          ? Response.json({
              secret: "ABCDEF",
              qr_png: "data:image/png;base64,test",
              expires_at: "2099-01-01T00:00:00Z",
            })
          : path.endsWith("/auth/totp/confirm")
            ? Response.json({
                recovery_codes: ["ONE-TIME-RECOVERY"],
                csrf_token: "rotated-csrf",
                user: admin,
              })
            : undefined,
  );
  render(<App />);
  await userEvent.click(
    await screen.findByRole("button", { name: "계정 보안" }),
  );
  await userEvent.click(
    await screen.findByRole("button", { name: "2FA 설정" }),
  );
  await userEvent.type(
    screen.getByLabelText("재인증 비밀번호"),
    "test-password",
  );
  await userEvent.click(screen.getByRole("button", { name: "인증 후 계속" }));
  await userEvent.type(await screen.findByLabelText("인증 코드"), "012345");
  await userEvent.click(screen.getByRole("button", { name: "인증 및 활성화" }));
  expect(await screen.findByText("ONE-TIME-RECOVERY")).toBeTruthy();
  expect(screen.getByRole("navigation", { name: "주 내비게이션" })).toBeTruthy();
  expect(localStorage.getItem("recovery_codes")).toBeNull();
  await userEvent.click(
    screen.getByLabelText("복구 코드를 안전한 곳에 저장했습니다."),
  );
  await userEvent.click(
    screen.getByRole("button", { name: "저장 확인 후 닫기" }),
  );
  expect(screen.getByRole("heading", { name: "계정 보안" })).toBeTruthy();
  expect(screen.queryByText("ONE-TIME-RECOVERY")).toBeNull();
});
it("사이드바 접기 상태를 재접속에도 보존한다", async () => {
  serve();
  const view = render(<App />);
  await userEvent.click(
    await screen.findByRole("button", { name: "사이드바 접기" }),
  );
  expect(localStorage.getItem("modudock-go-sidebar-collapsed")).toBe("true");
  view.unmount();
  render(<App />);
  expect(
    await screen.findByRole("button", { name: "사이드바 펼치기" }),
  ).toBeTruthy();
});
it("조회자에게 관리 기능을 노출하지 않는다", async () => {
  serve("viewer");
  render(<App />);
  await screen.findByRole("heading", { name: "개요" });
  expect(screen.queryByRole("button", { name: "사용자 계정" })).toBeNull();
  expect(screen.queryByRole("button", { name: "API 토큰" })).toBeNull();
  expect(
    within(screen.getByRole("navigation", { name: "주 내비게이션" })).getByRole(
      "button",
      { name: "작업 이력" },
    ),
  ).toBeTruthy();
});
it("로그인 실패 사유를 표시한다", async () => {
  vi.stubGlobal("fetch", async (path: string) =>
    Response.json(
      {
        error: {
          code: "unauthorized",
          message: path.endsWith("login")
            ? "계정 정보를 확인해 주세요."
            : "로그인 필요",
        },
      },
      { status: 401 },
    ),
  );
  render(<App />);
  await userEvent.type(await screen.findByLabelText("사용자 이름"), "wrong");
  await userEvent.type(screen.getByLabelText("비밀번호"), "password");
  await userEvent.click(screen.getByRole("button", { name: "로그인" }));
  expect(await screen.findByRole("alert")).toHaveProperty(
    "textContent",
    "계정 정보를 확인해 주세요.",
  );
});
it("모바일 메뉴에서 이동하면 서랍이 닫힌다", async () => {
  serve();
  render(<App />);
  await screen.findByRole("heading", { name: "개요" });
  fireEvent.click(screen.getByRole("button", { name: "메뉴 열기" }));
  const dialog = await screen.findByRole("dialog");
  const history = [...dialog.querySelectorAll("button")].find((b) =>
    b.textContent?.includes("작업 이력"),
  )!;
  fireEvent.click(history);
  await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
  expect(
    screen.getByRole("heading", { name: "작업 이력", level: 1 }),
  ).toBeTruthy();
});

it.each(["admin", "operator"])(
  "%s는 메뉴 없는 플러그인 목록에서 작업 공간에 들어가 실행한다",
  async (role) => {
    const plugin = {
      id: "tasks-only",
      name: "작업 전용",
      version: "1.0.0",
      state: "active",
      repository_url: "https://github.com/example/tasks-only",
      active_release_id: "release-1",
      manifest: {
        id: "tasks-only",
        name: "작업 전용",
        version: "1.0.0",
        data_schema_version: 1,
        tasks: [
          {
            id: "sync",
            title: "동기화",
            input_schema: { type: "object" },
            timeout_seconds: 60,
            idempotent: true,
          },
        ],
      },
    };
    let submitted: unknown;
    serve(role, (path, init) => {
      if (path.includes("/plugins?"))
        return Response.json({ items: [plugin], total: 1 });
      if (path.endsWith("/plugins/tasks-only")) return Response.json(plugin);
      if (path.endsWith("/tasks/sync/runs")) {
        submitted = JSON.parse(String(init?.body));
        return Response.json({
          id: "run-1",
          plugin_id: plugin.id,
          release_id: "release-1",
          task_id: "sync",
          state: "queued",
          progress: 0,
          created_at: "2026-10-23T00:00:00Z",
          started_at: null,
          finished_at: null,
        });
      }
    });
    render(<App />);
    const nav = await screen.findByRole("navigation", {
      name: "주 내비게이션",
    });
    await userEvent.click(
      within(nav).getByRole("button", { name: "플러그인" }),
    );
    if (role === "operator") {
      expect(
        screen.queryByRole("heading", { name: "GitHub에서 설치" }),
      ).toBeNull();
      expect(screen.queryByRole("button", { name: "관리" })).toBeNull();
      expect(screen.queryByRole("button", { name: "비활성화" })).toBeNull();
    }
    await userEvent.click(
      await screen.findByRole("button", { name: "작업 공간" }),
    );
    await screen.findByRole("heading", { name: "동기화" });
    await userEvent.click(screen.getByRole("button", { name: "작업 실행" }));
    expect(await screen.findByText("run-1")).toBeTruthy();
    expect(submitted).toEqual({ input: {} });
  },
);

it("꺼진 플러그인 메뉴를 내비게이션에 되살리지 않는다", async () => {
  serve("admin", (path) =>
    path.endsWith("/menus")
      ? Response.json({
          items: [
            {
              plugin_id: "p",
              plugin_name: "숨김",
              id: "hidden",
              title: "숨긴 메뉴",
              icon: "puzzle",
              position: 0,
              enabled: false,
            },
          ],
          total: 1,
        })
      : undefined,
  );
  render(<App />);
  await screen.findByRole("heading", { name: "개요" });
  expect(
    within(
      screen.getByRole("navigation", { name: "주 내비게이션" }),
    ).queryByRole("button", { name: "숨긴 메뉴" }),
  ).toBeNull();
});

it("세션 요청이 401이면 다시 로그인할 수 있다", async () => {
  let expired = false;
  serve("admin", (path) =>
    expired && path.endsWith("/status")
      ? Response.json(
          { error: { code: "unauthorized", message: "세션 만료" } },
          { status: 401 },
        )
      : undefined,
  );
  render(<App />);
  await screen.findByRole("heading", { name: "개요" });
  expired = true;
  await userEvent.click(
    within(
      screen
        .getByRole("heading", { name: "개요", level: 1 })
        .closest("header")!,
    ).getByRole("button", { name: "새로고침" }),
  );
  expect(await screen.findByRole("button", { name: "로그인" })).toBeTruthy();
  expect(screen.queryByRole("navigation")).toBeNull();
});

it("권한 거부 403은 로그인 상태를 유지하고 오류를 표시한다", async () => {
  serve("admin", (path) =>
    path.endsWith("/users?offset=0")
      ? Response.json(
          { error: { code: "forbidden", message: "권한이 없습니다." } },
          { status: 403 },
        )
      : undefined,
  );
  render(<App />);
  const nav = await screen.findByRole("navigation", { name: "주 내비게이션" });
  await userEvent.click(
    within(nav).getByRole("button", { name: "사용자 계정" }),
  );
  expect(await screen.findByRole("alert")).toHaveProperty(
    "textContent",
    "권한이 없습니다.",
  );
  expect(screen.queryByRole("button", { name: "로그인" })).toBeNull();
});

it("이미 만료된 세션의 로그아웃 401도 로그인 화면으로 돌아간다", async () => {
  serve("admin", (path) =>
    path.endsWith("/auth/logout")
      ? Response.json(
          { error: { code: "unauthorized", message: "세션 만료" } },
          { status: 401 },
        )
      : undefined,
  );
  render(<App />);
  await userEvent.click(
    await screen.findByRole("button", { name: "로그아웃" }),
  );
  expect(await screen.findByRole("button", { name: "로그인" })).toBeTruthy();
});
