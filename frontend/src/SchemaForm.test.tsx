// 스키마 폼의 숫자 변환과 비밀 설정 보존을 검증한다.
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { expect, it } from "vitest";
import { SchemaForm } from "./SchemaForm";

it("사용자가 편집하지 않은 비밀값을 그대로 보존한다", async () => {
  let result: unknown;
  render(
    <SchemaForm
      schema={{
        type: "object",
        properties: {
          secret: { type: "string", title: "API 키", writeOnly: true },
          count: { type: "integer", title: "횟수" },
        },
      }}
      initial={{ secret: "********", count: 2 }}
      onSubmit={async (v) => {
        result = v;
      }}
    />,
  );
  expect((screen.getByLabelText("API 키") as HTMLInputElement).value).toBe("");
  await userEvent.clear(screen.getByLabelText("횟수"));
  await userEvent.type(screen.getByLabelText("횟수"), "5");
  await userEvent.click(screen.getByRole("button", { name: "저장" }));
  expect(result).toEqual({ secret: "********", count: 5 });
});
it("잘못된 구조화 JSON은 제출하지 않는다", async () => {
  let sent = false;
  render(
    <SchemaForm
      schema={{
        type: "object",
        properties: { options: { type: "object", title: "옵션" } },
      }}
      initial={{ options: {} }}
      onSubmit={async () => {
        sent = true;
      }}
    />,
  );
  await userEvent.clear(screen.getByLabelText("옵션"));
  await userEvent.type(screen.getByLabelText("옵션"), "broken-json");
  await userEvent.click(screen.getByRole("button", { name: "저장" }));
  expect(sent).toBe(false);
});
