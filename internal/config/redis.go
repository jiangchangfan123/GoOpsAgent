package config

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/goopsagent/goopsagent/internal/models"
	goredis "github.com/redis/go-redis/v9"
)

type Client struct {
	rdb *goredis.Client
}

func NewClient(addr, password string) (*Client, error) {
	rdb := goredis.NewClient(&goredis.Options{
		Addr:     addr,
		Password: password,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	if err := rdb.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("redis ping: %w", err)
	}
	slog.Info("redis connected", "addr", addr)
	return &Client{rdb: rdb}, nil
}

// 获取缓存的会话消息
func (c *Client) GetSession(ctx context.Context, sessionID string) ([]models.ChatMessage, error) {
	key := fmt.Sprintf("session:%s:messages", sessionID)
	data, err := c.rdb.Get(ctx, key).Bytes()
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

// 缓存会话上下文
func (c *Client) CacheSession(ctx context.Context, sessionID string, messages []models.ChatMessage) error {
	key := fmt.Sprintf("session:%s:messages", sessionID)
	data, err := json.Marshal(messages)
	if err != nil {
		return err
	}
	return c.rdb.Set(ctx, key, data, 30*time.Minute).Err()
}

// PublishEvent 向频道发布 SSE 事件（用于跨实例广播）
func (c *Client) PublishEvent(ctx context.Context, channel string, event models.SSEEvent) error {
	data, err := json.Marshal(event)
	if err != nil {
		return err
	}
	return c.rdb.Publish(ctx, channel, data).Err()
}

// 订阅频道，返回消息channel
func (c *Client) Subscribe(ctx context.Context, channel string) (<-chan *goredis.Message, error) {
	//向redis发送订阅请求
	pubsub := c.rdb.Subscribe(ctx, channel)
	//同步等待redis服务端成功处理订阅请求
	if _, err := pubsub.Receive(ctx); err != nil {
		return nil, err
	}
	return pubsub.Channel(), nil
}

// Close 关闭连接
func (c *Client) Close() error {
	return c.rdb.Close()
}
