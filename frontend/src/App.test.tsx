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

function serve(role = "admin") {
  vi.stubGlobal("fetch", async (path: string) => {
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
