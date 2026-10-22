-- 003: 创建 tasks 表 —— Agent 诊断任务
CREATE TABLE IF NOT EXISTS tasks (
    id          VARCHAR(36)  PRIMARY KEY,
    session_id  VARCHAR(36)  NOT NULL DEFAULT '',
    message     TEXT         NOT NULL,                    -- 用户原始提问
    status      VARCHAR(20)  NOT NULL DEFAULT 'created', -- created/running/succeeded/failed/cancelled
    answer      TEXT,                                    -- Agent 最终回复
    tool_runs   JSON,                                    -- 工具调用记录 [{tool_name, input, output, duration_ms, error}]
    error       TEXT,                                    -- 失败时的错误信息
    created_at  DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at  DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    INDEX idx_session_id (session_id),
    INDEX idx_status     (status)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;