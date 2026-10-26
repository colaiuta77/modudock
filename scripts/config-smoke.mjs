// 실제 실행파일의 YAML 설정·접속 주소·로그 출력을 통합 검증한다.
import assert from "node:assert/strict";
import { spawn, spawnSync } from "node:child_process";
import { randomBytes } from "node:crypto";
import { mkdtemp, readFile, readdir, rm, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { resolve, join, sep } from "node:path";
import { createServer } from "node:net";

const binary = resolve(process.argv[2] || "dist/modudock-windows-amd64.exe");
const directory = await mkdtemp(join(tmpdir(), "modudock-config-smoke-"));
const data = join(directory, "data");
const config = join(directory, "config.yaml");
const log = join(directory, "modudock.log");
const password = randomBytes(24).toString("base64url");
const probe = createServer();
await new Promise((done) => probe.listen(0, "127.0.0.1", done));
const port = probe.address().port;
await new Promise((done) => probe.close(done));
const localOrigin = `http://127.0.0.1:${port}`;
const publicOrigin = "https://config.example.test";
let child;
function command(args, input) {
  const result = spawnSync(binary, args, { input, encoding: "utf8", timeout: 15000 });
  assert.equal(result.status, 0, result.error?.message || result.stderr || result.stdout);
}
try {
  await writeFile(config, `data_dir: ${JSON.stringify(data)}\nlisten: "0.0.0.0:${port}"\npublic_url: "${publicOrigin}"\nlog_file: ${JSON.stringify(log)}\n`, { mode: 0o600 });
  command(["config", "check", "--config", config]);
  assert.deepEqual(await readdir(directory), ["config.yaml"], "Validation must not create data or logs");
  command(["admin", "create", "--data-dir", data, "--username", "configsmoke", "--password-stdin"], password + "\n");
  child = spawn(binary, ["serve", "--config", config], { stdio: ["ignore", "pipe", "pipe"] });
  let output = "";
  child.stdout.on("data", (chunk) => { output += chunk; });
  child.stderr.on("data", (chunk) => { output += chunk; });
  child.on("error", (error) => { output += error.message; });
  for (let attempt = 0; attempt < 100; attempt++) {
    if (child.exitCode !== null) throw new Error(`Server exited: ${output}`);
    try { if ((await fetch(localOrigin + "/healthz")).ok) break; } catch {}
    if (attempt === 99) throw new Error(`Server did not become healthy: ${output}`);
    await new Promise((done) => setTimeout(done, 100));
  }
  command(["healthcheck", "--config", config]);
  const request = (origin) => fetch(localOrigin + "/api/v1/auth/login", {
    method: "POST", headers: { "Content-Type": "application/json", Origin: origin },
    body: JSON.stringify({ username: "configsmoke", password }),
  });
  assert.equal((await request(localOrigin)).status, 403, "Configured public origin must be enforced");
  const response = await request(publicOrigin);
  assert.equal(response.status, 200, await response.clone().text());
  for (const file of [log, join(data, "logs", "core.jsonl")]) {
    const entries = (await readFile(file, "utf8")).trim().split("\n").map(JSON.parse);
    assert.ok(entries.some((entry) => entry.message === "login"), `Login event missing in ${file}`);
  }
  console.log("PASS: YAML validation has no side effects, configured listener/origin, local healthcheck and both log outputs");
} finally {
  if (child && child.exitCode === null) {
    const stopped = new Promise((done) => child.once("exit", done));
    child.kill("SIGTERM");
    await stopped;
  }
  assert.ok(resolve(directory).startsWith(resolve(tmpdir()) + sep) && directory.includes("modudock-config-smoke-"));
  await rm(directory, { recursive: true, force: true });
}
