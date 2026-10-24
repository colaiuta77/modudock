// 작업 없는 플러그인과 동적 작업 목록의 스케줄 편집을 검증한다.
import { act, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import type { Plugin } from "./api";
import { SchedulesPage } from "./Operations";

afterEach(() => vi.unstubAllGlobals());

const resourceOnly: Plugin = {
  id: "resources",
  name: "리소스 전용",
  version: "1.0.0",
  state: "active",
  repository_url: "https://github.com/example/resources",
  active_release_id: "release-1",
  manifest: {
    id: "resources",
    name: "리소스 전용",
    version: "1.0.0",
    data_schema_version: 1,
    resources: [{ id: "items", methods: ["GET"] }],
  },
};

async function serve(plugins: Plugin[]) {
  vi.stubGlobal("fetch", async (path: string) =>
    Response.json({
      items: path.includes("/plugins?") ? plugins : [],
      total: path.includes("/plugins?") ? plugins.length : 0,
    }),
  );
  await act(async () => {
    render(
      <SchedulesPage
        user={{
          id: "u",
          username: "admin",
          role: "admin",
          is_active: true,
          created_at: "2026-10-24T00:00:00Z",
        }}
        notify={() => {}}
        changed={() => {}}
      />,
    );
  });
}

it.each<[string, Plugin[]]>([
  ["플러그인 없음", []],
  ["tasks 생략", [resourceOnly]],
  [
    "빈 tasks",
    [{ ...resourceOnly, manifest: { ...resourceOnly.manifest, tasks: [] } }],
  ],
])(
  "%s이면 스케줄 추가를 안내하고 빈 작업을 제출하지 않는다",
  async (_, plugins) => {
    await serve(plugins);
    await userEvent.click(screen.getByRole("button", { name: "스케줄 추가" }));
    expect(
      await screen.findByText("작업이 있는 활성 플러그인이 필요합니다."),
    ).toBeTruthy();
    expect(screen.queryByRole("button", { name: "저장" })).toBeNull();
  },
);

it("리소스 전용은 제외하고 작업 있는 플러그인의 동적 작업을 선택한다", async () => {
  const worker: Plugin = {
    ...resourceOnly,
    id: "worker",
    name: "작업 전용",
    manifest: {
      ...resourceOnly.manifest,
      id: "worker",
      tasks: [
        { id: "sync", title: "동기화" },
        { id: "clean", title: "정리" },
      ],
    },
  };
  await serve([
    resourceOnly,
    worker,
    { ...worker, id: "disabled", state: "disabled" },
  ]);
  await userEvent.click(screen.getByRole("button", { name: "스케줄 추가" }));
  const plugin = await screen.findByRole("combobox", { name: "플러그인" });
  expect((plugin as HTMLSelectElement).value).toBe("worker");
  expect(
    within(plugin).getAllByRole("option").map((o) => o.textContent),
  ).toEqual(["선택", "작업 전용"]);
  const task = screen.getByRole("combobox", { name: "작업" });
  await userEvent.selectOptions(task, "clean");
  expect((task as HTMLSelectElement).value).toBe("clean");
});
