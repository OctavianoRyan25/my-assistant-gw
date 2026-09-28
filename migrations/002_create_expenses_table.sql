-- Migration: 002_create_expenses_table.sql
CREATE TABLE IF NOT EXISTS `expenses` (
    `id`          BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    `amount`      DECIMAL(15, 2)  NOT NULL,
    `category`    VARCHAR(100)    NOT NULL DEFAULT 'lainnya',
    `description` TEXT            NOT NULL,
    `occurred_at` DATETIME        NOT NULL,
    `created_at`  DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (`id`),
    INDEX `idx_expenses_occurred_at` (`occurred_at`),
    INDEX `idx_expenses_category`    (`category`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
