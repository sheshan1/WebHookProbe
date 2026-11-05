CREATE UNIQUE INDEX uq_corrected_parcel_notification
    ON customer_notification (case_id, mode, parcel_id)
    WHERE mode = 'CORRECTED';

CREATE INDEX ix_notification_parcel
    ON customer_notification (case_id, mode, parcel_id);

CREATE INDEX ix_event_parcel
    ON processed_event (case_id, mode, parcel_id);
