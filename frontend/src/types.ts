export interface Assertion {
  pointer: string;
  equals: unknown;
}
export interface Verification {
  url: string;
  headers: Record<string, string>;
  assertions: Assertion[];
  pollIntervalMs: number;
  timeoutMs: number;
  requestTimeoutMs: number;
  stableForMs: number;
}
export interface Scenario {
  version: number;
  name: string;
  description: string;
  targetUrl: string;
  headers: Record<string, string>;
  body: string;
  copies: number;
  concurrency: number;
  timeoutMs: number;
  expectedStatuses: number[];
  verification?: Verification;
}
export interface Attempt {
  id: number;
  startedAt: string;
  durationMs: number;
  status: number | null;
  outcome: string;
  preview: string;
  truncated: boolean;
}
export interface Report {
  id: string;
  state: string;
  verdict: string;
  scenario: Scenario;
  startedAt: string;
  finishedAt: string | null;
  attempts: Attempt[];
  scheduled: number;
  unstarted: number;
  deliveryPassed: boolean;
  business: {
    outcome: string;
    detail: string;
    observations: {
      at: string;
      outcome: string;
      checks?: {
        pointer: string;
        expected: unknown;
        actual: unknown;
        found: boolean;
        matched: boolean;
      }[];
    }[];
    droppedObservations: number;
  };
}
export interface DemoCase {
  caseId: string;
  mode: string;
  generation: string;
  lifecycle: string;
}
export const terminal = (r: Report) =>
  ["completed", "cancelled", "failed"].includes(r.state);
export async function api<T>(url: string, init?: RequestInit): Promise<T> {
  const res = await fetch(url, init);
  if (res.status === 204) return null as T;
  const body: unknown = await res.json().catch(() => null);
  if (!res.ok)
    throw new Error(
      body && typeof body === "object" && "detail" in body
        ? String(body.detail)
        : `Request failed (${res.status}).`,
    );
  return body as T;
}
export function parseScenario(value: unknown): Scenario {
  if (!value || typeof value !== "object" || Array.isArray(value))
    throw new Error("Import one scenario JSON object.");
  const keys = [
    "version",
    "name",
    "description",
    "targetUrl",
    "headers",
    "body",
    "copies",
    "concurrency",
    "timeoutMs",
    "expectedStatuses",
    "verification",
  ];
  if (Object.keys(value).some((key) => !keys.includes(key)))
    throw new Error(
      "Unknown settings field. Import a version 1 scenario file.",
    );
  const s = { ...value } as Partial<Scenario>;
  if ("verification" in value && s.verification === null)
    throw new Error("Omit verification when no result check is needed.");
  if (s.description === undefined) s.description = "";
  if (s.verification && !s.verification.headers) s.verification.headers = {};
  const headerMap = (v: unknown) =>
    v &&
    typeof v === "object" &&
    !Array.isArray(v) &&
    Object.values(v).every((x) => typeof x === "string");
  if (
    s.version !== 1 ||
    typeof s.name !== "string" ||
    typeof s.description !== "string" ||
    typeof s.targetUrl !== "string" ||
    typeof s.body !== "string" ||
    !headerMap(s.headers) ||
    !Number.isInteger(s.copies) ||
    !Number.isInteger(s.concurrency) ||
    !Number.isInteger(s.timeoutMs) ||
    !Array.isArray(s.expectedStatuses) ||
    !s.expectedStatuses.every(Number.isInteger)
  )
    throw new Error("Invalid scenario shape. Use a version 1 scenario file.");
  if (
    s.verification &&
    (typeof s.verification.url !== "string" ||
      !headerMap(s.verification.headers) ||
      !Array.isArray(s.verification.assertions) ||
      !s.verification.assertions.every(
        (a) =>
          a &&
          typeof a === "object" &&
          typeof a.pointer === "string" &&
          "equals" in a &&
          (a.equals === null ||
            ["string", "number", "boolean"].includes(typeof a.equals)),
      ))
  )
    throw new Error("Invalid verification configuration.");
  return s as Scenario;
}
export function download(name: string, data: unknown) {
  const url = URL.createObjectURL(
    new Blob([JSON.stringify(data, null, 2) + "\n"], {
      type: "application/json",
    }),
  );
  const a = document.createElement("a");
  a.href = url;
  a.download = name;
  a.click();
  setTimeout(() => URL.revokeObjectURL(url), 1000);
}
export function safeScenario(s: Scenario): Scenario {
  const copy = structuredClone(s);
  const secrets: string[] = [];
  const safe = ["content-type", "accept", "x-demo-generation", "x-request-id"];
  for (const headers of [s.headers, s.verification?.headers ?? {}]) {
    for (const [key, value] of Object.entries(headers))
      if (!safe.includes(key.toLowerCase()) && value) {
        secrets.push(value);
        if (value.startsWith("Bearer ")) secrets.push(value.slice(7));
      }
  }
  const scrub = (text: string) => {
    for (const secret of secrets) {
      text = text.split(secret).join("[REDACTED]");
      text = text.split(JSON.stringify(secret).slice(1, -1)).join("[REDACTED]");
    }
    return text;
  };
  copy.name = scrub(copy.name);
  copy.description = scrub(copy.description);
  copy.body = scrub(copy.body);
  for (const headers of [copy.headers, copy.verification?.headers ?? {}])
    for (const key of Object.keys(headers))
      headers[key] = safe.includes(key.toLowerCase())
        ? scrub(headers[key])
        : "[REDACTED]";
  const safeUrl = (text: string) => {
    try {
      const u = new URL(text);
      u.username = "";
      u.password = "";
      if (u.search) u.search = "redacted";
      return scrub(u.toString());
    } catch {
      return scrub(text);
    }
  };
  copy.targetUrl = safeUrl(copy.targetUrl);
  if (copy.verification) {
    copy.verification.url = safeUrl(copy.verification.url);
    copy.verification.assertions = copy.verification.assertions.map((a) => ({
      ...a,
      equals: typeof a.equals === "string" ? scrub(a.equals) : a.equals,
    }));
  }
  return copy;
}
