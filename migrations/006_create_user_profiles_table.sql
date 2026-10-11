-- Migration: 006_create_user_profiles_table.sql
CREATE TABLE IF NOT EXISTS `user_profiles` (
    `id`                   BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    `height_cm`            DECIMAL(6, 2)   NOT NULL DEFAULT 0,
    `weight_kg`            DECIMAL(6, 2)   NOT NULL DEFAULT 0,
    `age`                  INT             NOT NULL DEFAULT 0,
    `gender`               VARCHAR(20)     NOT NULL DEFAULT 'male',
    `activity_level`       VARCHAR(50)     NOT NULL DEFAULT 'moderately_active',
    `daily_calorie_target` INT             NOT NULL DEFAULT 2000,
    `monthly_salary`       DECIMAL(15, 2)  NOT NULL DEFAULT 0,
    `created_at`           DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP,
    `updated_at`           DATETIME        NULL ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
