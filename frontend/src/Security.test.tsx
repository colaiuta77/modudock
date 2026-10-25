// QR 등록과 복구 코드 일회 표시 및 재인증 화면을 검증한다.
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import { SecurityPage, RecoveryCodes } from "./Security";
const user = {
  id: "u",
  username: "admin",
  role: "admin" as const,
  is_active: true,
  created_at: "",
};
afterEach(() => vi.unstubAllGlobals());
function serve(
  reply?: (path: string, init?: RequestInit) => Response | undefined,
) {
  vi.stubGlobal(
    "fetch",
    async (path: string, init?: RequestInit) =>
      reply?.(path, init) ||
      Response.json(
        path.endsWith("/security")
          ? {
              totp_enabled: false,
              recovery_codes_remaining: 0,
              passkeys: [],
              passkey_available: false,
              passkey_reason: "HTTPS 주소 설정이 필요합니다.",
            }
          : path.endsWith("/reauth")
            ? { proof: "proof", expires_at: "2099-01-01T00:00:00Z" }
            : {
                secret: "ABCDEF234567",
                qr_png: "data:image/png;base64,test",
                expires_at: "2099-01-01T00:00:00Z",
              },
      ),
  );
}
it("QR와 수동 키를 표시하고 잘못된 코드를 수정할 수 있다", async () => {
  let tries = 0;
  const signedOut = vi.fn(),
    codes = vi.fn();
  serve((path, init) => {
    if (path.endsWith("/confirm")) {
      tries++;
      expect(JSON.parse(String(init?.body)).code).toBe("012345");
      return tries === 1
        ? Response.json(
            {
              error: {
                code: "invalid_credentials",
                message: "코드를 확인하세요.",
              },
            },
            { status: 401 },
          )
        : Response.json({
            recovery_codes: ["RECOVERY-CODE"],
            signed_out: true,
          });
    }
  });
  render(
    <SecurityPage
      user={user}
      changed={vi.fn()}
      notify={vi.fn()}
      onSignedOut={signedOut}
      onRecoveryCodes={codes}
    />,
  );
  await userEvent.click(
    await screen.findByRole("button", { name: "2FA 설정" }),
  );
  await userEvent.type(
    screen.getByLabelText("재인증 비밀번호"),
    "test-password",
  );
  await userEvent.click(screen.getByRole("button", { name: "인증 후 계속" }));
  expect(await screen.findByAltText("인증 앱 등록용 QR 코드")).toBeTruthy();
  expect(screen.getByLabelText("시크릿 키")).toHaveProperty(
    "value",
    "ABCDEF234567",
  );
  await userEvent.type(screen.getByLabelText("인증 코드"), "012345");
  await userEvent.click(screen.getByRole("button", { name: "인증 및 활성화" }));
  expect(await screen.findByText("코드를 확인하세요.")).toBeTruthy();
  expect(screen.getByLabelText("시크릿 키")).toBeTruthy();
  await userEvent.click(screen.getByRole("button", { name: "인증 및 활성화" }));
  await waitFor(() => expect(codes).toHaveBeenCalledWith(["RECOVERY-CODE"]));
  expect(signedOut).toHaveBeenCalled();
});
it("등록 취소 시 QR와 비밀 키를 화면에서 제거한다", async () => {
  serve((path, init) =>
    path.endsWith("/setup") && init?.method === "DELETE"
      ? new Response(null, { status: 204 })
      : undefined,
  );
  render(
    <SecurityPage
      user={user}
      changed={vi.fn()}
      notify={vi.fn()}
      onSignedOut={vi.fn()}
      onRecoveryCodes={vi.fn()}
    />,
  );
  await userEvent.click(
    await screen.findByRole("button", { name: "2FA 설정" }),
  );
  await userEvent.type(
    screen.getByLabelText("재인증 비밀번호"),
    "test-password",
  );
  await userEvent.click(screen.getByRole("button", { name: "인증 후 계속" }));
  await screen.findByAltText("인증 앱 등록용 QR 코드");
  await userEvent.click(screen.getByRole("button", { name: "등록 취소" }));
  await waitFor(() =>
    expect(screen.queryByAltText("인증 앱 등록용 QR 코드")).toBeNull(),
  );
});
it("복구 코드는 저장소 없이 저장 확인 전까지만 보여 준다", async () => {
  const close = vi.fn();
  render(<RecoveryCodes codes={["ABCDEF-234567"]} onClose={close} />);
  expect(screen.getByText("ABCDEF-234567")).toBeTruthy();
  expect(localStorage.length + sessionStorage.length).toBe(0);
  await userEvent.click(
    screen.getByLabelText("복구 코드를 안전한 곳에 저장했습니다."),
  );
  await userEvent.click(
    screen.getByRole("button", { name: "저장 확인 후 로그인" }),
  );
  expect(close).toHaveBeenCalled();
});
