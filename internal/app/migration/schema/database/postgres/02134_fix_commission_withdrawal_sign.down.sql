-- Revert: flip negative withdrawal/convert-balance amounts back to positive.
UPDATE system_logs
SET content = (
    jsonb_set(
        content::jsonb,
        '{amount}',
        to_jsonb(-((content::jsonb->>'amount')::bigint))
    )
)::text
WHERE type = 33
  AND ((content::jsonb->>'type')::integer) IN (334, 336)
  AND ((content::jsonb->>'amount')::bigint) < 0;
