package dev.webhookprobe.receiver;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertTrue;
import static org.junit.jupiter.api.Assertions.assertThrows;
import com.fasterxml.jackson.databind.JsonNode;
import java.net.URI;
import java.net.http.HttpClient;
import java.net.http.HttpRequest;
import java.net.http.HttpResponse;
import java.util.ArrayList;
import java.util.List;
import java.util.Map;
import java.util.UUID;
import java.util.concurrent.Executors;
import java.util.concurrent.atomic.AtomicBoolean;
import org.junit.jupiter.api.Test;
import org.junit.jupiter.api.condition.EnabledIfEnvironmentVariable;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.boot.test.context.SpringBootTest;
import org.springframework.boot.test.context.TestConfiguration;
import org.springframework.context.annotation.Bean;
import org.springframework.context.annotation.Import;
import org.springframework.context.annotation.Primary;
import org.springframework.core.env.Environment;
import org.springframework.jdbc.core.JdbcTemplate;
import org.springframework.test.context.ActiveProfiles;

@SpringBootTest(webEnvironment = SpringBootTest.WebEnvironment.RANDOM_PORT,
        properties = "webhookprobe.demo.enabled=true")
@ActiveProfiles("test")
@Import(ReceiverIntegrationTests.FaultConfig.class)
@EnabledIfEnvironmentVariable(named = "RUN_RECEIVER_IT", matches = "true")
class ReceiverIntegrationTests {

    @Autowired
    Environment env;

    @Autowired
    JdbcTemplate db;

    private final HttpClient client = HttpClient.newHttpClient();
    static final AtomicBoolean FAIL = new AtomicBoolean();

    @TestConfiguration
    static class FaultConfig {

        @Bean
        @Primary
        ProcessingFault injectedFault() {
            return new ProcessingFault() {
                @Override
                public void beforeNotification() {
                    if (FAIL.get())
                        throw new ApiProblem(500, "PROCESSING_FAILED", "Controlled test failure.");
                }
            };
        }

    }

    String base() {
        return "http://127.0.0.1:" + env.getProperty("local.server.port");
    }

    HttpResponse<String> request(String method, String path, String body, String generation) throws Exception {
        var b = HttpRequest.newBuilder(URI.create(base() + path));
        if (generation != null)
            b.header("X-Demo-Generation", generation);
        if (body != null)
            b.header("Content-Type", "application/json");
        return client.send(b
            .method(method,
                    body == null ? HttpRequest.BodyPublishers.noBody() : HttpRequest.BodyPublishers.ofString(body))
            .build(), HttpResponse.BodyHandlers.ofString());
    }

    String fresh(String mode) throws Exception {
        String id = "case-" + UUID.randomUUID().toString().replace("-", "");
        String path = "/v1/demo/" + mode + "/cases/" + id;
        assertEquals(201, request("POST", "/v1/demo/" + mode + "/cases",
                "{\"caseId\":\"" + id + "\",\"parcelIds\":[\"p1\",\"p2\"]}", null)
            .statusCode());
        assertEquals(200, request("POST", path + "/activate", null, "1").statusCode());
        return path;
    }

    String event(String id, String parcel) {
        return "{\"schemaVersion\":\"1\",\"eventId\":\"" + id + "\",\"type\":\"parcel.delivered\",\"parcelId\":\""
                + parcel + "\"}";
    }

    JsonNode state(String path) throws Exception {
        return JsonInput.MAPPER.readTree(request("GET", path + "/parcels/p1/state", null, "1").body());
    }

    @Test
    void sequentialAndConcurrentDuplicateModes() throws Exception {
        for (String mode : List.of("corrected", "faulty")) {
            String path = fresh(mode);
            String body = event("e1", "p1");
            try (var pool = Executors.newFixedThreadPool(10)) {
                var jobs = new ArrayList<java.util.concurrent.Callable<Integer>>();
                for (int i = 0; i < 10; i++)
                    jobs.add(() -> request("POST", path + "/events", body, "1").statusCode());
                for (var f : pool.invokeAll(jobs))
                    assertEquals(200, f.get());
            }
            assertEquals(mode.equals("corrected") ? 1 : 10, state(path).get("notificationCount").asInt());
            assertEquals(200, request("POST", path + "/events", body, "1").statusCode());
            assertEquals(mode.equals("corrected") ? 1 : 11, state(path).get("notificationCount").asInt());
        }
    }

    @Test
    void distinctEventsConflictAndSemanticEquality() throws Exception {
        String path = fresh("corrected");
        assertEquals(200, request("POST", path + "/events", event("e1", "p1"), "1").statusCode());
        var reordered = request("POST", path + "/events",
                "{ \"parcelId\":\"p1\",\"type\":\"parcel.delivered\",\"eventId\":\"e1\",\"schemaVersion\":\"1\" }",
                "1");
        assertTrue(reordered.body().contains("DUPLICATE"));
        assertTrue(request("POST", path + "/events", event("e2", "p1"), "1").body().contains("ALREADY_DELIVERED"));
        assertEquals(1, state(path).get("notificationCount").asInt());
        assertEquals(2, state(path).get("acceptedEventCount").asInt());
        var conflict = request("POST", path + "/events", event("e1", "unknown"), "1");
        assertEquals(409, conflict.statusCode());
        assertTrue(conflict.body().contains("EVENT_ID_CONFLICT"));
        assertEquals(404, request("POST", path + "/events", event("e3", "unknown"), "1").statusCode());
        assertEquals(200, request("POST", path + "/events", event("e3", "p2"), "1").statusCode());
    }

    @Test
    void distinctConcurrentEventsStillProduceOneNotification() throws Exception {
        String path = fresh("corrected");
        try (var pool = Executors.newFixedThreadPool(2)) {
            var work = List.<java.util.concurrent.Callable<Integer>>of(
                    () -> request("POST", path + "/events", event("a", "p1"), "1").statusCode(),
                    () -> request("POST", path + "/events", event("b", "p1"), "1").statusCode());
            for (var f : pool.invokeAll(work))
                assertEquals(200, f.get());
        }
        assertEquals(1, state(path).get("notificationCount").asInt());
        assertEquals(2, state(path).get("acceptedEventCount").asInt());
    }

    @Test
    void rollbackThenExplicitRedelivery() throws Exception {
        String path = fresh("corrected");
        FAIL.set(true);
        try {
            assertEquals(500, request("POST", path + "/events", event("e1", "p1"), "1").statusCode());
        }
        finally {
            FAIL.set(false);
        }
        assertEquals(0, state(path).get("acceptedEventCount").asInt());
        assertEquals("IN_TRANSIT", state(path).get("state").asText());
        assertEquals(200, request("POST", path + "/events", event("e1", "p1"), "1").statusCode());
        assertEquals(1, state(path).get("notificationCount").asInt());
    }

    @Test
    void resetLifecycleAndGenerationIsolation() throws Exception {
        String path = fresh("corrected");
        String other = fresh("corrected");
        request("POST", path + "/events", event("e1", "p1"), "1");
        assertEquals(409, request("POST", path + "/reset", null, "1").statusCode());
        assertEquals(200, request("POST", path + "/close", "{\"reason\":\"COMPLETED\"}", "1").statusCode());
        assertEquals(200, request("POST", path + "/reset", null, "1").statusCode());
        assertEquals(409, request("GET", path + "/state", null, "1").statusCode());
        assertEquals(200, request("POST", path + "/activate", null, "2").statusCode());
        assertEquals(409, request("POST", path + "/events", event("e1", "p1"), "1").statusCode());
        assertEquals(0, state(other).get("notificationCount").asInt());
    }

    @Test
    void malformedUnsupportedAndUnknownInputsHaveNoEffects() throws Exception {
        String path = fresh("corrected");
        String good = event("e1", "p1");
        for (String invalid : List.of("{}", good.replace("parcel.delivered", "parcel.created"),
                good.replace("\"1\"", "1"), good.replace("e1", "bad id"),
                good.replace("\"eventId\":\"e1\"", "\"eventId\":\"e1\",\"eventId\":\"e2\""), good + " {}"))
            assertTrue(request("POST", path + "/events", invalid, "1").statusCode() >= 400);
        assertEquals(404,
                request("POST", path.replaceAll("case-[a-f0-9]+", "missing") + "/events", good, "1").statusCode());
        assertEquals(0, state(path).get("notificationCount").asInt());
        assertEquals(0, state(path).get("acceptedEventCount").asInt());
    }

    @Test
    void payloadSizeHeaderAndHealthContracts() throws Exception {
        String path = fresh("corrected");
        String good = event("e1", "p1");
        assertEquals(200,
                request("POST", path + "/events", good + " ".repeat(16384 - good.length()), "1").statusCode());
        assertEquals(413,
                request("POST", path + "/events", good + " ".repeat(16385 - good.length()), "1").statusCode());
        assertEquals(400, request("GET", path + "/state", null, null).statusCode());
        assertEquals(200, request("GET", "/actuator/health/readiness", null, null).statusCode());
        assertEquals(200, request("GET", "/actuator/health/liveness", null, null).statusCode());
    }

    @Test
    void databaseConstraintsRejectCrossCaseAndDuplicateCorrectedNotification() throws Exception {
        String path = fresh("corrected");
        request("POST", path + "/events", event("e1", "p1"), "1");
        String id = path.substring(path.lastIndexOf('/') + 1);
        assertThrows(org.springframework.dao.DataIntegrityViolationException.class, () -> db.update(
                "INSERT INTO customer_notification(case_id,mode,parcel_id,source_event_id,accepted_event_id) VALUES (?,'CORRECTED','p1','e1','e1')",
                id));
        assertThrows(org.springframework.dao.DataIntegrityViolationException.class,
                () -> db.update("INSERT INTO parcel(case_id,mode,parcel_id) VALUES (?,'FAULTY','orphan')", id));
    }

}
