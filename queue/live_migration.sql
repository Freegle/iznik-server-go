-- Background tasks queue table
-- Go API inserts rows, iznik-batch processes them.
-- Idempotent - safe to run multiple times.

CREATE TABLE IF NOT EXISTS background_tasks (
    id BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    task_type VARCHAR(50) NOT NULL,
    data JSON NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    processed_at TIMESTAMP NULL,
    failed_at TIMESTAMP NULL,
    error_message TEXT NULL,
    attempts INT UNSIGNED DEFAULT 0,
    INDEX idx_task_type (task_type),
    INDEX idx_pending (processed_at, created_at)
);
