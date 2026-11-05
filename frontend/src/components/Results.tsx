import { useState } from "react";
import { api, download, terminal } from "../types";
import type { Attempt, Report } from "../types";
const businessLabel: Record<string, string> = {
  not_configured: "Not checked",
  passed: "Passed",
  failed: "Failed",
  inconclusive: "Inconclusive",
};
export function Results({
  report,
  onLoad,
  onAction,
  locked,
}: {
  report: Report | undefined;
  onLoad: (r: Report) => void;
  onAction: (fn: () => Promise<void>, message?: string) => Promise<void>;
  locked: boolean;
}) {
  const [selected, setSelected] = useState<Attempt>();
  if (!report)
    return (
      <section className="results-empty" aria-label="Results">
        <h2>Results will appear here</h2>
        <p>
          Run a test to see each response and, if enabled, the result of your
          backend check.
        </p>
      </section>
    );
  const businessOutcome =
    report.scenario.verification &&
    !report.finishedAt &&
    report.business.outcome === "not_configured"
      ? "pending"
      : report.business.outcome;
  const latest =
    report.business.observations.filter((o) => o.checks?.length).at(-1)
      ?.checks ?? [];
  const deliveryDone =
    !!report.finishedAt || report.attempts.length === report.scenario.copies;
  return (
    <section className="panel results" aria-label="Results">
      <div className="section-heading">
        <h2>Test results</h2>
        <span className={"badge " + report.verdict}>{report.state}</span>
      </div>
      <p>{report.scenario.name}</p>
      <div className="metrics">
        <div>
          <h3>1. HTTP delivery</h3>
          <strong>
            {deliveryDone
              ? report.deliveryPassed
                ? "Expected responses"
                : "Unexpected result"
              : `${report.attempts.length} of ${report.scenario.copies} finished`}
          </strong>
          <p>
            {
              report.attempts.filter((a) => a.outcome === "expected_status")
                .length
            }{" "}
            expected responses · {report.unstarted} not started
          </p>
        </div>
        <div>
          <h3>2. Backend result</h3>
          <strong className={businessOutcome}>
            {businessLabel[businessOutcome] ?? "Checking…"}
          </strong>
          <p>
            {businessOutcome === "not_configured"
              ? "No state check configured. Successful HTTP responses do not prove the backend changed correctly."
              : report.business.detail || "Waiting for the configured checks."}
          </p>
        </div>
      </div>
      {latest.length > 0 && (
        <div className="table-scroll">
          <table>
            <caption>Expected and observed backend values</caption>
            <thead>
              <tr>
                <th>Field</th>
                <th>Expected</th>
                <th>Observed</th>
                <th>Check</th>
              </tr>
            </thead>
            <tbody>
              {latest.map((c, i) => (
                <tr key={i}>
                  <td>
                    <code>{c.pointer || "(whole response)"}</code>
                  </td>
                  <td>{JSON.stringify(c.expected)}</td>
                  <td>
                    {c.found ? JSON.stringify(c.actual) : "Field missing"}
                  </td>
                  <td>{c.matched ? "Match" : "No match"}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
      <div className="table-scroll">
        <table>
          <caption>Delivery attempts</caption>
          <thead>
            <tr>
              <th>Request</th>
              <th>HTTP status</th>
              <th>Time</th>
              <th>Result</th>
            </tr>
          </thead>
          <tbody>
            {report.attempts.map((a) => (
              <tr key={a.id}>
                <td>
                  <button
                    type="button"
                    className="quiet"
                    onClick={() => setSelected(a)}
                  >
                    #{a.id}
                  </button>
                </td>
                <td>{a.status ?? "No response"}</td>
                <td>{a.durationMs} ms</td>
                <td>{a.outcome.replaceAll("_", " ")}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
      {selected &&
        report.attempts.some((a) => a.startedAt === selected.startedAt) && (
          <details open className="response">
            <summary>
              Request #{selected.id} response{" "}
              {selected.truncated ? "(first 4 KiB)" : ""}
            </summary>
            <pre>{selected.preview || "No response body received."}</pre>
          </details>
        )}
      <div className="actions">
        <button
          type="button"
          onClick={() =>
            void onAction(async () =>
              download(
                "webhookprobe-" + report.id + ".json",
                await api<Report>("/api/v1/runs/" + report.id + "/report"),
              ),
            )
          }
        >
          Download {report.finishedAt ? "report" : "partial report"}
        </button>
        <button
          type="button"
          disabled={!terminal(report) || locked}
          onClick={() => onLoad(report)}
        >
          Use these settings again
        </button>
      </div>
      <details className="technical">
        <summary>Run details and verification history</summary>
        <p>
          Overall verdict: <b>{report.verdict}</b> · Run ID:{" "}
          <code>{report.id}</code>
        </p>
        <p>
          Started: {report.startedAt} · Finished:{" "}
          {report.finishedAt ?? "Still running"} · Omitted observations:{" "}
          {report.business.droppedObservations}
        </p>
        <pre>{JSON.stringify(report.business.observations, null, 2)}</pre>
        <p>
          Stopping a request cannot undo changes already made by the backend. A
          passing check covers only its configured observation window.
        </p>
      </details>
    </section>
  );
}
