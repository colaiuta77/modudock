// 브라우저 기본 WebAuthn 변환을 사용하고 취소 및 미지원 환경을 구분한다.
type NativeWebAuthn = typeof PublicKeyCredential & {
  parseCreationOptionsFromJSON(
    value: unknown,
  ): PublicKeyCredentialCreationOptions;
  parseRequestOptionsFromJSON(
    value: unknown,
  ): PublicKeyCredentialRequestOptions;
};
export class PasskeyCancelled extends Error {}
export function passkeyBrowserSupport(): string {
  if (
    typeof PublicKeyCredential === "undefined" ||
    !navigator.credentials?.create ||
    !navigator.credentials?.get
  )
    return "이 브라우저는 Passkey를 지원하지 않습니다. 최신 브라우저를 사용해 주세요.";
  const native = PublicKeyCredential as NativeWebAuthn;
  if (
    !globalThis.isSecureContext ||
    !native.parseCreationOptionsFromJSON ||
    !native.parseRequestOptionsFromJSON ||
    !("toJSON" in native.prototype)
  )
    return "Passkey는 HTTPS 또는 localhost와 최신 브라우저가 필요합니다.";
  return "";
}
async function credential(
  options: unknown,
  registration: boolean,
  signal?: AbortSignal,
): Promise<unknown> {
  const reason = passkeyBrowserSupport();
  if (reason) throw new Error(reason);
  const native = PublicKeyCredential as NativeWebAuthn;
  const json = (options as { publicKey: unknown }).publicKey;
  try {
    const result = registration
      ? await navigator.credentials.create({
          publicKey: native.parseCreationOptionsFromJSON(json),
          signal,
        })
      : await navigator.credentials.get({
          publicKey: native.parseRequestOptionsFromJSON(json),
          signal,
        });
    if (!result) throw new PasskeyCancelled();
    return (result as PublicKeyCredential & { toJSON(): unknown }).toJSON();
  } catch (error) {
    if (
      error instanceof DOMException &&
      ["NotAllowedError", "AbortError"].includes(error.name)
    )
      throw new PasskeyCancelled();
    throw error;
  }
}
export const createPasskey = (options: unknown, signal?: AbortSignal) =>
  credential(options, true, signal);
export const getPasskey = (options: unknown, signal?: AbortSignal) =>
  credential(options, false, signal);
