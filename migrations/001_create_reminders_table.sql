-- Migration: 001_create_reminders_table.sql
CREATE TABLE IF NOT EXISTS `reminders` (
    `id`           BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    `title`        VARCHAR(500)    NOT NULL,
    `scheduled_at` DATETIME        NOT NULL,
    `recurrence`   VARCHAR(50)     NULL,           -- daily/weekly/monthly or cron expr
    `status`       VARCHAR(20)     NOT NULL DEFAULT 'active', -- active/done/cancelled
    `created_at`   DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP,
    `updated_at`   DATETIME        NULL     ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (`id`),
    INDEX `idx_reminders_status`       (`status`),
    INDEX `idx_reminders_scheduled_at` (`scheduled_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
