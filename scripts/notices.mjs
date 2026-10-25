// 설치된 프론트·Go 의존성의 라이선스 원문을 배포 고지로 모은다.
import { readdir, readFile, mkdir, writeFile } from "node:fs/promises";
import { execFileSync } from "node:child_process";
import { dirname, join, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import assert from "node:assert/strict";

const root = resolve(dirname(fileURLToPath(import.meta.url)), "..");
const parts = ["ModuDock Go — Third-party notices\nGenerated from the installed runtime dependency versions and Tailwind CSS.\n"];
async function append(name, directory) {
  const entries = await readdir(directory, { withFileTypes: true });
  const files = entries.filter(e => e.isFile() && /^(licen[sc]e|copying|notice)([.-]|$)/i.test(e.name));
  if (!files.length && name === "react-remove-scroll-bar 2.3.8") {
    parts.push(await readFile(join(root, "docs/react-remove-scroll-bar-license.txt"), "utf8"));
    return;
  }
  if (!files.length) throw new Error(`Missing license text: ${name}`);
  parts.push(`\n${"=".repeat(72)}\n${name}\n`);
  for (const file of files) parts.push(await readFile(join(directory, file.name), "utf8"));
}
const lock = JSON.parse(await readFile(join(root, "frontend/package-lock.json"), "utf8"));
for (const [path, metadata] of Object.entries(lock.packages).sort(([a], [b]) => a.localeCompare(b))) {
  if (!path || metadata.optional || (metadata.dev && !path.endsWith("/tailwindcss"))) continue;
  await append(`${path.replace(/^node_modules\//, "")} ${metadata.version}`, join(root, "frontend", path));
}
const modules = execFileSync("go", ["list", "-m", "-f", "{{if .Dir}}{{.Path}}\t{{.Version}}\t{{.Dir}}{{end}}", "all"], { cwd: root, encoding: "utf8" });
for (const line of modules.trim().split(/\r?\n/)) {
  const [name, version, directory] = line.split("\t");
  if (name === "modudock" || !directory) continue;
  await append(`${name} ${version}`, directory);
}
const goroot = execFileSync("go", ["env", "GOROOT"], { encoding: "utf8" }).trim();
await append("Go standard library", goroot);
parts.push(await readFile(join(root, "docs/shadcn-license.txt"), "utf8"));
parts.push(await readFile(join(root, "docs/better-design-license.txt"), "utf8"));
const content = parts.join("\n");
assert.match(content, /lucide-react/);
assert.match(content, /Permission/);
await mkdir(join(root, "frontend/public"), { recursive: true });
await writeFile(join(root, "frontend/public/third-party-notices.txt"), content);
console.log(`Generated third-party notices (${parts.length} sections).`);
