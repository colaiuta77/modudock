// 스키마 폼의 숫자 변환과 비밀 설정 보존을 검증한다.
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { expect, it } from "vitest";
import { SchemaForm } from "./SchemaForm";

it("필수 boolean/object/array의 표시 기본값을 편집 없이 제출한다", async () => {
  let result: unknown;
  render(
    <SchemaForm
      schema={{
        type: "object",
        required: ["enabled", "options", "entries"],
        properties: {
          enabled: { type: "boolean", title: "사용" },
          options: { type: "object", title: "옵션" },
          entries: { type: "array", title: "항목" },
          optional: { type: "boolean", title: "선택 사항" },
        },
      }}
      onSubmit={async (value) => {
        result = value;
      }}
    />,
  );
  expect((screen.getByLabelText("사용") as HTMLInputElement).checked).toBe(
    false,
  );
  expect((screen.getByLabelText("옵션") as HTMLTextAreaElement).value).toBe(
    "{}",
  );
  expect((screen.getByLabelText("항목") as HTMLTextAreaElement).value).toBe(
    "[]",
  );
  await userEvent.click(screen.getByRole("button", { name: "저장" }));
  expect(result).toEqual({ enabled: false, options: {}, entries: [] });
});

it("필수 필드의 명시된 default와 initial을 표시하고 그대로 제출한다", async () => {
  let result: unknown;
  render(
    <SchemaForm
      schema={{
        required: ["enabled", "options", "entries"],
        properties: {
          enabled: { type: "boolean", title: "사용", default: true },
          options: { type: "object", title: "옵션", default: { count: 2 } },
          entries: { type: "array", title: "항목", default: ["default"] },
        },
      }}
      initial={{ entries: ["saved"] }}
      onSubmit={async (value) => {
        result = value;
      }}
    />,
  );
  expect((screen.getByLabelText("사용") as HTMLInputElement).checked).toBe(
    true,
  );
  expect(
    JSON.parse((screen.getByLabelText("옵션") as HTMLTextAreaElement).value),
  ).toEqual({ count: 2 });
  expect(
    JSON.parse((screen.getByLabelText("항목") as HTMLTextAreaElement).value),
  ).toEqual(["saved"]);
  await userEvent.click(screen.getByRole("button", { name: "저장" }));
  expect(result).toEqual({
    enabled: true,
    options: { count: 2 },
    entries: ["saved"],
  });
});

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

it("저장된 필수 비밀값을 편집 후 지워도 기존값 유지로 제출한다", async () => {
  let result: unknown;
  render(
    <SchemaForm
      schema={{
        required: ["secret"],
        properties: {
          secret: { type: "string", title: "API 키", writeOnly: true },
        },
      }}
      initial={{ secret: "********" }}
      onSubmit={async (value) => {
        result = value;
      }}
    />,
  );
  const input = screen.getByLabelText("API 키") as HTMLInputElement;
  await userEvent.type(input, "replacement");
  await userEvent.clear(input);
  expect(input.form?.checkValidity()).toBe(true);
  await userEvent.click(screen.getByRole("button", { name: "저장" }));
  expect(result).toEqual({ secret: "" });
});

it("신규 필수 비밀값의 빈 입력은 제출하지 않는다", async () => {
  let sent = false;
  render(
    <SchemaForm
      schema={{
        required: ["secret"],
        properties: {
          secret: { type: "string", title: "API 키", format: "password" },
        },
      }}
      onSubmit={async () => {
        sent = true;
      }}
    />,
  );
  const input = screen.getByLabelText("API 키") as HTMLInputElement;
  expect(input.form?.checkValidity()).toBe(false);
  await userEvent.click(screen.getByRole("button", { name: "저장" }));
  expect(sent).toBe(false);
});
