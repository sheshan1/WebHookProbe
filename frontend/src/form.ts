import { parseScenario } from "./types";
import type { Scenario } from "./types";
export interface HeaderRow {
  id: string;
  name: string;
  value: string;
}
export interface CheckRow {
  id: string;
  pointer: string;
  value: string;
  type: "number" | "string" | "boolean" | "null";
}
export const rowId = () => crypto.randomUUID();
export const headerRows = (headers: Record<string, string>): HeaderRow[] =>
  Object.entries(headers).map(([name, value]) => ({
    id: rowId(),
    name,
    value,
  }));
export function headerMap(rows: HeaderRow[]): Record<string, string> {
  const result: Record<string, string> = {};
  for (const row of rows) {
    if (!row.name && !row.value) continue;
    const name = row.name.trim();
    if (!name)
      throw new Error("Give each header a name, or remove the empty row.");
    if (
      Object.keys(result).some(
        (key) => key.toLowerCase() === name.toLowerCase(),
      )
    )
      throw new Error(`Header ${name} is repeated. Keep one value per header.`);
    result[name] = row.value;
  }
  return result;
}
export interface FormState {
  name: string;
  description: string;
  targetUrl: string;
  body: string;
  headers: HeaderRow[];
  copies: number;
  concurrency: number;
  timeoutMs: number;
  statuses: string;
  verify: boolean;
  stateUrl: string;
  checks: CheckRow[];
  stateHeaders: HeaderRow[];
  pollIntervalMs: number;
  verificationTimeoutMs: number;
  requestTimeoutMs: number;
  stableForMs: number;
}
export function emptyForm(): FormState {
  return {
    name: "",
    description: "",
    targetUrl: "",
    body: "",
    headers: headerRows({ "Content-Type": "application/json" }),
    copies: 1,
    concurrency: 1,
    timeoutMs: 5000,
    statuses: "200",
    verify: false,
    stateUrl: "",
    checks: [{ id: rowId(), pointer: "", value: "", type: "number" }],
    stateHeaders: [],
    pollIntervalMs: 100,
    verificationTimeoutMs: 5000,
    requestTimeoutMs: 1000,
    stableForMs: 500,
  };
}
export function fromScenario(s: Scenario): FormState {
  const f = emptyForm();
  return {
    ...f,
    name: s.name,
    description: s.description,
    targetUrl: s.targetUrl,
    body: s.body,
    headers: headerRows(s.headers),
    copies: s.copies,
    concurrency: s.concurrency,
    timeoutMs: s.timeoutMs,
    statuses: s.expectedStatuses.join(", "),
    verify: !!s.verification,
    ...(s.verification
      ? {
          stateUrl: s.verification.url,
          stateHeaders: headerRows(s.verification.headers),
          checks: s.verification.assertions.map((a) => ({
            id: rowId(),
            pointer: a.pointer,
            type:
              a.equals === null
                ? "null"
                : (typeof a.equals as CheckRow["type"]),
            value: a.equals === null ? "" : String(a.equals),
          })),
          pollIntervalMs: s.verification.pollIntervalMs,
          verificationTimeoutMs: s.verification.timeoutMs,
          requestTimeoutMs: s.verification.requestTimeoutMs,
          stableForMs: s.verification.stableForMs,
        }
      : {}),
  };
}
export function toScenario(f: FormState): Scenario {
  if (!f.targetUrl.trim())
    throw new Error(
      "Enter the URL that receives your webhook. Example text is only a hint.",
    );
  if (!Number.isInteger(f.copies) || f.copies < 1 || f.copies > 1000)
    throw new Error(
      "Number of requests must be a whole number from 1 to 1000.",
    );
  if (
    !Number.isInteger(f.concurrency) ||
    f.concurrency < 1 ||
    f.concurrency > Math.min(f.copies, 50)
  )
    throw new Error(
      "Parallel requests must be between 1 and the request count, up to 50.",
    );
  const statuses = f.statuses.split(",").map((v) => v.trim());
  if (statuses.some((v) => !/^\d{3}$/.test(v)))
    throw new Error(
      "Expected HTTP status must contain three-digit numbers, such as 200 or 200, 202.",
    );
  const s: Scenario = {
    version: 1,
    name: f.name.trim() || "Webhook test",
    description: f.description,
    targetUrl: f.targetUrl.trim(),
    body: f.body,
    headers: headerMap(f.headers),
    copies: f.copies,
    concurrency: f.concurrency,
    timeoutMs: f.timeoutMs,
    expectedStatuses: statuses.map(Number),
  };
  if (f.verify) {
    if (!f.stateUrl.trim())
      throw new Error(
        "Enter a state URL to check the result, or turn off the optional result check.",
      );
    s.verification = {
      url: f.stateUrl.trim(),
      headers: headerMap(f.stateHeaders),
      assertions: f.checks.map((c, i) => {
        let equals: unknown = c.value;
        if (c.pointer !== "" && !c.pointer.startsWith("/"))
          throw new Error(
            `Check ${i + 1}: field paths start with /. Use /notificationCount for that JSON field.`,
          );
        if (c.type === "number") {
          if (c.value.trim() === "")
            throw new Error(`Check ${i + 1}: enter the expected number.`);
          equals = JSON.parse(c.value);
          if (
            typeof equals !== "number" ||
            !Number.isFinite(equals) ||
            (Number.isInteger(equals) && !Number.isSafeInteger(equals))
          )
            throw new Error(
              `Check ${i + 1}: enter a safe JSON number. For larger numbers, use the CLI with a scenario file.`,
            );
        }
        if (c.type === "boolean") {
          if (!["true", "false"].includes(c.value))
            throw new Error(`Check ${i + 1}: choose true or false.`);
          equals = c.value === "true";
        }
        if (c.type === "null") equals = null;
        return { pointer: c.pointer, equals };
      }),
      pollIntervalMs: f.pollIntervalMs,
      timeoutMs: f.verificationTimeoutMs,
      requestTimeoutMs: f.requestTimeoutMs,
      stableForMs: f.stableForMs,
    };
  }
  return parseScenario(s);
}
