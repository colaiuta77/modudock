// 빌드된 실행파일의 관리자 생성·인증·권한·내장 UI를 실제 HTTP로 검증한다.
import assert from "node:assert/strict";
import { spawn, spawnSync } from "node:child_process";
import { randomBytes } from "node:crypto";
import { mkdtemp, rm } from "node:fs/promises";
import { tmpdir } from "node:os";
import { resolve, join, sep } from "node:path";
import { createServer } from "node:net";

const binary = resolve(process.argv[2] || "dist/modudock-windows-amd64.exe");
const dataDir = await mkdtemp(join(tmpdir(), "modudock-smoke-"));
const password = randomBytes(24).toString("base64url");
const probe = createServer();
await new Promise((done) => probe.listen(0, "127.0.0.1", done));
const port = probe.address().port;
await new Promise((done) => probe.close(done));
const origin = `http://127.0.0.1:${port}`;
let child;
try {
  const admin = spawnSync(binary, ["admin", "create", "--data-dir", dataDir, "--username", "smokeadmin", "--password-stdin"], { input: password + "\n", encoding: "utf8" });
  assert.equal(admin.status, 0, admin.error?.message || admin.stderr);
  child = spawn(binary, ["serve", "--data-dir", dataDir, "--listen", `127.0.0.1:${port}`], { stdio: ["ignore", "pipe", "pipe"] });
  let output = "";
  child.stdout.on("data", (chunk) => { output += chunk; });
  child.stderr.on("data", (chunk) => { output += chunk; });
  for (let attempt = 0; attempt < 100; attempt++) {
    if (child.exitCode !== null) throw new Error(`Server exited: ${output}`);
    try { if ((await fetch(origin + "/healthz")).ok) break; } catch {}
    if (attempt === 99) throw new Error("Server did not become healthy");
    await new Promise((done) => setTimeout(done, 100));
  }
  const page = await fetch(origin);
  assert.equal(page.status, 200);
  assert.match(await page.text(), /<div id="root"><\/div>/);
  assert.equal((await fetch(origin + "/api/v1/users")).status, 401);
  const login = await fetch(origin + "/api/v1/auth/login", { method: "POST", headers: { "Content-Type": "application/json", Origin: origin }, body: JSON.stringify({ username: "smokeadmin", password }) });
  assert.equal(login.status, 200, await login.clone().text());
  const { csrf_token } = await login.json();
  const cookie = login.headers.get("set-cookie").split(";")[0];
  const headers = { "Content-Type": "application/json", Origin: origin, Cookie: cookie, "X-CSRF-Token": csrf_token };
  const missingCSRF = { ...headers }; delete missingCSRF["X-CSRF-Token"];
  assert.equal((await fetch(origin + "/api/v1/users", { method: "POST", headers: missingCSRF, body: "{}" })).status, 403);
  const created = await fetch(origin + "/api/v1/users", { method: "POST", headers, body: JSON.stringify({ username: "smokeviewer", password, role: "viewer" }) });
  assert.equal(created.status, 201, await created.clone().text());
  const tokenResponse = await fetch(origin + "/api/v1/tokens", { method: "POST", headers, body: JSON.stringify({ name: "smoke", scopes: ["status:read"], expires_at: new Date(Date.now() + 86400000).toISOString() }) });
  assert.equal(tokenResponse.status, 201, await tokenResponse.clone().text());
  const { token } = await tokenResponse.json();
  assert.equal((await fetch(origin + "/api/v1/status", { headers: { Authorization: `Bearer ${token}` } })).status, 200);
  assert.equal((await fetch(origin + "/api/v1/users", { headers: { Authorization: `Bearer ${token}` } })).status, 403);
  const unknown = await fetch(origin + "/api/v1/not-a-route", { headers });
  assert.equal(unknown.status, 404);
  assert.doesNotMatch(unknown.headers.get("content-type") || "", /text\/html/);
  const spec = await fetch(origin + "/api/v1/openapi.json", { headers });
  assert.equal(spec.status, 200);
  const document = await spec.json();
  assert.equal(document.servers[0].url, "/api/v1");
  assert.ok(document.paths["/auth/login"]);
  console.log("PASS: native binary, embedded UI, login, CSRF, users, token scopes, API fallback and OpenAPI");
} finally {
  if (child && child.exitCode === null) {
    const stopped = new Promise((done) => child.once("exit", done));
    child.kill("SIGTERM");
    await stopped;
  }
  assert.ok(resolve(dataDir).startsWith(resolve(tmpdir()) + sep) && dataDir.includes("modudock-smoke-"));
  await rm(dataDir, { recursive: true, force: true });
}
