package store

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/goopsagent/goopsagent/internal/models"
	goredis "github.com/redis/go-redis/v9"
)

// ---- Redis 会话缓存 & Pub/Sub ----

// GetSessionMessages 获取缓存的会话消息历史
func (s *Store) GetSessionMessages(ctx context.Context, sessionID string) ([]models.ChatMessage, error) {
	key := fmt.Sprintf("session:%s:messages", sessionID)
	data, err := s.redis.Get(ctx, key).Bytes()
	if err != nil {
		if err == goredis.Nil {
			return nil, nil
		}
		return nil, err
	}
	var msgs []models.ChatMessage
	if err := json.Unmarshal(data, &msgs); err != nil {
		return nil, err
	}
	return msgs, nil
}

// CacheSessionMessages 缓存会话上下文（30 分钟 TTL）
func (s *Store) CacheSessionMessages(ctx context.Context, sessionID string, messages []models.ChatMessage) error {
	key := fmt.Sprintf("session:%s:messages", sessionID)
	data, err := json.Marshal(messages)
	if err != nil {
		return err
	}
	return s.redis.Set(ctx, key, data, 30*time.Minute).Err()
}

// PublishEvent 向 Redis 频道发布 SSE 事件（用于跨实例广播）
func (s *Store) PublishEvent(ctx context.Context, channel string, event models.SSEEvent) error {
	data, err := json.Marshal(event)
	if err != nil {
		return err
	}
	return s.redis.Publish(ctx, channel, data).Err()
}

// Subscribe 订阅 Redis 频道，返回消息 channel
func (s *Store) Subscribe(ctx context.Context, channel string) (<-chan *goredis.Message, error) {
	pubsub := s.redis.Subscribe(ctx, channel)
	if _, err := pubsub.Receive(ctx); err != nil {
		return nil, err
	}
	return pubsub.Channel(), nil
}