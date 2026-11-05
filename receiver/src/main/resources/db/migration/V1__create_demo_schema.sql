CREATE DOMAIN probe_identifier AS varchar(64) COLLATE "C"
    CHECK (char_length(VALUE) BETWEEN 1 AND 64
           AND VALUE ~ '^[A-Za-z0-9]'
           AND VALUE !~ '[^A-Za-z0-9_-]');

CREATE TABLE demo_case (
    case_id       probe_identifier NOT NULL,
    mode          varchar(9) COLLATE "C" NOT NULL
                  CHECK (mode IN ('CORRECTED', 'FAULTY')),
    generation    bigint NOT NULL DEFAULT 1 CHECK (generation > 0),
    lifecycle     varchar(6) COLLATE "C" NOT NULL DEFAULT 'READY'
                  CHECK (lifecycle IN ('READY', 'ACTIVE', 'CLOSED')),
    created_at    timestamptz NOT NULL DEFAULT clock_timestamp(),
    activated_at  timestamptz,
    closed_at     timestamptz,
    last_reset_at timestamptz,
    PRIMARY KEY (case_id, mode),
    CHECK ((lifecycle = 'READY' AND activated_at IS NULL AND closed_at IS NULL)
        OR (lifecycle = 'ACTIVE' AND activated_at IS NOT NULL AND closed_at IS NULL)
        OR (lifecycle = 'CLOSED' AND activated_at IS NOT NULL AND closed_at IS NOT NULL))
);

CREATE TABLE parcel (
    case_id      probe_identifier NOT NULL,
    mode         varchar(9) COLLATE "C" NOT NULL,
    parcel_id    probe_identifier NOT NULL,
    state        varchar(10) COLLATE "C" NOT NULL DEFAULT 'IN_TRANSIT'
                 CHECK (state IN ('IN_TRANSIT', 'DELIVERED')),
    created_at   timestamptz NOT NULL DEFAULT clock_timestamp(),
    delivered_at timestamptz,
    PRIMARY KEY (case_id, mode, parcel_id),
    FOREIGN KEY (case_id, mode) REFERENCES demo_case (case_id, mode),
    CHECK ((state = 'IN_TRANSIT' AND delivered_at IS NULL)
        OR (state = 'DELIVERED' AND delivered_at IS NOT NULL))
);

CREATE TABLE processed_event (
    case_id        probe_identifier NOT NULL,
    mode           varchar(9) COLLATE "C" NOT NULL CHECK (mode = 'CORRECTED'),
    event_id       probe_identifier NOT NULL,
    parcel_id      probe_identifier NOT NULL,
    schema_version varchar(1) COLLATE "C" NOT NULL CHECK (schema_version = '1'),
    event_type     varchar(16) COLLATE "C" NOT NULL CHECK (event_type = 'parcel.delivered'),
    outcome        varchar(17) COLLATE "C" NOT NULL
                   CHECK (outcome IN ('PROCESSED', 'ALREADY_DELIVERED')),
    accepted_at    timestamptz NOT NULL DEFAULT clock_timestamp(),
    PRIMARY KEY (case_id, mode, event_id),
    UNIQUE (case_id, mode, event_id, parcel_id),
    FOREIGN KEY (case_id, mode, parcel_id)
        REFERENCES parcel (case_id, mode, parcel_id)
);

CREATE TABLE customer_notification (
    notification_id  bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    case_id           probe_identifier NOT NULL,
    mode              varchar(9) COLLATE "C" NOT NULL,
    parcel_id         probe_identifier NOT NULL,
    source_event_id   probe_identifier NOT NULL,
    accepted_event_id probe_identifier,
    created_at        timestamptz NOT NULL DEFAULT clock_timestamp(),
    FOREIGN KEY (case_id, mode, parcel_id)
        REFERENCES parcel (case_id, mode, parcel_id),
    FOREIGN KEY (case_id, mode, accepted_event_id, parcel_id)
        REFERENCES processed_event (case_id, mode, event_id, parcel_id),
    CHECK ((mode = 'CORRECTED' AND accepted_event_id IS NOT NULL
            AND accepted_event_id = source_event_id)
        OR (mode = 'FAULTY' AND accepted_event_id IS NULL))
);

