CREATE INDEX CONCURRENTLY idx_traffic_log_timestamp_brin
    ON traffic_log USING BRIN ("timestamp")
    WITH (pages_per_range = 128, autosummarize = on);
