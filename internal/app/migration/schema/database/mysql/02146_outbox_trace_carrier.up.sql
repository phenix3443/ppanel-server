-- The outbox row remembers the producing request's W3C trace context, so the
-- event's queue delivery joins the originating trace end to end (e.g. the
-- registration request contains the trial-grant span).
ALTER TABLE `domain_event_outbox` ADD COLUMN `trace_carrier` VARCHAR(1024) NOT NULL DEFAULT '' COMMENT 'W3C trace context of the producing request (JSON carrier)';
