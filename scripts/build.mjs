// 프론트엔드와 라이선스를 내장한 플랫폼별 Go 실행파일을 빌드한다.
import assert from "node:assert/strict";
import { spawnSync, execFileSync } from "node:child_process";
import { cpSync, mkdirSync, readdirSync, rmSync, lstatSync } from "node:fs";
import { dirname, join, resolve, sep } from "node:path";
import { fileURLToPath } from "node:url";

const root = resolve(dirname(fileURLToPath(import.meta.url)), "..");
const front = join(root, "frontend");
const version = process.env.MODUDOCK_VERSION || "dev";
assert.match(version, /^[a-zA-Z0-9.+_-]+$/);
function run(command, args, cwd = root) {
  const result = spawnSync(command, args, { cwd, stdio: "inherit" });
  if (result.error) throw result.error;
  if (result.status !== 0) process.exit(result.status || 1);
}
function npm(args) {
  if (process.platform === "win32") run(process.execPath, [join(dirname(process.execPath), "node_modules/npm/bin/npm-cli.js"), ...args], front);
  else run("npm", args, front);
}
npm(["ci"]);
run("go", ["mod", "download"]);
run(process.execPath, [join(root, "scripts/notices.mjs")]);
npm(["run", "build"]);
const embedded = join(root, "internal/web/dist");
assert.ok(embedded.startsWith(root + sep));
mkdirSync(embedded, { recursive: true });
assert.ok(!lstatSync(embedded).isSymbolicLink());
for (const item of readdirSync(embedded)) {
  if (item !== ".gitkeep") rmSync(join(embedded, item), { recursive: true, force: true });
}
cpSync(join(front, "dist"), embedded, { recursive: true });
const targetOS = execFileSync("go", ["env", "GOOS"], { encoding: "utf8" }).trim();
const targetArch = execFileSync("go", ["env", "GOARCH"], { encoding: "utf8" }).trim();
const name = `modudock-${targetOS}-${targetArch}${targetOS === "windows" ? ".exe" : ""}`;
mkdirSync(join(root, "dist"), { recursive: true });
process.env.CGO_ENABLED = "0";
run("go", ["build", "-trimpath", "-ldflags", `-s -w -X main.version=${version}`, "-o", join(root, "dist", name), "./cmd/modudock"]);
cpSync(join(front, "public/third-party-notices.txt"), join(root, "dist/third-party-notices.txt"));
console.log(`Built ${join(root, "dist", name)}`);
