-- Fix commission withdrawal/convert-balance logs that were stored with a positive amount.
-- They represent a reduction in commission and must be negative so that
-- SumAmountByTypeAndObjectID returns the correct net total.
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
  AND ((content::jsonb->>'amount')::bigint) > 0;
