ALTER TABLE "user_device" ALTER COLUMN "user_agent" TYPE varchar(64) USING left("user_agent", 64);
