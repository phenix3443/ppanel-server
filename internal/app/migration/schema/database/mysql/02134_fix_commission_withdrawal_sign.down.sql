-- Revert: flip negative withdrawal/convert-balance amounts back to positive.
UPDATE system_logs
SET content = JSON_SET(
    content,
    '$.amount',
    -CAST(JSON_UNQUOTE(JSON_EXTRACT(content, '$.amount')) AS SIGNED)
)
WHERE type = 33
  AND CAST(JSON_UNQUOTE(JSON_EXTRACT(content, '$.type')) AS UNSIGNED) IN (334, 336)
  AND CAST(JSON_UNQUOTE(JSON_EXTRACT(content, '$.amount')) AS SIGNED) < 0;
