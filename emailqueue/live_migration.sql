-- Idempotent SQL to create the email_queue table on production.
-- Safe to run multiple times.

CREATE TABLE IF NOT EXISTS email_queue (
    id BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    email_type VARCHAR(50) NOT NULL COMMENT 'Mailable class identifier, e.g. forgot_password, verify_email',
    user_id BIGINT UNSIGNED NULL COMMENT 'Target user',
    group_id BIGINT UNSIGNED NULL COMMENT 'Related group (for welcome, modmail)',
    message_id BIGINT UNSIGNED NULL COMMENT 'Related message',
    chat_id BIGINT UNSIGNED NULL COMMENT 'Related chat room',
    extra_data JSON NULL COMMENT 'Additional data as JSON (email, subject, body, etc.)',
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    processed_at TIMESTAMP NULL COMMENT 'When successfully processed',
    failed_at TIMESTAMP NULL COMMENT 'When processing failed permanently',
    error_message TEXT NULL COMMENT 'Error details on failure',
    INDEX idx_pending (processed_at, created_at),
    INDEX idx_type (email_type)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
