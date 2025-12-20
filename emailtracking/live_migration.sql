-- Email Tracking Tables Migration
-- Run these on the live database to create the email tracking infrastructure
-- Generated: 2025-12-20

-- Main email tracking table
CREATE TABLE IF NOT EXISTS `email_tracking` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `tracking_id` varchar(64) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL,
  `email_type` varchar(50) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL COMMENT 'Type of email: Digest, Chat, Alert, etc.',
  `userid` bigint unsigned DEFAULT NULL,
  `groupid` bigint unsigned DEFAULT NULL,
  `recipient_email` varchar(255) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL,
  `subject` varchar(500) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `metadata` json DEFAULT NULL COMMENT 'Additional context like message_id, etc.',
  `sent_at` timestamp NULL DEFAULT NULL,
  `delivered_at` timestamp NULL DEFAULT NULL,
  `bounced_at` timestamp NULL DEFAULT NULL,
  `bounce_type` varchar(50) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `opened_at` timestamp NULL DEFAULT NULL,
  `opened_via` varchar(50) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci DEFAULT NULL COMMENT 'pixel, image, mdn, click',
  `clicked_at` timestamp NULL DEFAULT NULL,
  `clicked_link` varchar(500) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `scroll_depth_percent` tinyint unsigned DEFAULT NULL,
  `images_loaded` smallint unsigned NOT NULL DEFAULT '0',
  `links_clicked` smallint unsigned NOT NULL DEFAULT '0',
  `unsubscribed_at` timestamp NULL DEFAULT NULL,
  `created_at` timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP,
  `updated_at` timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  UNIQUE KEY `tracking_id` (`tracking_id`),
  KEY `email_type` (`email_type`),
  KEY `userid` (`userid`),
  KEY `groupid` (`groupid`),
  KEY `sent_at` (`sent_at`),
  KEY `opened_at` (`opened_at`),
  KEY `created_at` (`created_at`),
  CONSTRAINT `email_tracking_ibfk_1` FOREIGN KEY (`userid`) REFERENCES `users` (`id`) ON DELETE SET NULL,
  CONSTRAINT `email_tracking_ibfk_2` FOREIGN KEY (`groupid`) REFERENCES `groups` (`id`) ON DELETE SET NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='Email delivery and engagement tracking';

-- Click tracking table (includes button actions like unsubscribe)
CREATE TABLE IF NOT EXISTS `email_tracking_clicks` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `email_tracking_id` bigint unsigned NOT NULL,
  `link_url` varchar(500) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL,
  `link_position` varchar(50) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci DEFAULT NULL COMMENT 'e.g., item_1, cta_button',
  `action` varchar(50) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci DEFAULT NULL COMMENT 'e.g., unsubscribe, cta, view_item',
  `ip_address` varchar(45) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `user_agent` varchar(500) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `clicked_at` timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  KEY `email_tracking_id` (`email_tracking_id`),
  KEY `action` (`action`),
  CONSTRAINT `email_tracking_clicks_ibfk_1` FOREIGN KEY (`email_tracking_id`) REFERENCES `email_tracking` (`id`) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='Individual click events for email tracking';

-- Image load tracking table (for scroll depth estimation)
CREATE TABLE IF NOT EXISTS `email_tracking_images` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `email_tracking_id` bigint unsigned NOT NULL,
  `image_position` varchar(50) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL COMMENT 'e.g., header, item_1, footer',
  `estimated_scroll_percent` tinyint unsigned DEFAULT NULL,
  `loaded_at` timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  KEY `email_tracking_id` (`email_tracking_id`),
  CONSTRAINT `email_tracking_images_ibfk_1` FOREIGN KEY (`email_tracking_id`) REFERENCES `email_tracking` (`id`) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='Image load events for scroll depth tracking';

-- Verify tables were created
SELECT 'email_tracking' as table_name, COUNT(*) as row_count FROM email_tracking
UNION ALL
SELECT 'email_tracking_clicks', COUNT(*) FROM email_tracking_clicks
UNION ALL
SELECT 'email_tracking_images', COUNT(*) FROM email_tracking_images;
