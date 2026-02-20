-- V2 API Schema Changes - Run on production before deploying V2 handlers
-- All statements are idempotent (safe to run multiple times)

-- 1. Create socialactions table (new table for social media action tracking)
CREATE TABLE IF NOT EXISTS socialactions (
    id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    userid BIGINT UNSIGNED NOT NULL,
    groupid BIGINT UNSIGNED NOT NULL,
    msgid BIGINT UNSIGNED DEFAULT NULL,
    action_type VARCHAR(50) NOT NULL,
    uid VARCHAR(128) DEFAULT NULL,
    created TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    performed TIMESTAMP NULL DEFAULT NULL,
    PRIMARY KEY (id),
    KEY userid (userid),
    KEY groupid (groupid),
    KEY msgid (msgid),
    KEY action_type (action_type),
    KEY pending (groupid, performed)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- 2. Add rsvp column to chat_messages
SET @dbname = DATABASE();
SET @tablename = 'chat_messages';
SET @columnname = 'rsvp';
SET @preparedStatement = (SELECT IF(
  (SELECT COUNT(*) FROM INFORMATION_SCHEMA.COLUMNS
   WHERE TABLE_SCHEMA = @dbname AND TABLE_NAME = @tablename AND COLUMN_NAME = @columnname) > 0,
  'SELECT 1',
  "ALTER TABLE chat_messages ADD COLUMN rsvp ENUM('Yes','No','Maybe') DEFAULT NULL AFTER deleted"
));
PREPARE alterIfNotExists FROM @preparedStatement;
EXECUTE alterIfNotExists;
DEALLOCATE PREPARE alterIfNotExists;

-- 3. Add ReferToSupport to chat_messages type enum
-- Note: This ALTER is idempotent - if ReferToSupport already exists, it won't change
ALTER TABLE chat_messages MODIFY COLUMN type ENUM('Default','System','ModMail','Interested','Promised','Reneged','ReportedUser','Completed','Image','Address','Nudge','Schedule','ScheduleUpdated','Reminder','ReferToSupport') NOT NULL DEFAULT 'Default';

-- 4. Add shown column to alerts_tracking
SET @columnname = 'shown';
SET @tablename = 'alerts_tracking';
SET @preparedStatement = (SELECT IF(
  (SELECT COUNT(*) FROM INFORMATION_SCHEMA.COLUMNS
   WHERE TABLE_SCHEMA = @dbname AND TABLE_NAME = @tablename AND COLUMN_NAME = @columnname) > 0,
  'SELECT 1',
  'ALTER TABLE alerts_tracking ADD COLUMN shown INT UNSIGNED NOT NULL DEFAULT 0 AFTER response'
));
PREPARE alterIfNotExists FROM @preparedStatement;
EXECUTE alterIfNotExists;
DEALLOCATE PREPARE alterIfNotExists;

-- 5. Add clicked column to alerts_tracking
SET @columnname = 'clicked';
SET @preparedStatement = (SELECT IF(
  (SELECT COUNT(*) FROM INFORMATION_SCHEMA.COLUMNS
   WHERE TABLE_SCHEMA = @dbname AND TABLE_NAME = @tablename AND COLUMN_NAME = @columnname) > 0,
  'SELECT 1',
  'ALTER TABLE alerts_tracking ADD COLUMN clicked INT UNSIGNED NOT NULL DEFAULT 0 AFTER shown'
));
PREPARE alterIfNotExists FROM @preparedStatement;
EXECUTE alterIfNotExists;
DEALLOCATE PREPARE alterIfNotExists;

-- 6. Add partnerconsent column to messages
SET @tablename = 'messages';
SET @columnname = 'partnerconsent';
SET @preparedStatement = (SELECT IF(
  (SELECT COUNT(*) FROM INFORMATION_SCHEMA.COLUMNS
   WHERE TABLE_SCHEMA = @dbname AND TABLE_NAME = @tablename AND COLUMN_NAME = @columnname) > 0,
  'SELECT 1',
  'ALTER TABLE messages ADD COLUMN partnerconsent TINYINT(1) NOT NULL DEFAULT 0 AFTER deadline'
));
PREPARE alterIfNotExists FROM @preparedStatement;
EXECUTE alterIfNotExists;
DEALLOCATE PREPARE alterIfNotExists;

-- 7. Add deletedby column to messages
SET @columnname = 'deletedby';
SET @preparedStatement = (SELECT IF(
  (SELECT COUNT(*) FROM INFORMATION_SCHEMA.COLUMNS
   WHERE TABLE_SCHEMA = @dbname AND TABLE_NAME = @tablename AND COLUMN_NAME = @columnname) > 0,
  'SELECT 1',
  'ALTER TABLE messages ADD COLUMN deletedby BIGINT UNSIGNED DEFAULT NULL AFTER deleted'
));
PREPARE alterIfNotExists FROM @preparedStatement;
EXECUTE alterIfNotExists;
DEALLOCATE PREPARE alterIfNotExists;
