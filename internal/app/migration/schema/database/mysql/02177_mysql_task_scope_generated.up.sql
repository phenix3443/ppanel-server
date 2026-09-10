ALTER TABLE `task`
    ADD COLUMN `scope_type` TINYINT
        GENERATED ALWAYS AS (
            CAST(JSON_UNQUOTE(JSON_EXTRACT(IF(JSON_VALID(`scope`), `scope`, '{}'), '$.Type')) AS SIGNED)
        ) VIRTUAL,
    ALGORITHM=INSTANT;
