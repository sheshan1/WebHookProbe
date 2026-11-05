package dev.webhookprobe.receiver;

import jakarta.servlet.FilterChain;
import jakarta.servlet.ServletException;
import jakarta.servlet.http.HttpServletRequest;
import jakarta.servlet.http.HttpServletResponse;
import java.io.IOException;
import java.util.ArrayList;
import java.util.Comparator;
import java.util.LinkedHashMap;
import java.util.Map;
import java.util.UUID;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;
import org.springframework.http.HttpStatus;
import org.springframework.stereotype.Component;
import org.springframework.web.filter.OncePerRequestFilter;

@Component
class RequestBoundary extends OncePerRequestFilter {

    private static final Logger LOG = LoggerFactory.getLogger(RequestBoundary.class);

    @Override
    protected void doFilterInternal(HttpServletRequest req, HttpServletResponse res, FilterChain chain)
            throws ServletException, IOException {
        long start = System.nanoTime();
        String supplied = req.getHeader("X-Request-Id");
        boolean valid = (supplied == null || supplied.matches("[A-Za-z0-9][A-Za-z0-9_-]{0,63}"))
                && java.util.Collections.list(req.getHeaders("X-Request-Id")).size() <= 1;
        String id = supplied != null && valid ? supplied : UUID.randomUUID().toString();
        req.setAttribute("requestId", id);
        res.setHeader("X-Request-Id", id);
        res.setHeader("Cache-Control", "no-store");
        try {
            if (!valid)
                throw new ApiProblem(400, "INVALID_REQUEST_ID", "Invalid request correlation identifier.");
            if (req.getRequestURI().startsWith("/v1/demo/")) {
                if (req.getQueryString() != null)
                    throw new ApiProblem(400, "INVALID_REQUEST", "Query parameters are not supported.");
                String encoding = req.getHeader("Content-Encoding");
                if (encoding != null && !encoding.equalsIgnoreCase("identity"))
                    throw new ApiProblem(415, "UNSUPPORTED_CONTENT_ENCODING", "Send an uncompressed body.");
                String accept = req.getHeader("Accept");
                if (accept != null && !accept.contains("*/*") && !accept.contains("application/json")
                        && !accept.contains("application/problem+json"))
                    throw new ApiProblem(406, "NOT_ACCEPTABLE", "Accept JSON responses.");
            }
            chain.doFilter(req, res);
        }
        catch (ApiProblem ex) {
            problem(req, res, ex);
        }
        finally {
            LOG.atInfo()
                .addKeyValue("requestId", id)
                .addKeyValue("method", req.getMethod())
                .addKeyValue("status", res.getStatus())
                .addKeyValue("durationMs", (System.nanoTime() - start) / 1_000_000)
                .addKeyValue("route", req.getAttribute("route"))
                .addKeyValue("caseId", req.getAttribute("caseId"))
                .addKeyValue("mode", req.getAttribute("mode"))
                .addKeyValue("generation", req.getAttribute("generation"))
                .addKeyValue("eventId", req.getAttribute("eventId"))
                .addKeyValue("closeReason", req.getAttribute("closeReason"))
                .addKeyValue("code", req.getAttribute("errorCode"))
                .log("request completed");
        }
    }

    static void problem(HttpServletRequest req, HttpServletResponse res, ApiProblem ex) throws IOException {
        req.setAttribute("errorCode", ex.code);
        res.setStatus(ex.status);
        res.setContentType("application/problem+json");
        if (req.getAttribute("allow") != null)
            res.setHeader("Allow", req.getAttribute("allow").toString());
        Map<String, Object> out = new LinkedHashMap<>();
        out.put("type", "about:blank");
        out.put("title", HttpStatus.valueOf(ex.status).getReasonPhrase());
        out.put("status", ex.status);
        out.put("detail", ex.getMessage());
        out.put("instance", req.getRequestURI());
        out.put("code", ex.code);
        out.put("requestId", req.getAttribute("requestId"));
        if (ex.status == 422 || !ex.errors.isEmpty()) {
            var errors = new ArrayList<>(ex.errors);
            errors.sort(
                    Comparator.comparing((Map<String, String> e) -> e.get("field")).thenComparing(e -> e.get("code")));
            out.put("errors", errors.stream().limit(20).toList());
            out.put("errorsTruncated", errors.size() > 20);
        }
        JsonInput.MAPPER.writeValue(res.getOutputStream(), out);
    }

}
