package dev.webhookprobe.receiver;

import org.junit.jupiter.api.Test;
import static org.junit.jupiter.api.Assertions.assertThrows;
import static org.junit.jupiter.api.Assertions.assertEquals;
import org.springframework.mock.web.MockHttpServletRequest;
import java.nio.charset.StandardCharsets;
import java.util.Set;

class ReceiverApplicationTests {

    @Test
    void strictJsonRejectsDuplicatesAndTrailingContent() {
        for (String body : new String[] { "{\"eventId\":\"a\",\"eventId\":\"b\"}", "{} {}", "[]", "{\"extra\":1}",
                "\uFEFF{}" }) {
            var r = new MockHttpServletRequest();
            r.setContentType("application/json");
            r.setContent(body.getBytes(StandardCharsets.UTF_8));
            assertThrows(ApiProblem.class, () -> JsonInput.read(r, Set.of("eventId"), false));
        }
    }

    @Test
    void payloadBoundariesAndIdentifierRules() throws Exception {
        var r = new MockHttpServletRequest();
        r.setContentType("application/json");
        r.setContent(("{}" + " ".repeat(16382)).getBytes(StandardCharsets.UTF_8));
        assertEquals(0, JsonInput.read(r, Set.of(), false).size());
        r = new MockHttpServletRequest();
        r.setContentType("application/json");
        r.setContent(new byte[16385]);
        var tooLarge = r;
        assertEquals(413, assertThrows(ApiProblem.class, () -> JsonInput.read(tooLarge, Set.of(), false)).status);
        for (String id : new String[] { "", "a\n", "a b", "é", "a".repeat(65) })
            assertThrows(ApiProblem.class, () -> JsonInput.id(id));
    }

}
