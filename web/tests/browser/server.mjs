// Every run owns a fresh library. Never connects to a developer's existing Bookshelf.
import { mkdtemp, rm } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { fileURLToPath } from "node:url";
import { execFileSync, spawn } from "node:child_process";
const scratch = await mkdtemp(join(tmpdir(), "bookshelf-browser-"));
const binary = process.env.BOOKSHELF_TEST_BINARY || join(scratch, process.platform === "win32" ? "bookshelf.exe" : "bookshelf");
if (!process.env.BOOKSHELF_TEST_BINARY) execFileSync("go", ["build", "-o", binary, "./cmd/bookshelf"], {
  cwd: fileURLToPath(new URL("../../../", import.meta.url)), stdio: "inherit",
});
const child = spawn(binary, ["--data-dir", join(scratch, "data"), "--host", "127.0.0.1", "--port", "18323"], {
  env: { ...process.env, BOOKSHELF_ADMIN_USER: "admin", BOOKSHELF_ADMIN_PASSWORD: "review-browser-only" }, stdio: "inherit",
});
for (const signal of ["SIGINT", "SIGTERM"]) process.on(signal, () => child.kill(signal));
child.on("exit", async (code) => { await rm(scratch, { recursive: true, force: true }); process.exit(code || 0); });
