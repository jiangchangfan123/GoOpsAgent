-- 001: 创建 sessions 表 —— 对话会话
CREATE TABLE IF NOT EXISTS sessions (
    id          VARCHAR(36)  PRIMARY KEY,
    user_id     VARCHAR(64)  NOT NULL DEFAULT '',
    title       VARCHAR(255) NOT NULL DEFAULT '',
    status      VARCHAR(20)  NOT NULL DEFAULT 'active',   -- active / archived
    created_at  DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at  DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    INDEX idx_user_id (user_id),
    INDEX idx_status  (status)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;