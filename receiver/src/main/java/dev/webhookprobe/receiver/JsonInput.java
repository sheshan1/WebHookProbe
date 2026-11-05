package dev.webhookprobe.receiver;

import com.fasterxml.jackson.core.JsonParser;
import com.fasterxml.jackson.databind.DeserializationFeature;
import com.fasterxml.jackson.databind.JsonNode;
import com.fasterxml.jackson.databind.ObjectMapper;
import jakarta.servlet.http.HttpServletRequest;
import java.io.IOException;
import java.nio.ByteBuffer;
import java.nio.charset.CharacterCodingException;
import java.nio.charset.CodingErrorAction;
import java.nio.charset.StandardCharsets;
import java.util.ArrayList;
import java.util.List;
import java.util.Map;
import java.util.Set;
import org.springframework.http.MediaType;

final class JsonInput {

    static final ObjectMapper MAPPER = new ObjectMapper().enable(JsonParser.Feature.STRICT_DUPLICATE_DETECTION)
        .enable(DeserializationFeature.FAIL_ON_TRAILING_TOKENS);

    private JsonInput() {
    }

    static void id(String value) {
        if (value == null || !value.matches("[A-Za-z0-9][A-Za-z0-9_-]{0,63}")) {
            throw new ApiProblem(422, "PAYLOAD_VALIDATION_FAILED",
                    "Identifiers must be 1–64 ASCII letters, digits, underscores or hyphens, starting alphanumeric.");
        }
    }

    static JsonNode read(HttpServletRequest request, Set<String> fields, boolean empty) throws IOException {
        byte[] bytes = request.getInputStream().readNBytes(16385);
        if (bytes.length > 16384)
            throw new ApiProblem(413, "PAYLOAD_TOO_LARGE", "Maximum body size is 16384 bytes.");
        if (empty) {
            if (bytes.length != 0 || request.getContentType() != null)
                throw new ApiProblem(400, "INVALID_REQUEST",
                        "This operation requires an empty body without Content-Type.");
            return MAPPER.createObjectNode();
        }
        try {
            MediaType media = MediaType
                .parseMediaType(request.getContentType() == null ? "" : request.getContentType());
            if (!media.getType().equals("application") || !media.getSubtype().equals("json")
                    || (media.getCharset() != null && !media.getCharset().equals(StandardCharsets.UTF_8)))
                throw new IllegalArgumentException();
        }
        catch (IllegalArgumentException ex) {
            throw new ApiProblem(415, "UNSUPPORTED_MEDIA_TYPE", "Use application/json with UTF-8.");
        }
        String text;
        try {
            text = StandardCharsets.UTF_8.newDecoder()
                .onMalformedInput(CodingErrorAction.REPORT)
                .onUnmappableCharacter(CodingErrorAction.REPORT)
                .decode(ByteBuffer.wrap(bytes))
                .toString();
        }
        catch (CharacterCodingException ex) {
            throw new ApiProblem(400, "INVALID_JSON", "The body must be valid UTF-8 JSON.");
        }
        try {
            if (text.startsWith("\uFEFF"))
                throw new IOException("BOM");
            JsonNode node = MAPPER.readTree(text);
            if (node == null || !node.isObject())
                throw new IOException("object required");
            var names = node.fieldNames();
            while (names.hasNext())
                if (!fields.contains(names.next()))
                    throw new ApiProblem(400, "INVALID_REQUEST", "Unknown JSON field.");
            return node;
        }
        catch (IOException ex) {
            throw new ApiProblem(400, "INVALID_JSON",
                    "Expected one JSON object without duplicate keys or trailing content.");
        }
    }

    static Map<String, String> strings(JsonNode node, String... fields) {
        List<Map<String, String>> errors = new ArrayList<>();
        Map<String, String> values = new java.util.LinkedHashMap<>();
        for (String field : fields) {
            JsonNode v = node.get(field);
            if (v == null || !v.isTextual() || v.textValue().isBlank()) {
                errors.add(Map.of("field", "/" + field, "code", "REQUIRED_STRING", "message",
                        "A nonempty string is required."));
            }
            else
                values.put(field, v.textValue());
        }
        if (!errors.isEmpty())
            throw new ApiProblem(422, "PAYLOAD_VALIDATION_FAILED", "Invalid fields.", errors);
        return values;
    }

}
