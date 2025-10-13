package realtime

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"nhooyr.io/websocket"
)

type channel struct {
	topic             string
	state             ChannelState
	config            *ChannelConfig
	client            *RealtimeClient
	broadcastHandlers map[string]func(json.RawMessage)
	postgresHandlers  map[string]func(PostgresChangeEvent)
	callbacks         map[string][]interface{}
	mu                sync.RWMutex
	joinedOnce        bool
	subscribed        bool
	subscribeMu       sync.Mutex
}

func newChannel(topic string, config *ChannelConfig, client *RealtimeClient) *channel {
	return &channel{
		topic:             topic,
		state:             ChannelStateClosed,
		config:            config,
		client:            client,
		broadcastHandlers: make(map[string]func(json.RawMessage)),
		postgresHandlers:  make(map[string]func(PostgresChangeEvent)),
		callbacks:         make(map[string][]interface{}),
	}
}

func (ch *channel) Subscribe(ctx context.Context, callback func(SubscribeState, error)) error {
	ch.subscribeMu.Lock()
	defer ch.subscribeMu.Unlock()

	// Idempotent: if already subscribed, succeed immediately
	if ch.subscribed {
		ch.client.logger.Printf("Channel %s already subscribed (idempotent)", ch.topic)
		if callback != nil {
			callback(SubscribeStateSubscribed, nil)
		}
		return nil
	}

	ch.mu.Lock()
	ch.state = ChannelStateJoining
	ch.mu.Unlock()

	subscribeMsg := struct {
		Type    string      `json:"type"`
		Topic   string      `json:"topic"`
		Event   string      `json:"event"`
		Payload interface{} `json:"payload"`
	}{
		Type:    "subscribe",
		Topic:   ch.topic,
		Event:   "phx_join",
		Payload: ch.config,
	}

	data, err := json.Marshal(subscribeMsg)
	if err != nil {
		return err
	}

	if err := ch.client.conn.Write(ctx, websocket.MessageText, data); err != nil {
		ch.mu.Lock()
		ch.state = ChannelStateErrored
		ch.mu.Unlock()
		if callback != nil {
			callback(SubscribeStateChannelError, err)
		}
		return err
	}

	ch.joinedOnce = true
	ch.subscribed = true // Track current subscription state

	ch.mu.Lock()
	ch.state = ChannelStateJoined
	ch.mu.Unlock()

	if callback != nil {
		callback(SubscribeStateSubscribed, nil)
	}

	return nil
}

func (ch *channel) Unsubscribe() error {
	ch.subscribeMu.Lock()
	defer ch.subscribeMu.Unlock()

	// Idempotent: if not subscribed, succeed immediately
	if !ch.subscribed {
		return nil
	}

	ch.mu.Lock()
	ch.state = ChannelStateLeaving
	ch.mu.Unlock()

	unsubscribeMsg := struct {
		Type  string `json:"type"`
		Topic string `json:"topic"`
		Event string `json:"event"`
	}{
		Type:  "unsubscribe",
		Topic: ch.topic,
		Event: "phx_leave",
	}

	data, err := json.Marshal(unsubscribeMsg)
	if err != nil {
		return err
	}

	if err := ch.client.conn.Write(context.Background(), websocket.MessageText, data); err != nil {
		return err
	}

	// Reset subscription state
	ch.subscribed = false

	// Clear all callbacks to prevent accumulation (CRIT-05)
	ch.mu.Lock()
	ch.callbacks = make(map[string][]interface{})
	ch.mu.Unlock()

	return nil
}

func (ch *channel) OnMessage(callback func(Message)) {
	ch.mu.Lock()
	defer ch.mu.Unlock()
	ch.callbacks["message"] = append(ch.callbacks["message"], callback)
}

func (ch *channel) OnPresence(callback func(PresenceEvent)) {
	ch.mu.Lock()
	defer ch.mu.Unlock()
	ch.callbacks["presence"] = append(ch.callbacks["presence"], callback)
}

func (ch *channel) OnBroadcast(event string, callback func(json.RawMessage)) error {
	ch.mu.Lock()
	defer ch.mu.Unlock()

	key := fmt.Sprintf("broadcast:%s", event)

	// Single-handler semantics (last-wins)
	// Replace existing handler instead of appending
	ch.callbacks[key] = []interface{}{callback}

	return nil
}

func (ch *channel) SendBroadcast(event string, payload interface{}) error {
	broadcastMsg := struct {
		Type    string      `json:"type"`
		Topic   string      `json:"topic"`
		Event   string      `json:"event"`
		Payload interface{} `json:"payload"`
		Ref     int         `json:"ref"`
	}{
		Type:    "broadcast",
		Topic:   ch.topic,
		Event:   event,
		Payload: payload,
		Ref:     ch.client.NextRef(),
	}

	data, err := json.Marshal(broadcastMsg)
	if err != nil {
		return err
	}
	return ch.client.conn.Write(context.Background(), websocket.MessageText, data)
}

func (ch *channel) OnPostgresChange(event string, callback func(PostgresChangeEvent)) error {
	ch.mu.Lock()
	defer ch.mu.Unlock()
	ch.callbacks[fmt.Sprintf("postgres_changes:%s", event)] = append(ch.callbacks[fmt.Sprintf("postgres_changes:%s", event)], callback)
	return nil
}

func (ch *channel) Track(payload interface{}) error {
	trackMsg := struct {
		Type    string      `json:"type"`
		Topic   string      `json:"topic"`
		Event   string      `json:"event"`
		Payload interface{} `json:"payload"`
	}{
		Type:    "track",
		Topic:   ch.topic,
		Event:   "track",
		Payload: payload,
	}

	data, err := json.Marshal(trackMsg)
	if err != nil {
		return err
	}
	return ch.client.conn.Write(context.Background(), websocket.MessageText, data)
}

func (ch *channel) Untrack() error {
	untrackMsg := struct {
		Type  string `json:"type"`
		Topic string `json:"topic"`
		Event string `json:"event"`
	}{
		Type:  "untrack",
		Topic: ch.topic,
		Event: "untrack",
	}

	data, err := json.Marshal(untrackMsg)
	if err != nil {
		return err
	}
	return ch.client.conn.Write(context.Background(), websocket.MessageText, data)
}

func (ch *channel) GetState() ChannelState {
	ch.mu.RLock()
	defer ch.mu.RUnlock()
	return ch.state
}

func (ch *channel) rejoin() error {
	ch.subscribeMu.Lock()
	wasSubscribed := ch.subscribed
	ch.subscribed = false // Reset to allow re-subscription
	ch.subscribeMu.Unlock()

	// Only rejoin if was previously subscribed
	if !wasSubscribed && !ch.joinedOnce {
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), ch.client.config.Timeout)
	defer cancel()

	return ch.Subscribe(ctx, func(state SubscribeState, err error) {
		if err != nil {
			ch.client.logger.Printf("Rejoin failed for %s: %v", ch.topic, err)
		}
	})
}

// GetTopic returns the channel's topic
func (c *channel) GetTopic() string {
	return c.topic
}
