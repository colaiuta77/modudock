// UI 회귀 테스트의 브라우저 저장소와 DOM 상태를 초기화한다.
import { cleanup } from "@testing-library/react";
import { afterEach } from "vitest";

afterEach(() => {
  cleanup();
  sessionStorage.clear();
  localStorage.clear();
});
