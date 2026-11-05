package dev.webhookprobe.receiver;

import static org.junit.jupiter.api.Assertions.assertEquals;
import java.net.URI;
import java.net.http.HttpClient;
import java.net.http.HttpRequest;
import java.net.http.HttpResponse;
import org.junit.jupiter.api.Test;
import org.junit.jupiter.api.condition.EnabledIfEnvironmentVariable;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.boot.test.context.SpringBootTest;
import org.springframework.core.env.Environment;

@SpringBootTest(webEnvironment = SpringBootTest.WebEnvironment.RANDOM_PORT,
        properties = "webhookprobe.demo.enabled=false")
@EnabledIfEnvironmentVariable(named = "RUN_RECEIVER_IT", matches = "true")
class DisabledDemoIntegrationTests {

    @Autowired
    Environment env;

    @Test
    void demoAbsentWithoutEnablingProperty() throws Exception {
        var client = HttpClient.newHttpClient();
        String base = "http://127.0.0.1:" + env.getProperty("local.server.port");
        for (String mode : new String[] { "corrected", "faulty" }) {
            var request = HttpRequest.newBuilder(URI.create(base + "/v1/demo/" + mode + "/cases"))
                .header("Content-Type", "application/json")
                .POST(HttpRequest.BodyPublishers.ofString("{}"))
                .build();
            assertEquals(404, client.send(request, HttpResponse.BodyHandlers.discarding()).statusCode());
        }
        assertEquals(200,
                client
                    .send(HttpRequest.newBuilder(URI.create(base + "/actuator/health/liveness")).build(),
                            HttpResponse.BodyHandlers.discarding())
                    .statusCode());
        assertEquals(503,
                client
                    .send(HttpRequest.newBuilder(URI.create(base + "/actuator/health/readiness")).build(),
                            HttpResponse.BodyHandlers.discarding())
                    .statusCode());
    }

}
