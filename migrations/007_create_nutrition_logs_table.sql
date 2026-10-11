-- Migration: 007_create_nutrition_logs_table.sql
CREATE TABLE IF NOT EXISTS `nutrition_logs` (
    `id`                 BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    `food_name`          VARCHAR(255)    NOT NULL,
    `estimated_calories` INT             NOT NULL DEFAULT 0,
    `protein`            DECIMAL(6, 2)   NOT NULL DEFAULT 0,
    `carbs`              DECIMAL(6, 2)   NOT NULL DEFAULT 0,
    `fat`                DECIMAL(6, 2)   NOT NULL DEFAULT 0,
    `confidence`         DECIMAL(4, 2)   NOT NULL DEFAULT 0,
    `notes`              TEXT            NULL,
    `logged_at`          DATETIME        NOT NULL,
    `created_at`         DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (`id`),
    INDEX `idx_nutrition_logs_logged_at` (`logged_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
