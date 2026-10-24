// JSON Schema 설정과 작업 입력을 편집하는 폼을 제공한다.
import { useState } from "react";
import type { Schema } from "./api";
import { Field, ErrorBox } from "./common";
import { Input } from "./components/ui/input";
import { Textarea } from "./components/ui/textarea";
import { Button } from "./components/ui/button";

export function SchemaForm({
  schema = {},
  initial = {},
  onSubmit,
  label = "저장",
  disabled = false,
}: {
  schema?: Schema;
  initial?: Record<string, unknown>;
  onSubmit: (value: Record<string, unknown>) => Promise<unknown>;
  label?: string;
  disabled?: boolean;
}) {
  const properties = schema.properties || {};
  const [values, setValues] = useState<Record<string, unknown>>(() =>
    Object.fromEntries([
      ...Object.entries(properties)
        .filter(
          ([key, p]) =>
            schema.required?.includes(key) &&
            !p.enum &&
            ["boolean", "object", "array"].includes(p.type || ""),
        )
        .map(([key, p]) => [
          key,
          p.type === "boolean" ? false : p.type === "array" ? [] : {},
        ]),
      ...Object.entries(properties)
        .filter(([, p]) => p.default !== undefined)
        .map(([k, p]) => [k, p.default]),
      ...Object.entries(initial),
    ]),
  );
  const [drafts, setDrafts] = useState<Record<string, string>>({});
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  return (
    <form
      className="schema-form"
      onSubmit={async (e) => {
        e.preventDefault();
        setError("");
        try {
          const result = { ...values };
          for (const [key, raw] of Object.entries(drafts))
            result[key] = JSON.parse(raw);
          setBusy(true);
          await onSubmit(result);
        } catch (e) {
          setError(e instanceof Error ? e.message : "입력값을 확인해 주세요.");
        } finally {
          setBusy(false);
        }
      }}
    >
      {Object.keys(properties).length === 0 && (
        <p className="muted">추가 입력 항목이 없습니다.</p>
      )}
      <div className="form-grid">
        {Object.entries(properties).map(([key, property]) => {
          const title = property.title || key,
            required = schema.required?.includes(key),
            value = values[key];
          const set = (next: unknown) =>
            setValues((current) => ({ ...current, [key]: next }));
          const secret = property.writeOnly || property.format === "password";
          let control;
          if (property.enum)
            control = (
              <select
                required={required}
                value={value === undefined ? "" : JSON.stringify(value)}
                onChange={(e) =>
                  set(e.target.value ? JSON.parse(e.target.value) : undefined)
                }
              >
                <option value="">선택</option>
                {property.enum.map((entry, i) => (
                  <option key={i} value={JSON.stringify(entry)}>
                    {String(entry)}
                  </option>
                ))}
              </select>
            );
          else if (property.type === "boolean")
            control = (
              <input
                type="checkbox"
                checked={value === true}
                onChange={(e) => set(e.target.checked)}
              />
            );
          else if (property.type === "object" || property.type === "array")
            control = (
              <Textarea
                className="json-input"
                value={
                  drafts[key] ??
                  JSON.stringify(
                    value ?? (property.type === "array" ? [] : {}),
                    null,
                    2,
                  )
                }
                onChange={(e) => {
                  const raw = e.target.value;
                  setDrafts((d) => ({ ...d, [key]: raw }));
                  try {
                    JSON.parse(raw);
                    e.target.setCustomValidity("");
                  } catch {
                    e.target.setCustomValidity("유효한 JSON을 입력해 주세요.");
                  }
                }}
              />
            );
          else if (property.type === "number" || property.type === "integer")
            control = (
              <Input
                type="number"
                required={required}
                min={property.minimum}
                max={property.maximum}
                step={property.type === "integer" ? 1 : "any"}
                value={value === undefined ? "" : String(value)}
                onChange={(e) =>
                  set(
                    e.target.value === "" ? undefined : Number(e.target.value),
                  )
                }
              />
            );
          else
            control = (
              <Input
                type={secret ? "password" : "text"}
                autoComplete={secret ? "new-password" : undefined}
                required={required && value !== "********"}
                minLength={property.minLength}
                maxLength={property.maxLength}
                placeholder={
                  secret && value === "********" ? "저장된 값 유지" : ""
                }
                value={
                  secret && value === "********" ? "" : String(value ?? "")
                }
                onChange={(e) => set(e.target.value)}
              />
            );
          return (
            <Field key={key} label={title} hint={property.description}>
              {control}
            </Field>
          );
        })}
      </div>
      <ErrorBox message={error} />
      <div className="form-actions">
        <Button disabled={disabled || busy} type="submit">
          {busy ? "처리 중…" : label}
        </Button>
      </div>
    </form>
  );
}
