# Sample tests: inputs, actions and expected results

Use these examples to learn the UI before connecting another backend. Run commands from the folder containing compose.yaml. The samples use synthetic data and do not require your own application.

## 1. Start the simple sample receiver

```bash
docker compose -f compose.yaml -f compose.samples.yaml up -d --build --wait
```

Open **http://127.0.0.1:8082**. The sample receiver runs alongside the two core services. Its host health URL is **http://127.0.0.1:8090/health**. This sample needs no Java or PostgreSQL.

The runner uses `http://fixture:8090` because it is inside Docker. If port8090 is occupied, set FIXTURE_PORT to another free host port in `.env`; the runner still uses8090 internally.

## 2. Enter a complete successful test by hand

Click **New test** and enter:

| Input | Value |
|---|---|
| Webhook URL | `http://fixture:8090/webhook` |
| Request header | Content-Type = application/json (default) |
| Number of requests | 1 |
| Requests at the same time | 1 |
| Expected HTTP status | 200 |

Payload:

```json
{
  "eventId": "sample-order-001"
}
```

Enable **Verify what changed in my backend**:

| Input | Value |
|---|---|
| State URL | `http://fixture:8090/state/sample-order-001` |
| Field path | `/notificationCount` |
| Value type | Number |
| Expected value | 1 |
| State request headers | None |

Click **Run test**. Expected:

- One200 response with `{"duplicate": false}` on its first delivery.
- HTTP delivery **Expected responses**.
- Backend result **Passed**; expected1, observed1, Match.
- Overall verdict passed in Run details/report.

The corresponding read-only state response is:

```json
{
  "notificationCount": 1
}
```

You can inspect it directly in your browser at **http://127.0.0.1:8090/state/sample-order-001**. The same URL inside the runner uses the service name fixture instead of127.0.0.1.

## 3. Repeat the same event

Keep the same payload and state URL. Select **Repeated**, then Run. Repeat with **At the same time**.

| Preset | Attempts | HTTP | notificationCount | Backend result |
|---|---|---|---|---|
| Repeated | 5 | All200 | 1 | Passed |
| At the same time | 10 | All200 | 1 | Passed |

Later responses have `duplicate:true`. The sample receiver remembers the event ID and performs its simulated action once. Restarting only this sample receiver clears its in-memory event state:

```bash
docker compose -f compose.yaml -f compose.samples.yaml restart fixture
```

Use another eventId and matching state URL for a fresh independent test. Each sample event is independent of other IDs.

## 4. Observe failures and inconclusive checks

Each row below uses the payload `{"eventId":"sample-order-001"}` and the default Content-Type header. Use Number /notificationCount =1 for any configured check. Keep other settings at their defaults unless specified. Click New test before each row, then fill the stated values.

| Case | Webhook URL suffix after http://fixture:8090 | State URL suffix | Delivery settings | Expected result |
|---|---|---|---|---|
| No state check | /webhook | Verification off | Once | HTTP succeeds; backend **Not checked**; overall inconclusive |
| Wrong business value | /webhook | /mismatch/sample-order-001 | Once | HTTP200; observed2 instead of1; backend **Failed** after about5s |
| Bad JSON from state URL | /webhook | /invalid/sample-order-001 | Once | HTTP200 delivery; backend **Inconclusive** |
| Missing field | /webhook | /missing/sample-order-001 | Once | HTTP200 delivery; state{}; backend **Inconclusive** |
| State endpoint unavailable | /webhook | /unavailable/sample-order-001 | Once | Delivery succeeds; state503; backend **Inconclusive** |
| Slow delivery | /slow | Verification off | Once; request timeout100ms in Advanced settings | No response before deadline; timeout; delivery fails |
| Unreachable target | Use full URL http://fixture:1/webhook | Verification off | Once | No status; transport error; delivery fails |
| HTTP error | /error | Verification off | Once | HTTP503; unexpected status; delivery fails |
| Redirect | /redirect | Verification off | Once | HTTP307; runner does not follow it; delivery fails |
| Large response | /large | Verification off | Once | HTTP200; response preview truncated at4096 bytes; backend Not checked |

For a delayed success, use a **fresh** event, for example `{"eventId":"delayed-order-002"}`:

- Webhook URL: `http://fixture:8090/webhook`
- State URL: `http://fixture:8090/delayed/delayed-order-002`
- Check: Number /notificationCount =1.

Expected: initial state0, then1 after approximately1.2 seconds; backend Passed once it remains matching for500ms. Reusing an already accepted event may skip the delay, so change the ID or restart the fixture first.

To see typed comparisons, send an accepted event and change Value type to **Text**, expected value `1`. Actual state is the number1, so the text"1" does not match. The result fails despite a successful delivery.

## 5. Stop during delivery or verification

### Delivery

Use `http://fixture:8090/slow`, payload `{"eventId":"stop-order-003"}`, verification off. Enter100 requests,2 at the same time, request timeout10000ms.

Run, immediately choose **Download partial report**, then **Stop test**.

Expected: state cancelled, overall inconclusive, positive not-started count; partial JSON has finishedAt=null. A stopped caller cannot undo remote actions: the slow sample may still accept a request after the caller disconnects. Inspect `/state/stop-order-003` after5 seconds to see that possibility.

### Verification

Use /webhook with payload `{"eventId":"stop-check-004"}` and State URL /mismatch/stop-check-004. Check Number /notificationCount =1. Set Check deadline30000ms in Advanced settings.

Run and wait for state verifying, then Stop test. Expected: successful delivery stays recorded; backend result Inconclusive; run cancelled. Stopping is not a rollback.

A second API submission during either active run receives409 RUN_ACTIVE. The UI disables conflicting actions while a run is active.

## 6. Save, import and inspect redaction

Use Webhook URL `http://fixture:8090/echo`, payload `{"eventId":"secret-check-005"}`, verification off. Add a request header:

```text
Authorization: Bearer sample-dummy-secret
```

Run and download the report. Search the file for sample-dummy-secret: it should not appear. The saved header and reflected response contain [REDACTED].

Export settings, import that file, then Run. Expected: a validation error asking you to re-enter redacted credentials. Replace the placeholder with a dummy value to continue. Never use real credentials in a shared sample.

Importing malformed settings JSON should show an error and leave the form usable. Clicking inside an empty payload/URL shows no actual value: placeholders are only hints. Clicking a populated field must preserve your input.

## 7. Optional Java/PostgreSQL parcel example

This example demonstrates a deliberately faulty receiver next to a corrected implementation backed by transactions and database constraints.

```bash
docker compose --profile demo up -d --build --wait
```

At the bottom of the UI expand **Try the included parcel example**. For every row:

1. Choose Example receiver and Example delivery.
2. Click **Prepare example**. It creates fresh parcel/event IDs and fills the main form with real input values.
3. Inspect the payload and state checks, then click **Run test**.
4. Inspect HTTP delivery and the expected/observed values.
5. Download the report and click **Close example** before preparing the next row.

| Receiver | Delivery | HTTP responses | Notification count | Backend result |
|---|---|---|---|---|
| Corrected | Once | 1 ×200 | 1 | Passed |
| Corrected | Repeated | 5 ×200 | 1 | Passed |
| Corrected | At the same time | 10 ×200 | 1 | Passed |
| Faulty | Once | 1 ×200 | 1 | Passed |
| Faulty | Repeated | 5 ×200 | 5 | Failed |
| Faulty | At the same time | 10 ×200 | 10 | Failed |

A prepared payload has this shape (the actual IDs are generated for your case):

```json
{
  "schemaVersion": "1",
  "eventId": "evt-generated-id-1",
  "type": "parcel.delivered",
  "parcelId": "parcel-generated-id-1"
}
```

This shape belongs only to the Java example. Do not add these fields or its X-Demo-Generation header to an unrelated backend.

Corrected responses contain PROCESSED once, then DUPLICATE. Two different accepted event IDs for an already delivered parcel produce ALREADY_DELIVERED without another notification. Reusing one event ID with different valid content returns409 EVENT_ID_CONFLICT.

Faulty mode deliberately creates a notification per delivery; HTTP200 still appears successful. The independent state check reveals the failure. The notification is a database record, not a real email/SMS.

Abandon example explicitly closes a stranded ACTIVE case after external work has stopped. Closing and resetting do not happen implicitly when a run is cancelled. The architecture reference documents reset/generation protection and all Java routes.

## 8. Test invalid payloads and event conflicts

### Generic sample receiver

Use Webhook URL http://fixture:8090/webhook, Once, default JSON header and verification off. Try these payloads separately:

| Payload | Expected response |
|---|---|
| `{}` | 400, eventId string required |
| `{"eventId": 12}` | 400, eventId string required |
| `{` | 400, malformed JSON |
| `{"eventId": "valid-example"}` | 200 |

Keeping Expected HTTP status200 makes the first three show Unexpected result. Set it to400 if your test's goal is to prove that an invalid input is rejected correctly. With verification off, backend state remains Not checked even when that HTTP rejection is expected.

### Strict Java example

Prepare a fresh corrected example, select Once in the main form, and keep its URL and generated headers. Replace only the payload. These complete negative examples reject before parcel lookup, so their syntactically valid placeholder parcel ID does not need to exist:

Missing fields →422 PAYLOAD_VALIDATION_FAILED:

```json
{}
```

Unsupported version →422 UNSUPPORTED_SCHEMA_VERSION:

```json
{"schemaVersion":"2","eventId":"negative-version","type":"parcel.delivered","parcelId":"parcel-example"}
```

Unsupported event type →422 UNSUPPORTED_EVENT_TYPE:

```json
{"schemaVersion":"1","eventId":"negative-type","type":"parcel.unknown","parcelId":"parcel-example"}
```

Wrong field type →422 PAYLOAD_VALIDATION_FAILED:

```json
{"schemaVersion":1,"eventId":"negative-type","type":"parcel.delivered","parcelId":"parcel-example"}
```

Unknown property →400 INVALID_REQUEST:

```json
{"schemaVersion":"1","eventId":"negative-field","type":"parcel.delivered","parcelId":"parcel-example","extra":true}
```

Duplicate property →400 INVALID_JSON (send literally, without reformatting away the duplicate):

```text
{"eventId":"first","eventId":"second"}
```

The attempts' response previews contain the error code and request ID. No notification or accepted event should be created. The fresh case remains IN_TRANSIT with notificationCount0. Its originally prepared DELIVERED/1 checks therefore fail intentionally. To verify rejection without mutation, set Expected HTTP status to the error status and change the checks to Text /state=IN_TRANSIT and Number /notificationCount=0; then both delivery expectations and backend checks pass.

For identity behavior, prepare another fresh corrected example and keep a copy of its generated valid payload:

1. Send it unchanged →200 PROCESSED, count1.
2. Send it again →200 DUPLICATE, count1.
3. Change only eventId to another valid ID →200 ALREADY_DELIVERED, still count1.
4. Restore the original eventId and change only parcelId to unknown-parcel →409 EVENT_ID_CONFLICT, count1. Identity conflict takes priority over lookup of that changed parcel.
5. Use a new eventId with unknown-parcel →404 PARCEL_NOT_FOUND, no accepted event for that new ID.

Close the example after finishing. The live verifier below automates these invariants and additional context/reset errors.

## 9. Generate ready-to-import files (optional Python3)

Instead of typing each failure case, generate14 complete settings files:

```bash
python3 scripts/sample_cases.py generate
```

Select **Import settings** and choose a JSON file from `.local/samples/`. Show hidden files or paste the full path if your file chooser hides that directory.

Files: verified, delivery-only, mismatch, invalid-state, missing-state, delayed, timeout, unreachable, http-error, redirect, large-response, redaction, cancel-delivery, cancel-verification. The two cancel cases require you to click Stop test; otherwise they continue until completion/deadline.

For complete HTTP regression checks, start the sample receiver and optional Java demo together, then run the verifier while the API is idle:

```bash
docker compose --profile demo -f compose.yaml -f compose.samples.yaml up -d --build --wait
python3 scripts/sample_cases.py verify
```

Expected:42/42 workflow checks passed, exit0. Results and per-scenario reports go to `.local/samples/`. This includes validation failures, duplicate identity, reset/stale-generation protection, delayed/missing/malformed verification, redaction, cancellation, partial reports and single-active admission. The script creates synthetic cases; it does not reset your own backend.

## 10. Restart and dependency checks

Download a report first. Run a long verification case, reload the browser and confirm the same background run continues. Stop it, then:

```bash
docker compose restart runner
```

Reload. Expected: the old in-memory run is unavailable; use **Stop watching / clear unavailable run**. Your downloaded report remains. Java demo records, if running, remain in PostgreSQL.

Use a separate disposable project for outages. First copy .env.example to .env.sandbox and set UI_PORT=18082 in that file so it does not conflict with the main application:

```bash
docker compose --env-file .env.sandbox --profile demo -p webhookprobe-sandbox up -d --build --wait
```

Open **http://127.0.0.1:18082**. Do not run outage checks against another application's database.

Stop its db service with `docker compose -p webhookprobe-sandbox stop db`. Expected: receiver readiness503, liveness200, and event processing503 DATABASE_UNAVAILABLE. `docker compose -p webhookprobe-sandbox start db` restores readiness and committed state after recovery.

To inspect health through the UI proxy, use `/demo/actuator/health/readiness` and `/demo/actuator/health/liveness` on the sandbox's web address. The health response is compact JSON rather than a delivery report.

## 11. Stop the samples

```bash
docker compose --profile demo -f compose.yaml -f compose.samples.yaml down
```

This stops core and optional sample services while preserving the Java demo volume. Do not add -v unless you intentionally want to delete that demo history. Generated settings and downloaded files on your computer remain.

For automated source/browser checks, API contracts, reset requests and database details, see [Architecture](ARCHITECTURE.md). These examples do not replace a full accessibility audit or a fresh-machine setup test by another developer.
