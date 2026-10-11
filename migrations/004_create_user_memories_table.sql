-- Migration: 004_create_user_memories_table.sql
CREATE TABLE IF NOT EXISTS `user_memories` (
    `id`         BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    `category`   VARCHAR(50)     NOT NULL DEFAULT 'personal', -- personality, preference, habit, work, personal
    `fact`       TEXT            NOT NULL,
    `created_at` DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP,
    `updated_at` DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (`id`),
    INDEX `idx_user_memories_category` (`category`),
    INDEX `idx_user_memories_created_at` (`created_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
