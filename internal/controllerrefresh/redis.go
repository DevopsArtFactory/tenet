package controllerrefresh

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"github.com/redis/go-redis/v9"
)

const (
	defaultRedisKeyPrefix = "tenet:status"
	BackendRedis          = "redis"
)

type Request struct {
	RequestID   string `json:"requestId"`
	DeliveryID  string `json:"deliveryId"`
	Repository  string `json:"repository"`
	Ref         string `json:"ref"`
	After       string `json:"after,omitempty"`
	RequestedAt string `json:"requestedAt"`
}

type Receipt struct {
	Backend   string
	Channel   string
	RequestID string
}

type Message struct {
	Request Request
	Err     error
}

type Publisher interface {
	Publish(context.Context, Request) (Receipt, error)
}

type Subscriber interface {
	Subscribe(context.Context) (Subscription, error)
}

type Subscription interface {
	C() <-chan Message
	Channel() string
	Close() error
}

type RedisHandoff struct {
	client    redis.UniversalClient
	keyPrefix string
}

func NewRedisHandoff(client redis.UniversalClient, keyPrefix string) RedisHandoff {
	if keyPrefix == "" {
		keyPrefix = defaultRedisKeyPrefix
	}
	return RedisHandoff{client: client, keyPrefix: keyPrefix}
}

func (h RedisHandoff) Publish(ctx context.Context, request Request) (Receipt, error) {
	if err := request.validate(); err != nil {
		return Receipt{}, err
	}
	payload, err := json.Marshal(request)
	if err != nil {
		return Receipt{}, fmt.Errorf("encode controller refresh request: %w", err)
	}
	channel := h.channel()
	if err := h.client.Publish(ctx, channel, payload).Err(); err != nil {
		return Receipt{}, fmt.Errorf("publish controller refresh request: %w", err)
	}
	return Receipt{
		Backend:   BackendRedis,
		Channel:   channel,
		RequestID: request.RequestID,
	}, nil
}

func (h RedisHandoff) Subscribe(ctx context.Context) (Subscription, error) {
	pubsub := h.client.Subscribe(ctx, h.channel())
	if _, err := pubsub.Receive(ctx); err != nil {
		_ = pubsub.Close()
		return nil, fmt.Errorf("subscribe controller refresh handoff: %w", err)
	}

	subscription := &redisSubscription{
		channel:  h.channel(),
		pubsub:   pubsub,
		messages: make(chan Message, 1),
	}
	go subscription.run(ctx)
	return subscription, nil
}

func (h RedisHandoff) channel() string {
	return h.keyPrefix + ":controller-refresh"
}

func (r Request) validate() error {
	switch {
	case strings.TrimSpace(r.RequestID) == "":
		return fmt.Errorf("controller refresh requestId is required")
	case strings.TrimSpace(r.DeliveryID) == "":
		return fmt.Errorf("controller refresh deliveryId is required")
	case strings.TrimSpace(r.Repository) == "":
		return fmt.Errorf("controller refresh repository is required")
	case strings.TrimSpace(r.Ref) == "":
		return fmt.Errorf("controller refresh ref is required")
	case strings.TrimSpace(r.RequestedAt) == "":
		return fmt.Errorf("controller refresh requestedAt is required")
	default:
		return nil
	}
}

type redisSubscription struct {
	channel   string
	pubsub    *redis.PubSub
	messages  chan Message
	closeOnce sync.Once
}

func (s *redisSubscription) C() <-chan Message {
	return s.messages
}

func (s *redisSubscription) Channel() string {
	return s.channel
}

func (s *redisSubscription) Close() error {
	var err error
	s.closeOnce.Do(func() {
		err = s.pubsub.Close()
	})
	return err
}

func (s *redisSubscription) run(ctx context.Context) {
	defer close(s.messages)
	redisMessages := s.pubsub.Channel()

	for {
		select {
		case <-ctx.Done():
			return
		case redisMessage, ok := <-redisMessages:
			if !ok {
				return
			}
			message := Message{}
			if err := json.Unmarshal([]byte(redisMessage.Payload), &message.Request); err != nil {
				message.Err = fmt.Errorf("decode controller refresh request: %w", err)
			} else if err := message.Request.validate(); err != nil {
				message.Err = err
			}

			select {
			case <-ctx.Done():
				return
			case s.messages <- message:
			}
		}
	}
}
