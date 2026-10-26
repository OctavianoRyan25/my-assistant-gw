-- Migration: 008_create_budget_table.sql
CREATE TABLE IF NOT EXISTS `budget_settings` (
    `id`                        BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    `monthly_salary`            DECIMAL(15, 2)  NOT NULL DEFAULT 0,
    `warning_threshold_percent` DECIMAL(5, 2)   NOT NULL DEFAULT 80.00,
    `period`                    VARCHAR(7)    NOT NULL DEFAULT '2026-01',
    `created_at`                DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP,
    `updated_at`                DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
