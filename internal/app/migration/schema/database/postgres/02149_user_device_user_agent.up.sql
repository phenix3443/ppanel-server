-- Modern browser User-Agent strings routinely exceed the original
-- varchar(64), which made every device bind fail with "value too long".
ALTER TABLE "user_device" ALTER COLUMN "user_agent" TYPE varchar(512);
