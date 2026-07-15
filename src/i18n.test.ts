import { baseCompile, type CompileError } from "@intlify/message-compiler";
import { describe, expect, it } from "vitest";
import { messages } from "./i18n";

function messageEntries(value: unknown, path = ""): Array<[string, string]> {
  if (typeof value === "string") return [[path, value]];
  if (!value || typeof value !== "object") return [];
  return Object.entries(value).flatMap(([key, child]) => messageEntries(child, path ? `${path}.${key}` : key));
}

describe("i18n messages", () => {
  it("compiles every Chinese and English message", () => {
    const failures: string[] = [];
    for (const [path, message] of messageEntries(messages)) {
      const errors: CompileError[] = [];
      try {
        baseCompile(message, { onError: (error) => errors.push(error) });
      } catch (error) {
        failures.push(`${path}: ${(error as Error).message}`);
        continue;
      }
      for (const error of errors) failures.push(`${path}: ${error.message}`);
    }
    expect(failures).toEqual([]);
  });
});
