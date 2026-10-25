// QR 등록과 복구 코드 일회 표시 및 재인증 화면을 검증한다.
import { act, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import { SecurityPage, RecoveryCodes } from "./Security";
import * as webauthn from "./webauthn";
const user = {
  id: "u",
  username: "admin",
  role: "admin" as const,
  is_active: true,
  created_at: "",
};
afterEach(() => {
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});
function serve(
  reply?: (
    path: string,
    init?: RequestInit,
  ) => Response | Promise<Response> | undefined,
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

it("화면 이탈 뒤 도착한 재인증은 후속 등록을 시작하지 않는다", async () => {
  let finish!: (value: Response) => void;
  let signal: AbortSignal | null | undefined;
  const pending = new Promise<Response>((resolve) => {
    finish = resolve;
  });
  const paths: string[] = [];
  serve((path, init) => {
    paths.push(path);
    if (path.endsWith("/reauth")) {
      signal = init?.signal;
      return pending;
    }
  });
  const view = render(
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
  view.unmount();
  await act(async () => {
    finish(
      Response.json({ proof: "old-proof", expires_at: "2099-01-01T00:00:00Z" }),
    );
  });
  expect(paths.some((path) => path.endsWith("/totp/setup"))).toBe(false);
  expect(signal?.aborted).toBe(true);
});

it("계정 전환 뒤 늦은 복구 코드는 새 계정 화면에 표시하지 않는다", async () => {
  let finish!: (value: Response) => void;
  const pending = new Promise<Response>((resolve) => {
    finish = resolve;
  });
  const codes = vi.fn(),
    signedOut = vi.fn();
  serve((path) => (path.endsWith("/confirm") ? pending : undefined));
  const props = {
    changed: vi.fn(),
    notify: vi.fn(),
    onSignedOut: signedOut,
    onRecoveryCodes: codes,
  };
  const view = render(<SecurityPage key={user.id} user={user} {...props} />);
  await userEvent.click(
    await screen.findByRole("button", { name: "2FA 설정" }),
  );
  await userEvent.type(
    screen.getByLabelText("재인증 비밀번호"),
    "test-password",
  );
  await userEvent.click(screen.getByRole("button", { name: "인증 후 계속" }));
  await screen.findByAltText("인증 앱 등록용 QR 코드");
  await userEvent.type(screen.getByLabelText("인증 코드"), "012345");
  await userEvent.click(screen.getByRole("button", { name: "인증 및 활성화" }));
  view.rerender(
    <SecurityPage key="other" user={{ ...user, id: "other" }} {...props} />,
  );
  await act(async () => {
    finish(
      Response.json({
        recovery_codes: ["OLD-ACCOUNT-SECRET"],
        signed_out: true,
      }),
    );
  });
  expect(codes).not.toHaveBeenCalled();
  expect(signedOut).not.toHaveBeenCalled();
});

it("화면 이탈은 인증기 창을 취소하고 늦은 등록 결과를 버린다", async () => {
  let finish!: (value: unknown) => void;
  const pending = new Promise<unknown>((resolve) => {
    finish = resolve;
  });
  vi.spyOn(webauthn, "passkeyBrowserSupport").mockReturnValue("");
  const create = vi.spyOn(webauthn, "createPasskey").mockReturnValue(pending);
  const paths: string[] = [];
  serve((path) => {
    paths.push(path);
    if (path.endsWith("/security"))
      return Response.json({
        totp_enabled: false,
        recovery_codes_remaining: 0,
        passkeys: [],
        passkey_available: true,
        passkey_reason: "",
      });
    if (path.endsWith("/begin")) return Response.json({ publicKey: {} });
  });
  const view = render(
    <SecurityPage
      user={user}
      changed={vi.fn()}
      notify={vi.fn()}
      onSignedOut={vi.fn()}
      onRecoveryCodes={vi.fn()}
    />,
  );
  await userEvent.type(screen.getByLabelText("새 Passkey 이름"), "기기");
  await userEvent.click(
    await screen.findByRole("button", { name: "Passkey 추가" }),
  );
  await userEvent.type(
    screen.getByLabelText("재인증 비밀번호"),
    "test-password",
  );
  await userEvent.click(screen.getByRole("button", { name: "인증 후 계속" }));
  await waitFor(() => expect(create).toHaveBeenCalled());
  view.unmount();
  expect(create.mock.calls[0][1]?.aborted).toBe(true);
  await act(async () => {
    finish({ id: "cancelled-key" });
  });
  expect(paths.some((path) => path.endsWith("/register/finish"))).toBe(false);
});

it("웹 세션 종료와 API 토큰 유지의 차이를 안내한다", async () => {
  serve();
  render(
    <SecurityPage
      user={user}
      changed={vi.fn()}
      notify={vi.fn()}
      onSignedOut={vi.fn()}
      onRecoveryCodes={vi.fn()}
    />,
  );
  expect(await screen.findByText(/API 토큰은 유지되며/)).toBeTruthy();
});
