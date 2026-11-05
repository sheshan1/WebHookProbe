import { useEffect, useRef, useState } from "react";
import { api, download, parseScenario, safeScenario, terminal } from "./types";
import type { DemoCase, Report, Scenario } from "./types";
import { emptyForm, fromScenario, toScenario } from "./form";
import type { FormState } from "./form";
import { HeaderFields } from "./components/HeaderFields";
import { ResultChecks } from "./components/ResultChecks";
import { DemoPanel } from "./components/DemoPanel";
import { Results } from "./components/Results";

function savedCase(): DemoCase | undefined {
  try {
    const c = JSON.parse(localStorage.getItem("webhookprobe.case") ?? "null");
    return c &&
      typeof c.caseId === "string" &&
      ["corrected", "faulty"].includes(c.mode) &&
      typeof c.generation === "string" &&
      ["READY", "ACTIVE", "CLOSED"].includes(c.lifecycle)
      ? c
      : undefined;
  } catch {
    return undefined;
  }
}
export function App() {
  const [form, setForm] = useState(emptyForm);
  const [report, setReport] = useState<Report>();
  const [demo, setDemo] = useState<DemoCase | undefined>(savedCase);
  const [pollId, setPollId] = useState(
    localStorage.getItem("webhookprobe.run") ?? "",
  );
  const [recovering, setRecovering] = useState(!!pollId);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [notice, setNotice] = useState("");
  const importRef = useRef<HTMLInputElement>(null);
  const resultRef = useRef<HTMLDivElement>(null);
  const running = !!report && !terminal(report);
  const locked = busy || running || recovering;
  const update = <K extends keyof FormState>(key: K, value: FormState[K]) =>
    setForm((f) => ({ ...f, [key]: value }));
  useEffect(() => {
    if (demo) localStorage.setItem("webhookprobe.case", JSON.stringify(demo));
    else localStorage.removeItem("webhookprobe.case");
  }, [demo]);
  useEffect(() => {
    if (!pollId) return;
    let disposed = false;
    let timer: ReturnType<typeof setTimeout>;
    async function poll() {
      try {
        const r = await api<Report>("/api/v1/runs/" + pollId);
        if (disposed) return;
        setReport(r);
        setRecovering(false);
        if (!terminal(r)) timer = setTimeout(poll, 500);
        else setPollId("");
      } catch (e) {
        if (disposed) return;
        setRecovering(false);
        setError(e instanceof Error ? e.message : String(e));
        timer = setTimeout(poll, 2000);
      }
    }
    void poll();
    return () => {
      disposed = true;
      clearTimeout(timer);
    };
  }, [pollId]);
  useEffect(() => {
    if (localStorage.getItem("webhookprobe.run")) return;
    let alive = true;
    api<Report | null>("/api/v1/runs/active")
      .then((r) => {
        if (alive && r) {
          setReport(r);
          setPollId(r.id);
          localStorage.setItem("webhookprobe.run", r.id);
        }
      })
      .catch(() => {});
    return () => {
      alive = false;
    };
  }, []);
  async function action(fn: () => Promise<void>, message?: string) {
    setBusy(true);
    setError("");
    setNotice("");
    try {
      await fn();
      if (message) setNotice(message);
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      setBusy(false);
    }
  }
  function load(s: Scenario) {
    setForm(fromScenario(s));
  }
  async function run() {
    await action(async () => {
      const s = toScenario(form);
      const r = await api<Report>("/api/v1/runs", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(s),
      });
      setReport(r);
      setPollId(r.id);
      localStorage.setItem("webhookprobe.run", r.id);
      resultRef.current?.scrollIntoView({ behavior: "smooth", block: "start" });
    });
  }
  function clearRun() {
    setPollId("");
    setReport(undefined);
    setRecovering(false);
    localStorage.removeItem("webhookprobe.run");
    setError("");
    setNotice(
      "Stopped watching this run. This does not cancel a running test or undo backend changes.",
    );
  }
  return (
    <>
      <header>
        <a className="brand" href="/">
          WebHookProbe
        </a>
        <span>Test webhook delivery. Check what changed.</span>
      </header>
      <main>
        <section className="intro">
          <h1>Test your webhook in four steps</h1>
          <p>
            Enter your backend’s URL and payload, choose how to send it, then
            review the responses. Add a result check if you want to confirm what
            changed in your backend.
          </p>
          <nav aria-label="Test flow">
            <span>1 · Request</span>
            <span>2 · Delivery</span>
            <span>3 · Result check</span>
            <span>4 · Run & review</span>
          </nav>
        </section>
        {error && (
          <div role="alert" className="message error">
            {error}
            <button
              type="button"
              className="quiet"
              aria-label="Dismiss error"
              onClick={() => setError("")}
            >
              Dismiss
            </button>
          </div>
        )}
        {notice && (
          <div role="status" className="message">
            {notice}
          </div>
        )}
        <div className="file-actions">
          <button
            type="button"
            disabled={locked}
            onClick={() => importRef.current?.click()}
          >
            Import settings
          </button>
          <button
            type="button"
            disabled={locked}
            onClick={() =>
              void action(
                async () =>
                  download(
                    "webhookprobe-settings.json",
                    safeScenario(toScenario(form)),
                  ),
                "Settings exported. Private headers and URL query values were redacted; re-enter them before reuse.",
              )
            }
          >
            Export settings
          </button>
          <button
            type="button"
            className="quiet"
            disabled={locked}
            onClick={() => {
              setForm(emptyForm());
              clearRun();
              setNotice(
                "Request cleared. Example text is a hint, not a saved value.",
              );
              setError("");
            }}
          >
            New test
          </button>
        </div>
        <input
          ref={importRef}
          type="file"
          accept="application/json,.json"
          hidden
          onChange={(e) => {
            const file = e.target.files?.[0];
            if (file)
              void action(async () => {
                if (file.size > 262144)
                  throw new Error("Settings file exceeds 256 KiB.");
                load(parseScenario(JSON.parse(await file.text())));
              }, "Settings imported. Review the inputs and re-enter any redacted credentials before running.");
            e.target.value = "";
          }}
        />
        <form
          onSubmit={(e) => {
            e.preventDefault();
            void run();
          }}
          noValidate
        >
          <fieldset disabled={locked}>
            <section className="panel">
              <div className="section-heading">
                <span className="step">1</span>
                <h2>Build the request</h2>
                <span className="method">POST</span>
              </div>
              <label>
                Webhook URL
                <input
                  type="url"
                  value={form.targetUrl}
                  placeholder="http://host.docker.internal:9000/webhooks"
                  autoComplete="off"
                  onChange={(e) => update("targetUrl", e.target.value)}
                />
              </label>
              <p className="help">
                The runner sends this request. For a backend on this computer,
                use <code>host.docker.internal</code> instead of{" "}
                <code>localhost</code>. Your backend must accept connections
                from Docker.
              </p>
              <label>
                Payload
                <textarea
                  aria-label="Payload"
                  rows={7}
                  value={form.body}
                  placeholder={
                    '{\n  "eventId": "evt-001",\n  "type": "order.paid",\n  "orderId": "order-001"\n}'
                  }
                  spellCheck={false}
                  onChange={(e) => update("body", e.target.value)}
                />
              </label>
              <p className="help">
                Use the data your backend expects. Example text disappears when
                you click and is never sent. Leave blank only if your endpoint
                accepts an empty body.
              </p>
              <details>
                <summary>Request headers</summary>
                <p className="help">
                  Content-Type describes the payload format. Add Authorization
                  only if your backend requires it.
                </p>
                <HeaderFields
                  label="Request"
                  rows={form.headers}
                  onChange={(v) => update("headers", v)}
                  disabled={locked}
                />
              </details>
            </section>
            <section className="panel">
              <div className="section-heading">
                <span className="step">2</span>
                <h2>Choose how to send it</h2>
              </div>
              <div className="presets" aria-label="Delivery presets">
                {[
                  ["Once", 1, 1],
                  ["Repeated", 5, 1],
                  ["At the same time", 10, 10],
                ].map(([name, copies, concurrency]) => (
                  <button
                    type="button"
                    key={name}
                    aria-pressed={
                      form.copies === copies && form.concurrency === concurrency
                    }
                    onClick={() =>
                      setForm((f) => ({
                        ...f,
                        copies: Number(copies),
                        concurrency: Number(concurrency),
                      }))
                    }
                  >
                    {name}
                  </button>
                ))}
              </div>
              <div className="three">
                <label>
                  Number of requests
                  <input
                    type="number"
                    min={1}
                    max={1000}
                    value={Number.isNaN(form.copies) ? "" : form.copies}
                    onChange={(e) => update("copies", e.target.valueAsNumber)}
                  />
                </label>
                <label>
                  Requests at the same time
                  <input
                    type="number"
                    min={1}
                    max={50}
                    value={
                      Number.isNaN(form.concurrency) ? "" : form.concurrency
                    }
                    onChange={(e) =>
                      update("concurrency", e.target.valueAsNumber)
                    }
                  />
                </label>
                <label>
                  Expected HTTP status
                  <input
                    value={form.statuses}
                    placeholder="200, 202"
                    onChange={(e) => update("statuses", e.target.value)}
                  />
                </label>
              </div>
              <p className="help">
                Every request sends the same payload. “1 at the same time” sends
                requests one after another. Start with one request before
                testing duplicates.
              </p>
            </section>
            <section className="panel">
              <div className="section-heading">
                <span className="step">3</span>
                <h2>Check the backend result</h2>
                <span className="optional">Optional</span>
              </div>
              <label className="checkbox">
                <input
                  type="checkbox"
                  checked={form.verify}
                  onChange={(e) => update("verify", e.target.checked)}
                />
                Verify what changed in my backend
              </label>
              <p className="help">
                A successful HTTP response only confirms delivery. To check the
                result, your backend must expose a read-only URL that returns
                JSON.
              </p>
              {form.verify && (
                <>
                  <label>
                    State URL
                    <input
                      type="url"
                      value={form.stateUrl}
                      placeholder="http://host.docker.internal:9000/orders/order-001/state"
                      autoComplete="off"
                      onChange={(e) => update("stateUrl", e.target.value)}
                    />
                  </label>
                  <ResultChecks
                    rows={form.checks}
                    onChange={(v) => update("checks", v)}
                    disabled={locked}
                  />
                  <details>
                    <summary>State request headers</summary>
                    <p className="help">
                      These are separate from the delivery headers. Add
                      credentials here if your read-only endpoint requires them.
                    </p>
                    <HeaderFields
                      label="State"
                      rows={form.stateHeaders}
                      onChange={(v) => update("stateHeaders", v)}
                      disabled={locked}
                    />
                  </details>
                </>
              )}
            </section>
            <details className="panel advanced">
              <summary>Advanced settings</summary>
              <label>
                Test name
                <input
                  value={form.name}
                  placeholder="Order payment duplicate check"
                  onChange={(e) => update("name", e.target.value)}
                />
              </label>
              <label>
                Request timeout (ms)
                <input
                  type="number"
                  value={Number.isNaN(form.timeoutMs) ? "" : form.timeoutMs}
                  onChange={(e) => update("timeoutMs", e.target.valueAsNumber)}
                />
              </label>
              {form.verify && (
                <>
                  <p className="help">
                    Checks start after delivery. All expected values must keep
                    matching during the stability window.
                  </p>
                  <div className="two">
                    <label>
                      Check deadline (ms)
                      <input
                        type="number"
                        value={
                          Number.isNaN(form.verificationTimeoutMs)
                            ? ""
                            : form.verificationTimeoutMs
                        }
                        onChange={(e) =>
                          update(
                            "verificationTimeoutMs",
                            e.target.valueAsNumber,
                          )
                        }
                      />
                    </label>
                    <label>
                      Poll interval (ms)
                      <input
                        type="number"
                        value={
                          Number.isNaN(form.pollIntervalMs)
                            ? ""
                            : form.pollIntervalMs
                        }
                        onChange={(e) =>
                          update("pollIntervalMs", e.target.valueAsNumber)
                        }
                      />
                    </label>
                    <label>
                      State request timeout (ms)
                      <input
                        type="number"
                        value={
                          Number.isNaN(form.requestTimeoutMs)
                            ? ""
                            : form.requestTimeoutMs
                        }
                        onChange={(e) =>
                          update("requestTimeoutMs", e.target.valueAsNumber)
                        }
                      />
                    </label>
                    <label>
                      Matching window (ms)
                      <input
                        type="number"
                        value={
                          Number.isNaN(form.stableForMs) ? "" : form.stableForMs
                        }
                        onChange={(e) =>
                          update("stableForMs", e.target.valueAsNumber)
                        }
                      />
                    </label>
                  </div>
                </>
              )}
            </details>
          </fieldset>
          <section className="run-bar">
            <div>
              <h2>4. Run and review</h2>
              <p>
                The request will use your entered values. Each run tests one
                payload.
              </p>
            </div>
            <button className="primary" type="submit" disabled={locked}>
              {busy
                ? "Working…"
                : running
                  ? "Test running…"
                  : recovering
                    ? "Reconnecting…"
                    : "Run test"}
            </button>
            {running && (
              <button
                type="button"
                disabled={busy}
                onClick={() =>
                  void action(async () => {
                    await api("/api/v1/runs/" + report.id + "/cancel", {
                      method: "POST",
                    });
                  }, "Stop requested. In-flight requests may already have changed your backend.")
                }
              >
                Stop test
              </button>
            )}
          </section>
        </form>
        <div ref={resultRef}>
          <Results
            report={report}
            locked={locked}
            onAction={action}
            onLoad={(r) => {
              load(r.scenario);
              setNotice(
                "Settings loaded. Re-enter redacted credentials and use fresh target data for an independent test.",
              );
            }}
          />
        </div>
        {pollId && (
          <button type="button" className="quiet clear-run" onClick={clearRun}>
            Stop watching / clear unavailable run
          </button>
        )}
        <DemoPanel
          locked={locked}
          demo={demo}
          onDemo={setDemo}
          onLoad={load}
          onAction={action}
        />
      </main>
      <footer>WebHookProbe · Runs locally · No account required</footer>
    </>
  );
}
