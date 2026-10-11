-- Migration: 005_create_workouts_table.sql
CREATE TABLE IF NOT EXISTS `workouts` (
    `id`               BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    `title`            VARCHAR(255)    NOT NULL,
    `type`             VARCHAR(50)     NOT NULL DEFAULT 'gym',
    `duration_minutes` INT             NOT NULL DEFAULT 0,
    `calories_burned`  INT             NOT NULL DEFAULT 0,
    `distance_meters`  DECIMAL(10, 2)  NOT NULL DEFAULT 0,
    `description`      TEXT            NULL,
    `logged_at`        DATETIME        NOT NULL,
    `created_at`       DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP,
    `updated_at`       DATETIME        NULL ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (`id`),
    INDEX `idx_workouts_logged_at` (`logged_at`),
    INDEX `idx_workouts_type` (`type`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
