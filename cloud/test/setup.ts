import { applyD1Migrations, env, reset } from "cloudflare:test";
import { beforeEach, inject } from "vitest";
import type { D1Migration } from "@cloudflare/vitest-pool-workers";

declare module "vitest" {
  export interface ProvidedContext {
    migrations: D1Migration[];
  }
}

beforeEach(async () => {
  await reset();
  await applyD1Migrations(env.DB, inject("migrations"));
});
