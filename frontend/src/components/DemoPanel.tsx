import { useState } from "react";
import { api } from "../types";
import type { DemoCase, Scenario } from "../types";
export function DemoPanel({
  locked,
  demo,
  onDemo,
  onLoad,
  onAction,
}: {
  locked: boolean;
  demo: DemoCase | undefined;
  onDemo: (c: DemoCase) => void;
  onLoad: (s: Scenario) => void;
  onAction: (fn: () => Promise<void>, message: string) => Promise<void>;
}) {
  const [mode, setMode] = useState("corrected");
  const [pattern, setPattern] = useState("concurrent");
  const [base, setBase] = useState("http://receiver:8080");
  const prepare = () =>
    onAction(async () => {
      const token = crypto.randomUUID().replaceAll("-", "");
      const caseId = "case-" + token;
      const parcelId = "parcel-" + token + "-1";
      const eventId = "evt-" + token + "-1";
      const path = "/demo/v1/demo/" + mode + "/cases";
      await api(path, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          caseId,
          parcelIds: [parcelId, "parcel-" + token + "-2"],
        }),
      });
      const c = await api<DemoCase>(path + "/" + caseId + "/activate", {
        method: "POST",
        headers: { "X-Demo-Generation": "1" },
      });
      onDemo(c); // Keep recovery controls available even if later client-side loading fails.
      const target =
        base.replace(/\/$/, "") + "/v1/demo/" + mode + "/cases/" + caseId;
      const copies =
        pattern === "normal" ? 1 : pattern === "sequential" ? 5 : 10;
      onLoad({
        version: 1,
        name: mode + " / " + pattern,
        description: "Isolated parcel example.",
        targetUrl: target + "/events",
        headers: {
          "Content-Type": "application/json",
          "X-Demo-Generation": c.generation,
        },
        body: JSON.stringify(
          { schemaVersion: "1", eventId, type: "parcel.delivered", parcelId },
          null,
          2,
        ),
        copies,
        concurrency: pattern === "concurrent" ? copies : 1,
        timeoutMs: 5000,
        expectedStatuses: [200],
        verification: {
          url: target + "/parcels/" + parcelId + "/state",
          headers: { "X-Demo-Generation": c.generation },
          assertions: [
            { pointer: "/state", equals: "DELIVERED" },
            { pointer: "/notificationCount", equals: 1 },
          ],
          pollIntervalMs: 100,
          timeoutMs: 5000,
          requestTimeoutMs: 1000,
          stableForMs: 500,
        },
      });
    }, "Example loaded. Review the request below, then run the test.");
  const close = (reason: string) =>
    onAction(async () => {
      if (!demo) return;
      onDemo(
        await api<DemoCase>(
          `/demo/v1/demo/${demo.mode}/cases/${demo.caseId}/close`,
          {
            method: "POST",
            headers: {
              "Content-Type": "application/json",
              "X-Demo-Generation": demo.generation,
            },
            body: JSON.stringify({ reason }),
          },
        ),
      );
    }, "Example closed. You can prepare a fresh one.");
  return (
    <details className="sample-panel">
      <summary>Try the included parcel example</summary>
      <p>
        Optional: start the demo services with{" "}
        <code>docker compose --profile demo up -d --build --wait</code>. Your
        own backend does not need them.
      </p>
      <div className="two">
        <label>
          Example receiver
          <select
            aria-label="Example receiver"
            value={mode}
            disabled={locked}
            onChange={(e) => setMode(e.target.value)}
          >
            <option value="corrected">Corrected — one notification</option>
            <option value="faulty">Faulty — duplicate notifications</option>
          </select>
        </label>
        <label>
          Example delivery
          <select
            aria-label="Example delivery"
            value={pattern}
            disabled={locked}
            onChange={(e) => setPattern(e.target.value)}
          >
            <option value="normal">Once (1 request)</option>
            <option value="sequential">Repeated (5 requests)</option>
            <option value="concurrent">At the same time (10 requests)</option>
          </select>
        </label>
      </div>
      <details>
        <summary>Example connection settings</summary>
        <label>
          Example receiver URL
          <input
            value={base}
            disabled={locked}
            onChange={(e) => setBase(e.target.value)}
            placeholder="http://receiver:8080"
          />
        </label>
        <p className="help">
          Only change this for native development. A native runner uses
          http://127.0.0.1:8080.
        </p>
      </details>
      <div className="actions">
        <button
          type="button"
          disabled={locked || demo?.lifecycle === "ACTIVE"}
          onClick={() => void prepare()}
        >
          Prepare example
        </button>
        {demo?.lifecycle === "ACTIVE" && (
          <>
            <button
              type="button"
              disabled={locked}
              onClick={() => void close("COMPLETED")}
            >
              Close example
            </button>
            <button
              type="button"
              className="quiet"
              disabled={locked}
              onClick={() => void close("ABANDONED")}
            >
              Abandon example
            </button>
          </>
        )}
      </div>
      {demo && (
        <p className="help">
          {demo.mode} · {demo.lifecycle} · <code>{demo.caseId}</code>. Close an
          active example before preparing the next one.
        </p>
      )}
    </details>
  );
}
