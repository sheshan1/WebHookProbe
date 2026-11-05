package dev.webhookprobe.receiver;

import com.fasterxml.jackson.databind.JsonNode;
import jakarta.servlet.http.HttpServletRequest;
import java.io.IOException;
import java.util.ArrayList;
import java.util.List;
import java.util.Set;
import org.springframework.boot.autoconfigure.condition.ConditionalOnProperty;
import org.springframework.http.ResponseEntity;
import org.springframework.web.bind.annotation.RequestMapping;
import org.springframework.web.bind.annotation.RestController;

@RestController
@ConditionalOnProperty(name = "webhookprobe.demo.enabled", havingValue = "true")
class DemoController {

    private final DemoService service;

    DemoController(DemoService service) {
        this.service = service;
    }

    @RequestMapping("/v1/demo/**")
    ResponseEntity<Object> route(HttpServletRequest request) throws IOException {
        String[] path = request.getServletPath().substring("/v1/demo/".length()).split("/", -1);
        if (path.length < 2 || !Set.of("corrected", "faulty").contains(path[0]) || !path[1].equals("cases"))
            throw new ApiProblem(404, "NOT_FOUND", "Unknown route.");
        String mode = path[0].toUpperCase(java.util.Locale.ROOT);
        request.setAttribute("mode", path[0]);
        request.setAttribute("route", "/v1/demo/{mode}/cases");
        if (path.length == 2) {
            method(request, "POST");
            JsonNode input = JsonInput.read(request, Set.of("caseId", "parcelIds"), false);
            String id = JsonInput.strings(input, "caseId").get("caseId");
            JsonInput.id(id);
            var ids = input.get("parcelIds");
            if (ids == null || !ids.isArray() || ids.size() < 1 || ids.size() > 10)
                throw new ApiProblem(422, "PAYLOAD_VALIDATION_FAILED", "Provide 1–10 distinct parcel identifiers.");
            List<String> parcels = new ArrayList<>();
            for (JsonNode p : ids) {
                if (!p.isTextual())
                    throw new ApiProblem(422, "PAYLOAD_VALIDATION_FAILED", "Parcel IDs must be strings.");
                JsonInput.id(p.textValue());
                parcels.add(p.textValue());
            }
            if (parcels.stream().distinct().count() != parcels.size())
                throw new ApiProblem(422, "PAYLOAD_VALIDATION_FAILED", "Parcel IDs must be distinct.");
            return ResponseEntity.status(201)
                .header("Location", request.getRequestURI() + "/" + id)
                .body(service.create(mode, id, parcels));
        }
        String id = path[2];
        request.setAttribute("caseId", id);
        request.setAttribute("route", "/v1/demo/{mode}/cases/{caseId}/" + (path.length > 3 ? path[3] : ""));
        pathId(id);
        if (path.length == 3) {
            method(request, "GET");
            JsonInput.read(request, Set.of(), true);
            return ResponseEntity.ok(service.metadata(mode, id));
        }
        boolean known = (path.length == 4 && Set.of("activate", "reset", "close", "state", "events").contains(path[3]))
                || (path.length == 5 && path[3].equals("events"))
                || (path.length == 6 && path[3].equals("parcels") && path[5].equals("state"));
        if (!known)
            throw new ApiProblem(404, "NOT_FOUND", "Unknown route.");
        long generation = generation(request);
        request.setAttribute("generation", generation);
        if (path.length == 4) {
            String action = path[3];
            switch (action) {
                case "activate", "reset" -> {
                    method(request, "POST");
                    JsonInput.read(request, Set.of(), true);
                    return ResponseEntity.ok(service.lifecycle(mode, id, generation, action));
                }
                case "close" -> {
                    method(request, "POST");
                    var input = JsonInput.read(request, Set.of("reason"), false);
                    String reason = JsonInput.strings(input, "reason").get("reason");
                    if (!Set.of("COMPLETED", "ABANDONED").contains(reason))
                        throw new ApiProblem(422, "PAYLOAD_VALIDATION_FAILED", "Use COMPLETED or ABANDONED.");
                    request.setAttribute("closeReason", reason);
                    return ResponseEntity.ok(service.lifecycle(mode, id, generation, action));
                }
                case "state" -> {
                    method(request, "GET");
                    JsonInput.read(request, Set.of(), true);
                    return ResponseEntity.ok(service.state(mode, id, generation, null));
                }
                case "events" -> {
                    method(request, "POST");
                    var input = JsonInput.read(request, Set.of("schemaVersion", "eventId", "type", "parcelId"), false);
                    var event = JsonInput.strings(input, "schemaVersion", "eventId", "type", "parcelId");
                    JsonInput.id(event.get("eventId"));
                    JsonInput.id(event.get("parcelId"));
                    if (!event.get("schemaVersion").equals("1"))
                        throw new ApiProblem(422, "UNSUPPORTED_SCHEMA_VERSION", "Only schemaVersion 1 is supported.");
                    if (!event.get("type").equals("parcel.delivered"))
                        throw new ApiProblem(422, "UNSUPPORTED_EVENT_TYPE", "Only parcel.delivered is supported.");
                    request.setAttribute("eventId", event.get("eventId"));
                    return ResponseEntity.ok(service.deliver(mode, id, generation, event));
                }
                default -> throw new ApiProblem(404, "NOT_FOUND", "Unknown route.");
            }
        }
        if (path.length == 5 && path[3].equals("events")) {
            method(request, "GET");
            JsonInput.read(request, Set.of(), true);
            pathId(path[4]);
            return ResponseEntity.ok(service.acceptedEvent(mode, id, generation, path[4]));
        }
        if (path.length == 6 && path[3].equals("parcels") && path[5].equals("state")) {
            method(request, "GET");
            JsonInput.read(request, Set.of(), true);
            pathId(path[4]);
            return ResponseEntity.ok(service.state(mode, id, generation, path[4]));
        }
        throw new ApiProblem(404, "NOT_FOUND", "Unknown route.");
    }

    private static void method(HttpServletRequest request, String expected) {
        if (!request.getMethod().equals(expected)) {
            request.setAttribute("allow", expected);
            throw new ApiProblem(405, "METHOD_NOT_ALLOWED", "Unsupported HTTP method.");
        }
    }

    private static long generation(HttpServletRequest request) {
        var headers = request.getHeaders("X-Demo-Generation");
        String value = headers.hasMoreElements() ? headers.nextElement() : "";
        if (headers.hasMoreElements() || !value.matches("[1-9][0-9]{0,18}"))
            throw new ApiProblem(400, "INVALID_CONTEXT", "Supply one positive X-Demo-Generation header.");
        try {
            return Long.parseLong(value);
        }
        catch (NumberFormatException ex) {
            throw new ApiProblem(400, "INVALID_CONTEXT", "Generation exceeds bigint range.");
        }
    }

    private static void pathId(String value) {
        try {
            JsonInput.id(value);
        }
        catch (ApiProblem problem) {
            throw new ApiProblem(400, "INVALID_CONTEXT", "Invalid path identifier.");
        }
    }

}
