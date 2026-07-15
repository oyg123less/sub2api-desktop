import type { Bindings } from "./src/types";

declare global {
  namespace Cloudflare {
    interface Env extends Bindings {}
  }
}

export {};
