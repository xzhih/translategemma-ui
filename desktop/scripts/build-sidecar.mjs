import { execFileSync, spawnSync } from "node:child_process";
import { chmodSync, mkdirSync } from "node:fs";
import { dirname, join, resolve } from "node:path";
import { fileURLToPath } from "node:url";

const __filename = fileURLToPath(import.meta.url);
const __dirname = dirname(__filename);
const desktopRoot = resolve(__dirname, "..");
const projectRoot = resolve(desktopRoot, "..");
const binariesDir = join(desktopRoot, "src-tauri", "binaries");

function detectTargetTriple() {
  const explicit = process.env.TAURI_ENV_TARGET_TRIPLE || process.env.CARGO_BUILD_TARGET;
  if (explicit && explicit.trim()) {
    return explicit.trim();
  }

  const rustc = execFileSync("rustc", ["-vV"], {
    encoding: "utf8",
    cwd: projectRoot,
  });
  const hostLine = rustc
    .split("\n")
    .map((line) => line.trim())
    .find((line) => line.startsWith("host: "));
  if (!hostLine) {
    throw new Error("unable to detect Rust host target triple");
  }
  return hostLine.slice("host: ".length).trim();
}

function mapTripleToGo(targetTriple) {
  let goos = "";
  let goarch = "";

  if (targetTriple.includes("apple-darwin")) {
    goos = "darwin";
  } else if (targetTriple.includes("windows")) {
    goos = "windows";
  } else if (targetTriple.includes("linux")) {
    goos = "linux";
  } else {
    throw new Error(`unsupported target OS in ${targetTriple}`);
  }

  if (targetTriple.startsWith("x86_64-")) {
    goarch = "amd64";
  } else if (targetTriple.startsWith("aarch64-")) {
    goarch = "arm64";
  } else {
    throw new Error(`unsupported target architecture in ${targetTriple}`);
  }

  return {
    goos,
    goarch,
    exeSuffix: goos === "windows" ? ".exe" : "",
  };
}

const targetTriple = detectTargetTriple();
const { goos, goarch, exeSuffix } = mapTripleToGo(targetTriple);
const outputPath = join(binariesDir, `translategemma-ui-${targetTriple}${exeSuffix}`);

mkdirSync(binariesDir, { recursive: true });
console.log(`[desktop] building Go sidecar for ${targetTriple}`);

const result = spawnSync("go", ["build", "-trimpath", "-o", outputPath, "./cmd/translategemma-ui"], {
  cwd: projectRoot,
  env: {
    ...process.env,
    CGO_ENABLED: "0",
    GOOS: goos,
    GOARCH: goarch,
  },
  stdio: "inherit",
});

if (result.status !== 0) {
  process.exit(result.status ?? 1);
}

if (!exeSuffix) {
  chmodSync(outputPath, 0o755);
}

console.log(`[desktop] sidecar ready at ${outputPath}`);
