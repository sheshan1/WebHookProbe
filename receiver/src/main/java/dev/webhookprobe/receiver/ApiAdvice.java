package dev.webhookprobe.receiver;

import jakarta.servlet.http.HttpServletRequest;
import jakarta.servlet.http.HttpServletResponse;
import java.io.IOException;
import org.slf4j.LoggerFactory;
import org.springframework.dao.DataAccessException;
import org.springframework.dao.DataIntegrityViolationException;
import org.springframework.dao.QueryTimeoutException;
import org.springframework.transaction.TransactionException;
import org.springframework.web.bind.annotation.ExceptionHandler;
import org.springframework.web.bind.annotation.RestControllerAdvice;
import org.springframework.web.servlet.resource.NoResourceFoundException;

@RestControllerAdvice
class ApiAdvice {

    @ExceptionHandler(Exception.class)
    void handle(Exception ex, HttpServletRequest req, HttpServletResponse res) throws IOException {
        ApiProblem problem;
        if (ex instanceof ApiProblem p)
            problem = p;
        else if (ex instanceof NoResourceFoundException)
            problem = new ApiProblem(404, "NOT_FOUND", "Unknown route.");
        else if (ex instanceof DataIntegrityViolationException)
            problem = new ApiProblem(500, "INTERNAL_ERROR", "Database invariant failed; inspect server logs.");
        else if (isDatabaseTimeout(ex) || ex instanceof QueryTimeoutException)
            problem = new ApiProblem(503, "PROCESSING_TIMEOUT",
                    "Database operation timed out; outcome may be unknown.");
        else if (ex instanceof DataAccessException || ex instanceof TransactionException)
            problem = new ApiProblem(503, "DATABASE_UNAVAILABLE", "Database operation failed; outcome may be unknown.");
        else
            problem = new ApiProblem(500, "INTERNAL_ERROR", "Unexpected processing failure.");
        if (problem.status >= 500)
            LoggerFactory.getLogger(ApiAdvice.class).error("Request {} failed", req.getAttribute("requestId"), ex);
        RequestBoundary.problem(req, res, problem);
    }

    private static boolean isDatabaseTimeout(Throwable error) {
        for (Throwable cause = error; cause != null; cause = cause.getCause()) {
            if (cause instanceof java.sql.SQLException sql && sql.getSQLState() != null
                    && java.util.Set.of("55P03", "57014", "25P04").contains(sql.getSQLState()))
                return true;
        }
        return false;
    }

}
