-- 002: 创建 messages 表 —— 对话消息
CREATE TABLE IF NOT EXISTS messages (
    id          VARCHAR(36)  PRIMARY KEY,
    session_id  VARCHAR(36)  NOT NULL,
    task_id     VARCHAR(36)  DEFAULT '',
    role        VARCHAR(20)  NOT NULL,                    -- user / assistant / system / tool
    content     TEXT         NOT NULL,
    created_at  DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
    INDEX idx_session_id (session_id),
    INDEX idx_task_id     (task_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;