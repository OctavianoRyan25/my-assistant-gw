-- Migration: 003_create_chat_history_table.sql
CREATE TABLE IF NOT EXISTS `chat_history` (
    `id`         BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    `role`       VARCHAR(20)     NOT NULL,   -- user/assistant/system
    `message`    TEXT            NOT NULL,
    `created_at` DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (`id`),
    INDEX `idx_chat_history_created_at` (`created_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
