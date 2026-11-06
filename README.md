# WebHookProbe

Send a webhook to your backend, repeat it on purpose, and see whether your backend handles it correctly.

A **webhook** is an HTTP request carrying an event, such as “an order was paid.” WebHookProbe acts as the sender. You supply the destination, payload and expected result. It works with a backend written in any language that accepts HTTP POST requests.

**The flow:** build the request → choose delivery settings → optionally check your backend's state → run and review.

## What you need

- Docker with Docker Compose installed and running.
- This project folder, downloaded from [GitHub](https://github.com/sheshan1/WebHookProbe) as a ZIP and extracted, or checked out with Git.
- A browser.

You do **not** need Java, Go, Node.js, Python or PostgreSQL installed to run the application with Docker. The normal application starts only the web interface and the Go runner. The Java receiver and database are optional examples.

## Start the application

1. Open a terminal in the project folder: the folder containing `compose.yaml`. If you use Git, you can obtain it with:

   ```bash
   git clone https://github.com/sheshan1/WebHookProbe.git
   cd WebHookProbe
   ```

2. Check that Docker is ready:

   ```bash
   docker --version
   docker compose version
   ```

   Both commands should show a version. If Docker reports that it cannot connect, start Docker Desktop or the Docker service first.

3. Start WebHookProbe:

   ```bash
   docker compose up -d --build --wait
   ```

   The first run downloads and builds software, so it takes longer. Wait for the command to finish successfully.

4. Open **http://127.0.0.1:8082** in your browser.
5. Check service status if necessary:

   ```bash
   docker compose ps
   ```

   `frontend` and `runner` should be healthy. No account or registration is needed.

No `.env` file is required for the defaults. To change the web port, copy `.env.example` to `.env`, set `UI_PORT` to another free port, and start again. For example, `UI_PORT=8085` changes the address to `http://127.0.0.1:8085`.

**Don't have a backend yet?** Follow [Sample tests](docs/SAMPLE_TESTS.md). It supplies a local example receiver, complete payloads and expected outputs.

## Connect your own backend

You need an endpoint that accepts POST, for example `http://localhost:9000/webhooks`. Start your backend as you normally would. Use test data because repeated requests can trigger real business actions in your application.

The request comes from the **runner**, which is inside Docker. It does not come directly from the browser. Choose the address accordingly:

| Where your backend runs | Webhook URL to enter |
|---|---|
| On the same computer, port 9000 | `http://host.docker.internal:9000/webhooks` |
| At a reachable HTTPS address | `https://your-test-server.example/webhooks` |
| In another container on the same Docker network | `http://your-service-name:9000/webhooks` |

For a backend on your computer, it must accept connections from Docker. A server bound only to `127.0.0.1` may reject them. Configure its development server to listen on a Docker-reachable interface, commonly `0.0.0.0`, while keeping your normal access controls. The supplied Compose file adds the `host.docker.internal` host mapping used on Linux. Your own backend may also require firewall or network changes. If it must remain loopback-only, use the native CLI described in the [architecture reference](docs/ARCHITECTURE.md#native-development-and-cli).

For an existing Docker Compose backend, attach the runner to that backend's network using a local override. For example, if `docker network ls` shows a network named `myapp_default`, create `compose.backend.yaml`:

```yaml
services:
  runner:
    networks:
      - default
      - backend
networks:
  backend:
    external: true
    name: myapp_default
```

Start with `docker compose -f compose.yaml -f compose.backend.yaml up -d --build --wait`. If your backend service is named `api`, use `http://api:9000/webhooks`. WebHookProbe does not start or modify that application's services or database.

You do **not** need to:

- Install a WebHookProbe SDK or library in your application.
- Use our parcel payload format, event names, database tables or demo headers.
- Start our Java receiver, PostgreSQL database or sample Python receiver.
- Give the runner access to your database or application source code.

For HTTP delivery testing, your existing endpoint is enough. For automatic business-result verification, provide a separate read-only HTTP endpoint as explained below. WebHookProbe cannot discover your business rules by itself.

## 1. Build the request

Enter the **Webhook URL** that your backend accepts. Replace the example with your own address.

Paste the exact **Payload** your backend expects. For example, an order system might accept:

```json
{
  "eventId": "evt-001",
  "type": "order.paid",
  "orderId": "order-001",
  "amount": 49.99,
  "currency": "USD"
}
```

This is an illustration, not a required format. Your backend decides the field names, types and required values. WebHookProbe sends the body as entered; it does not add event fields or repair invalid JSON. To test another payload, create another run. Every copy within a run uses the same payload.

Example text in empty fields is a **placeholder**. It disappears on focus and is never sent. Typed, imported or prepared values remain when you click the field. Operational defaults such as one request and status 200 are real settings, not disappearing examples.

Open **Request headers** if your backend needs them:

| Header name | Header value | When needed |
|---|---|---|
| `Content-Type` | `application/json` | JSON request body; included by default |
| `Authorization` | `Bearer YOUR_TEST_TOKEN` | Your endpoint requires a bearer token |
| `X-Api-Key` | Your test API key | Your endpoint uses API-key authentication |

Use **Add request header** to add a row. Remove headers that do not apply. No provider-specific webhook signatures are generated: if your backend requires HMAC signatures, you must prepare valid headers for the exact payload yourself or use a suitable test endpoint.

## 2. Choose delivery settings

Start with **Once**:

- **Number of requests:** how many copies to send.
- **Requests at the same time:** the maximum number in flight. `1` sends them one after another.
- **Expected HTTP status:** the status your endpoint should return, usually 200, 201, 202 or 204. For several acceptable statuses, enter `200, 202`.

The presets fill these settings:

| Preset | Requests | At the same time | Purpose |
|---|---|---|---|
| Once | 1 | 1 | Confirm the endpoint and payload work |
| Repeated | 5 | 1 | Check repeated delivery of the same event |
| At the same time | 10 | 10 | Check simultaneous copies of the same event |

The runner does not automatically retry failed requests or follow redirects. An expected202 means your backend accepted the request; it does not prove background processing finished.

**Advanced settings** contains a test name and timeout values. Keep the defaults for your first test. The normal request timeout is 5000 ms, which is 5 seconds.

## 3. Optionally verify what changed

Without this step, WebHookProbe checks HTTP responses only. The result says **Not checked** for backend state.

For automatic verification, your backend must expose a read-only **GET** endpoint returning JSON. For example:

```text
GET /orders/order-001/state
```

```json
{
  "status": "PAID",
  "notificationCount": 1
}
```

1. Enable **Verify what changed in my backend**.
2. Enter the **State URL**, such as `http://host.docker.internal:9000/orders/order-001/state`.
3. Add result checks:

   | Field path | Value type | Expected value |
   |---|---|---|
   | `/status` | Text | `PAID` |
   | `/notificationCount` | Number | `1` |

4. If the state endpoint needs authentication, open **State request headers** and add it there too. Delivery headers are not automatically copied to this separate endpoint.

A field path selects a value from the JSON response. Nested fields use `/order/status`; array elements use `/items/0/id`. Types matter: the number 1 differs from the text "1". Add as many checks as needed, up to 20.

After delivery finishes, the runner polls the state endpoint. With default settings, all checks must keep matching over 500 ms within a 5-second deadline. This allows a short processing delay. It cannot guarantee that the state will never change later. The state URL must describe the particular record/event you are testing, not an unrelated global counter.

If you cannot provide such an endpoint, leave verification off and inspect your own logs, database or application manually. WebHookProbe never connects directly to your database.

## 4. Run and read the results

Click **Run test**. Inputs are locked while the run is active. The page shows two separate results:

| Result | Meaning |
|---|---|
| HTTP delivery: **Expected responses** | Every requested copy returned an acceptable status |
| HTTP delivery: **Unexpected result** | An unexpected status, connection failure, timeout or incomplete delivery occurred |
| Backend result: **Passed** | All configured checks matched over their observation window |
| Backend result: **Failed** | Readable state did not satisfy the checks |
| Backend result: **Inconclusive** | The runner could not establish a stable result, for example because state was missing or unavailable |
| Backend result: **Not checked** | No state verification was configured |

The values table shows **expected versus observed** state. The attempts table shows each request's status, time and outcome. Click a request number to inspect its response.

The overall verdict is available in **Run details and verification history** and in downloaded reports. **Passed** requires successful delivery and a passed backend check. Successful delivery without verification has overall verdict **inconclusive**, not a false business pass. `completed` only means execution finished.

Use **Stop test** to cancel future/in-flight work. It cannot undo an action your backend already committed. You can download a **partial report** during execution or a final **report** afterward.

## Save and repeat

- **Export settings:** save this request and delivery/check configuration as JSON.
- **Import settings:** load a saved file for review.
- **Use these settings again:** load the previous run's configuration into the form.
- **New test:** start with empty example fields and normal defaults.

Private header values and URL query values are redacted in exports/reports. Re-enter them before running an exported configuration. Inspect files before sharing: arbitrary payload/response data may contain other sensitive values the tool cannot recognize.

For a duplicate test, keep the same event identity your backend uses. For an independent test, create fresh target records/event IDs or reset your test data using your own application's tools. WebHookProbe does not automatically reset your backend.

## Stop, restart and troubleshoot

```bash
docker compose stop
docker compose start
```

`docker compose down` removes containers and the network. Exported files on your computer remain. Runner history is held in memory, so restart loses unexported runs. Optional parcel-demo database data remains in its Docker volume; ordinary shutdown does not delete it.

| Problem | What to check |
|---|---|
| Browser cannot open 8082 | Run `docker compose ps`; check Docker is running and the startup command finished |
| Port already in use | Set another UI_PORT in `.env`, then run the startup command again |
| Transport error / no response | Check the URL from the runner's network, backend listening interface, service port and firewall |
| HTTP 404 | Use the actual webhook route; do not copy a browser page URL |
| HTTP 401/403 | Supply the authentication headers your endpoint requires |
| HTTP 400/422 | Compare your payload with your backend's contract; inspect the response body |
| Timeout | Check backend speed and the timeout in Advanced settings; the action may still have committed |
| Backend result is inconclusive | Check the state URL, authentication, JSON field paths and observation deadline |
| RUN_ACTIVE /409 | Another run is active; finish or stop it first. One API run is allowed at a time |
| Old run unavailable after restart | Clear the unavailable run and start a new one; use previously downloaded reports |

For service logs:

```bash
docker compose logs --tail=80 frontend runner
```

## More detail

- [Sample tests](docs/SAMPLE_TESTS.md): copyable payloads, happy/failure paths, cancellation and expected results.
- [Architecture](docs/ARCHITECTURE.md): components, flows, APIs, schema, transactions, deployment and developer checks.

First-release limits: POST delivery only; one payload per run; one active API run; up to 1000 copies and 50 concurrent requests; no automatic retries, signature generation, user accounts or public hosting. Run locally against endpoints you are authorized to test.

## License

Licensed under the [MIT License](LICENSE).
