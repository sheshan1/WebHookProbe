package dev.webhookprobe.receiver;

import java.sql.Timestamp;
import java.util.ArrayList;
import java.util.LinkedHashMap;
import java.util.List;
import java.util.Map;
import java.util.function.Supplier;
import org.springframework.jdbc.core.JdbcTemplate;
import org.springframework.stereotype.Service;
import org.springframework.transaction.PlatformTransactionManager;
import org.springframework.transaction.TransactionDefinition;
import org.springframework.transaction.support.TransactionTemplate;

@Service
public class DemoService {

    private final JdbcTemplate db;

    private final TransactionTemplate write;

    private final TransactionTemplate read;

    private final ProcessingFault fault;

    public DemoService(JdbcTemplate db, PlatformTransactionManager manager, ProcessingFault fault) {
        this.db = db;
        this.fault = fault;
        write = new TransactionTemplate(manager);
        write.setIsolationLevel(TransactionDefinition.ISOLATION_READ_COMMITTED);
        write.setTimeout(3);
        read = new TransactionTemplate(manager);
        read.setReadOnly(true);
        read.setIsolationLevel(TransactionDefinition.ISOLATION_REPEATABLE_READ);
        read.setTimeout(2);
    }

    private <T> T transaction(Supplier<T> action) {
        return write.execute(status -> {
            db.execute("SET LOCAL lock_timeout = '1s'");
            db.execute("SET LOCAL statement_timeout = '2s'");
            db.execute("SET LOCAL transaction_timeout = '3s'");
            return action.get();
        });
    }

    private Map<String, Object> caseRow(String mode, String id, Long generation, boolean lock) {
        var rows = db.queryForList("SELECT * FROM demo_case WHERE case_id=? AND mode=?" + (lock ? " FOR UPDATE" : ""),
                id, mode);
        if (rows.isEmpty())
            throw new ApiProblem(404, "CASE_NOT_FOUND", "Case does not exist.");
        var row = rows.getFirst();
        if (generation != null && ((Number) row.get("generation")).longValue() != generation)
            throw new ApiProblem(409, "GENERATION_MISMATCH",
                    "Discover the current case metadata and explicitly prepare a new run.");
        return row;
    }

    private Map<String, Object> descriptor(Map<String, Object> row) {
        Map<String, Object> out = new LinkedHashMap<>();
        out.put("caseId", row.get("case_id"));
        out.put("mode", row.get("mode").toString().toLowerCase(java.util.Locale.ROOT));
        out.put("generation", row.get("generation").toString());
        out.put("lifecycle", row.get("lifecycle"));
        for (String key : List.of("created_at", "activated_at", "closed_at", "last_reset_at")) {
            String name = switch (key) {
                case "created_at" -> "createdAt";
                case "activated_at" -> "activatedAt";
                case "closed_at" -> "closedAt";
                default -> "lastResetAt";
            };
            out.put(name, timestamp(row.get(key)));
        }
        return out;
    }

    private static String timestamp(Object value) {
        return value == null ? null : ((Timestamp) value).toInstant().toString();
    }

    private Map<String, Object> setupResult(String mode, String id) {
        return Map.of("case", descriptor(caseRow(mode, id, null, false)), "parcelIds", db.queryForList(
                "SELECT parcel_id FROM parcel WHERE case_id=? AND mode=? ORDER BY parcel_id", String.class, id, mode));
    }

    public Map<String, Object> create(String mode, String id, List<String> parcels) {
        return transaction(() -> {
            int n = db.update("INSERT INTO demo_case(case_id,mode) VALUES (?,?) ON CONFLICT DO NOTHING", id, mode);
            if (n == 0)
                throw new ApiProblem(409, "CASE_EXISTS", "Case already exists.");
            for (String parcel : parcels)
                db.update("INSERT INTO parcel(case_id,mode,parcel_id) VALUES (?,?,?)", id, mode, parcel);
            return setupResult(mode, id);
        });
    }

    public Map<String, Object> metadata(String mode, String id) {
        return read.execute(s -> descriptor(caseRow(mode, id, null, false)));
    }

    public Map<String, Object> lifecycle(String mode, String id, long generation, String action) {
        return transaction(() -> {
            var row = caseRow(mode, id, generation, true);
            String state = row.get("lifecycle").toString();
            switch (action) {
                case "activate" -> {
                    if (!state.equals("READY"))
                        throw new ApiProblem(409, "CASE_STATE_CONFLICT", "Only READY cases can activate.");
                    db.update(
                            "UPDATE demo_case SET lifecycle='ACTIVE',activated_at=clock_timestamp() WHERE case_id=? AND mode=?",
                            id, mode);
                }
                case "close" -> {
                    if (state.equals("READY"))
                        throw new ApiProblem(409, "CASE_STATE_CONFLICT", "READY cases cannot close.");
                    if (state.equals("ACTIVE"))
                        db.update(
                                "UPDATE demo_case SET lifecycle='CLOSED',closed_at=clock_timestamp() WHERE case_id=? AND mode=?",
                                id, mode);
                }
                case "reset" -> {
                    if (state.equals("ACTIVE"))
                        throw new ApiProblem(409, "CASE_ACTIVE",
                                "Stop delivery and verification, then explicitly close before reset.");
                    if (generation == Long.MAX_VALUE)
                        throw new ApiProblem(500, "GENERATION_EXHAUSTED", "Create a fresh case.");
                    db.update("DELETE FROM customer_notification WHERE case_id=? AND mode=?", id, mode);
                    db.update("DELETE FROM processed_event WHERE case_id=? AND mode=?", id, mode);
                    db.update("UPDATE parcel SET state='IN_TRANSIT',delivered_at=NULL WHERE case_id=? AND mode=?", id,
                            mode);
                    db.update(
                            "UPDATE demo_case SET generation=generation+1,lifecycle='READY',activated_at=NULL,closed_at=NULL,last_reset_at=clock_timestamp() WHERE case_id=? AND mode=?",
                            id, mode);
                    return setupResult(mode, id);
                }
                default -> throw new IllegalArgumentException("Unknown action");
            }
            return descriptor(caseRow(mode, id, generation, false));
        });
    }

    public Map<String, Object> deliver(String mode, String id, long generation, Map<String, String> event) {
        return transaction(() -> {
            var c = caseRow(mode, id, generation, true);
            if (!c.get("lifecycle").equals("ACTIVE"))
                throw new ApiProblem(409, "CASE_NOT_ACTIVE", "Activate a READY case before delivery.");
            String eventId = event.get("eventId"), parcelId = event.get("parcelId");
            String outcome = "PROCESSED";
            if (mode.equals("CORRECTED")) {
                var previous = db.queryForList(
                        "SELECT * FROM processed_event WHERE case_id=? AND mode=? AND event_id=?", id, mode, eventId);
                if (!previous.isEmpty()) {
                    var p = previous.getFirst();
                    if (!p.get("parcel_id").equals(parcelId)
                            || !p.get("schema_version").equals(event.get("schemaVersion"))
                            || !p.get("event_type").equals(event.get("type")))
                        throw new ApiProblem(409, "EVENT_ID_CONFLICT",
                                "This event ID was already accepted with different content.");
                    return deliveryResult(mode, id, generation, eventId, parcelId, "DUPLICATE");
                }
            }
            var parcels = db.queryForList("SELECT * FROM parcel WHERE case_id=? AND mode=? AND parcel_id=?", id, mode,
                    parcelId);
            if (parcels.isEmpty())
                throw new ApiProblem(404, "PARCEL_NOT_FOUND", "Parcel does not exist.");
            boolean already = parcels.getFirst().get("state").equals("DELIVERED");
            if (mode.equals("CORRECTED")) {
                outcome = already ? "ALREADY_DELIVERED" : "PROCESSED";
                db.update(
                        "INSERT INTO processed_event(case_id,mode,event_id,parcel_id,schema_version,event_type,outcome) VALUES (?,?,?,?,?,?,?)",
                        id, mode, eventId, parcelId, event.get("schemaVersion"), event.get("type"), outcome);
            }
            if (!already || mode.equals("FAULTY")) {
                db.update(
                        "UPDATE parcel SET state='DELIVERED',delivered_at=COALESCE(delivered_at,clock_timestamp()) WHERE case_id=? AND mode=? AND parcel_id=?",
                        id, mode, parcelId);
                fault.beforeNotification();
                db.update(
                        "INSERT INTO customer_notification(case_id,mode,parcel_id,source_event_id,accepted_event_id) VALUES (?,?,?,?,?)",
                        id, mode, parcelId, eventId, mode.equals("CORRECTED") ? eventId : null);
            }
            return deliveryResult(mode, id, generation, eventId, parcelId, outcome);
        });
    }

    private Map<String, Object> deliveryResult(String mode, String id, long generation, String event, String parcel,
            String outcome) {
        return Map.of("caseId", id, "mode", mode.toLowerCase(java.util.Locale.ROOT), "generation",
                Long.toString(generation), "eventId", event, "parcelId", parcel, "outcome", outcome);
    }

    private Map<String, Object> parcelState(String mode, String id, long generation, Map<String, Object> c,
            String parcel) {
        var rows = db.queryForList("SELECT * FROM parcel WHERE case_id=? AND mode=? AND parcel_id=?", id, mode, parcel);
        if (rows.isEmpty())
            throw new ApiProblem(404, "PARCEL_NOT_FOUND", "Parcel does not exist.");
        Map<String, Object> out = new LinkedHashMap<>();
        out.put("caseId", id);
        out.put("mode", mode.toLowerCase(java.util.Locale.ROOT));
        out.put("generation", Long.toString(generation));
        out.put("lifecycle", c.get("lifecycle"));
        out.put("parcelId", parcel);
        out.put("state", rows.getFirst().get("state"));
        out.put("notificationCount",
                db.queryForObject(
                        "SELECT count(*) FROM customer_notification WHERE case_id=? AND mode=? AND parcel_id=?",
                        Long.class, id, mode, parcel));
        out.put("acceptedEventCount",
                db.queryForObject("SELECT count(*) FROM processed_event WHERE case_id=? AND mode=? AND parcel_id=?",
                        Long.class, id, mode, parcel));
        out.put("deliveredAt", timestamp(rows.getFirst().get("delivered_at")));
        return out;
    }

    public Map<String, Object> state(String mode, String id, long generation, String parcel) {
        return read.execute(status -> {
            var c = caseRow(mode, id, generation, false);
            if (parcel != null)
                return parcelState(mode, id, generation, c, parcel);
            List<Map<String, Object>> parcels = new ArrayList<>();
            for (String p : db.queryForList(
                    "SELECT parcel_id FROM parcel WHERE case_id=? AND mode=? ORDER BY parcel_id", String.class, id,
                    mode))
                parcels.add(parcelState(mode, id, generation, c, p));
            return Map.of("case", descriptor(c), "parcels", parcels, "notificationCount",
                    parcels.stream().mapToLong(p -> ((Number) p.get("notificationCount")).longValue()).sum(),
                    "acceptedEventCount",
                    parcels.stream().mapToLong(p -> ((Number) p.get("acceptedEventCount")).longValue()).sum());
        });
    }

    public Map<String, Object> acceptedEvent(String mode, String id, long generation, String event) {
        return read.execute(status -> {
            caseRow(mode, id, generation, false);
            var rows = db.queryForList("SELECT * FROM processed_event WHERE case_id=? AND mode=? AND event_id=?", id,
                    mode, event);
            if (rows.isEmpty())
                throw new ApiProblem(404, "EVENT_NOT_FOUND", "Accepted event does not exist.");
            var p = rows.getFirst();
            return Map.of("caseId", id, "mode", mode.toLowerCase(java.util.Locale.ROOT), "generation",
                    Long.toString(generation), "payload",
                    Map.of("schemaVersion", p.get("schema_version"), "eventId", event, "type", p.get("event_type"),
                            "parcelId", p.get("parcel_id")),
                    "outcome", p.get("outcome"), "acceptedAt", timestamp(p.get("accepted_at")));
        });
    }

}
