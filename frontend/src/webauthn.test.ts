// 브라우저 기본 WebAuthn JSON 변환과 취소 및 미지원 처리를 검증한다.
import { afterEach, expect, it, vi } from "vitest";
import {
  createPasskey,
  getPasskey,
  passkeyBrowserSupport,
  PasskeyCancelled,
} from "./webauthn";
afterEach(() => vi.unstubAllGlobals());
it("미지원 환경에서는 브라우저 업그레이드를 안내한다", async () => {
  vi.stubGlobal("PublicKeyCredential", undefined);
  expect(passkeyBrowserSupport()).not.toBe("");
  await expect(getPasskey({ publicKey: {} })).rejects.toThrow("브라우저");
});
it("브라우저 파서와 credential JSON 직렬화를 그대로 사용한다", async () => {
  const parse = vi.fn(() => ({ challenge: new Uint8Array([1]) }));
  const toJSON = vi.fn(() => ({ id: "test-key" }));
  vi.stubGlobal("isSecureContext", true);
  vi.stubGlobal("PublicKeyCredential", {
    parseCreationOptionsFromJSON: parse,
    parseRequestOptionsFromJSON: parse,
    prototype: { toJSON },
  });
  const create = vi.fn(async () => ({ toJSON }));
  vi.stubGlobal("navigator", { credentials: { create, get: create } });
  expect(await createPasskey({ publicKey: { challenge: "AQ" } })).toEqual({
    id: "test-key",
  });
  expect(parse).toHaveBeenCalledWith({ challenge: "AQ" });
  expect(create).toHaveBeenCalled();
  expect(sessionStorage.length + localStorage.length).toBe(0);
});
it("사용자가 인증기를 닫은 경우 취소로 구분한다", async () => {
  vi.stubGlobal("isSecureContext", true);
  vi.stubGlobal("PublicKeyCredential", {
    parseCreationOptionsFromJSON: vi.fn(),
    parseRequestOptionsFromJSON: vi.fn(),
    prototype: { toJSON: vi.fn() },
  });
  vi.stubGlobal("navigator", {
    credentials: {
      create: vi.fn(),
      get: vi.fn(async () => {
        throw new DOMException("cancelled", "NotAllowedError");
      }),
    },
  });
  await expect(getPasskey({ publicKey: {} })).rejects.toBeInstanceOf(
    PasskeyCancelled,
  );
});
