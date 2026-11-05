package dev.webhookprobe.receiver;

import java.util.List;
import java.util.Map;

final class ApiProblem extends RuntimeException {

    final int status;

    final String code;

    final List<Map<String, String>> errors;

    ApiProblem(int status, String code, String message) {
        this(status, code, message, List.of());
    }

    ApiProblem(int status, String code, String message, List<Map<String, String>> errors) {
        super(message);
        this.status = status;
        this.code = code;
        this.errors = errors;
    }

}
