import { spawnSync } from "node:child_process";
import { createRequire } from "node:module";

const require = createRequire(import.meta.url);

const cli = require.resolve("next/dist/bin/next");
const result = spawnSync(process.execPath, [cli, ...process.argv.slice(2)], {
  env: { ...process.env, NEXT_TELEMETRY_DISABLED: "1" },
  stdio: "inherit",
});

if (result.error) throw result.error;
process.exit(result.status ?? 1);
