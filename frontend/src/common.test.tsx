// 수동 새로고침이 현재 페이지 데이터를 다시 읽는지 검증한다.
import { act, renderHook, waitFor } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { useLoad } from "./common";

afterEach(() => vi.unstubAllGlobals());
it("전체 새로고침 이벤트로 기존 폼을 해제하지 않고 데이터를 다시 조회한다", async () => {
  let requests = 0;
  vi.stubGlobal("fetch", async () => Response.json({ count: ++requests }));
  const { result } = renderHook(() => useLoad<{ count: number }>("/users"));
  await waitFor(() => expect(result.current.data?.count).toBe(1));
  act(() => window.dispatchEvent(new Event("modudock:refresh")));
  await waitFor(() => expect(result.current.data?.count).toBe(2));
});
