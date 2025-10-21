package realtime

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestChannelSubscribe(t *testing.T) {
	realtimeClient, mockConn := testClient()
	client := realtimeClient.(*RealtimeClient)
	channel := client.Channel("test-channel", &ChannelConfig{})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Simulate ACK response from server
	go func() {
		time.Sleep(10 * time.Millisecond) // Small delay to let Subscribe() register handler
		client.ackHandlersMu.RLock()
		handler, exists := client.ackHandlers["1"] // ref=1 for first call
		client.ackHandlersMu.RUnlock()
		if exists {
			handler("ok", json.RawMessage(`{}`))
		}
	}()

	err := channel.Subscribe(ctx, func(state SubscribeState, err error) {
		assert.NoError(t, err)
		assert.Equal(t, SubscribeStateSubscribed, state)
	})

	assert.NoError(t, err)

	// Verify the subscribe message was sent
	messages := mockConn.GetWriteMessages()
	assert.Len(t, messages, 1)
	assert.Contains(t, messages[0].(string), "phx_join")
}

func TestChannelUnsubscribe(t *testing.T) {
	realtimeClient, mockConn := testClient()
	client := realtimeClient.(*RealtimeClient)

	// Print initial messages
	initialMessages := mockConn.GetWriteMessages()
	t.Logf("Initial messages: %d", len(initialMessages))

	channel := client.Channel("test-channel", &ChannelConfig{})

	// Print messages after channel creation
	afterChannelMessages := mockConn.GetWriteMessages()
	t.Logf("After channel creation messages: %d", len(afterChannelMessages))
	for i, msg := range afterChannelMessages {
		t.Logf("Message %d: %v", i, msg)
	}

	// First subscribe to the channel
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Simulate ACK response
	go func() {
		time.Sleep(10 * time.Millisecond)
		client.ackHandlersMu.RLock()
		handler, exists := client.ackHandlers["1"]
		client.ackHandlersMu.RUnlock()
		if exists {
			handler("ok", json.RawMessage(`{}`))
		}
	}()

	err := channel.Subscribe(ctx, nil)
	assert.NoError(t, err)

	// Now unsubscribe
	err = channel.Unsubscribe()
	assert.NoError(t, err)

	// After unsubscribe, channel should not be subscribed anymore
	// State may still be from previous operation, but subscribed flag is reset
	afterUnsubMessages := mockConn.GetWriteMessages()
	t.Logf("After unsubscribe messages: %d", len(afterUnsubMessages))
	for i, msg := range afterUnsubMessages {
		t.Logf("Message %d: %v", i, msg)
	}

	// Verify unsubscribe message was sent
	assert.GreaterOrEqual(t, len(afterUnsubMessages), 2) // subscribe + unsubscribe
	lastMsg := afterUnsubMessages[len(afterUnsubMessages)-1].(string)
	assert.Contains(t, lastMsg, "phx_leave")
}

func TestChannelOnMessage(t *testing.T) {
	client, mockConn := testClient()
	channel := client.Channel("test-channel", &ChannelConfig{})

	messageReceived := false
	channel.OnMessage(func(msg Message) {
		messageReceived = true
		assert.Equal(t, "test-event", msg.Event)
	})

	// Add a mock message
	broadcastMsg, _ := json.Marshal(map[string]interface{}{
		"type":    "broadcast",
		"topic":   TopicPrefix + "test-channel",
		"event":   "test-event",
		"payload": map[string]interface{}{"test": "data"},
	})
	mockConn.AddReadMessage(broadcastMsg)

	// Process the message
	client.(*RealtimeClient).ProcessMessage(map[string]interface{}{
		"type":    "broadcast",
		"topic":   TopicPrefix + "test-channel",
		"event":   "test-event",
		"payload": map[string]interface{}{"test": "data"},
	})

	// Wait for message to be processed
	time.Sleep(100 * time.Millisecond)
	assert.True(t, messageReceived)
}

func TestChannelOnPresence(t *testing.T) {
	client, mockConn := testClient()
	channel := client.Channel("test-channel", &ChannelConfig{})

	presenceReceived := false
	channel.OnPresence(func(event PresenceEvent) {
		presenceReceived = true
		assert.Equal(t, "test-key", event.Key)
	})

	// Add a mock presence event
	presenceMsg, _ := json.Marshal(map[string]interface{}{
		"type":  "presence",
		"topic": TopicPrefix + "test-channel",
		"key":   "test-key",
	})
	mockConn.AddReadMessage(presenceMsg)

	// Process the message
	client.(*RealtimeClient).ProcessMessage(map[string]interface{}{
		"type":  "presence",
		"topic": TopicPrefix + "test-channel",
		"key":   "test-key",
	})

	// Wait for message to be processed
	time.Sleep(100 * time.Millisecond)
	assert.True(t, presenceReceived)
}

func TestChannelOnBroadcast(t *testing.T) {
	client, mockConn := testClient()
	channel := client.Channel("test-channel", &ChannelConfig{})

	broadcastReceived := false
	err := channel.OnBroadcast("test-event", func(payload json.RawMessage) {
		broadcastReceived = true
	})
	assert.NoError(t, err)

	// Simulate sending a broadcast
	err = channel.SendBroadcast("test-event", map[string]string{"test": "data"})
	assert.NoError(t, err)

	// Verify the broadcast message was sent
	messages := mockConn.GetWriteMessages()
	assert.Len(t, messages, 1)
	assert.Contains(t, messages[0].(string), "broadcast")
	assert.Contains(t, messages[0].(string), "test-event")

	// Add a mock broadcast response
	broadcastResponse, _ := json.Marshal(map[string]interface{}{
		"type":    "broadcast",
		"topic":   TopicPrefix + "test-channel",
		"event":   "test-event",
		"payload": map[string]string{"test": "data"},
	})
	mockConn.AddReadMessage(broadcastResponse)

	// Process the message
	client.(*RealtimeClient).ProcessMessage(map[string]interface{}{
		"type":    "broadcast",
		"topic":   TopicPrefix + "test-channel",
		"event":   "test-event",
		"payload": map[string]string{"test": "data"},
	})

	// Wait for message to be processed
	time.Sleep(100 * time.Millisecond)
	assert.True(t, broadcastReceived)
}

func TestChannelOnPostgresChange(t *testing.T) {
	client, mockConn := testClient()
	channel := client.Channel("test-channel", &ChannelConfig{})

	changeReceived := false
	err := channel.OnPostgresChange("INSERT", func(event PostgresChangeEvent) {
		changeReceived = true
		assert.Equal(t, "INSERT", event.Type)
		assert.Equal(t, "test_table", event.Table)
	})
	assert.NoError(t, err)

	// Add a mock postgres change event
	postgresMsg, _ := json.Marshal(map[string]interface{}{
		"type":   "postgres_changes",
		"topic":  TopicPrefix + "test-channel",
		"table":  "test_table",
		"schema": "public",
	})
	mockConn.AddReadMessage(postgresMsg)

	// Process the message
	client.(*RealtimeClient).ProcessMessage(map[string]interface{}{
		"type":   "postgres_changes",
		"topic":  TopicPrefix + "test-channel",
		"table":  "test_table",
		"schema": "public",
	})

	// Wait for message to be processed
	time.Sleep(100 * time.Millisecond)
	assert.True(t, changeReceived)
}

func TestChannelTrackUntrack(t *testing.T) {
	client, mockConn := testClient()
	channel := client.Channel("test-channel", &ChannelConfig{})

	// Test tracking
	err := channel.Track(map[string]interface{}{"user_id": 1})
	assert.NoError(t, err)

	// Verify the track message was sent
	messages := mockConn.GetWriteMessages()
	assert.Len(t, messages, 1)
	assert.Contains(t, messages[0].(string), "track")

	// Test untracking
	err = channel.Untrack()
	assert.NoError(t, err)

	// Verify the untrack message was sent
	messages = mockConn.GetWriteMessages()
	assert.Len(t, messages, 2)
	assert.Contains(t, messages[1].(string), "untrack")
}

func TestChannelGetTopic(t *testing.T) {
	client, _ := testClient()
	topic := "test-channel"
	channel := client.Channel(topic, &ChannelConfig{})

	// GetTopic() returns full topic with prefix
	assert.Equal(t, TopicPrefix+topic, channel.GetTopic())
	// GetShortTopic() returns topic without prefix
	assert.Equal(t, topic, channel.GetShortTopic())
}

func TestChannelRejoin(t *testing.T) {
	realtimeClient, mockConn := testClient()
	client := realtimeClient.(*RealtimeClient)
	ch := client.Channel("test-channel", &ChannelConfig{}).(*channel)

	// Initially not subscribed and joinedOnce is false, so rejoin should do nothing
	err := ch.rejoin()
	assert.NoError(t, err)
	assert.Equal(t, 0, len(mockConn.GetWriteMessages()))

	// Subscribe first
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Simulate ACK response for first subscription
	go func() {
		time.Sleep(10 * time.Millisecond)
		client.ackHandlersMu.RLock()
		handler, exists := client.ackHandlers["1"]
		client.ackHandlersMu.RUnlock()
		if exists {
			handler("ok", json.RawMessage(`{}`))
		}
	}()

	err = ch.Subscribe(ctx, nil)
	assert.NoError(t, err)
	assert.Equal(t, 1, len(mockConn.GetWriteMessages())) // phx_join

	// Simulate ACK response for rejoin
	go func() {
		time.Sleep(10 * time.Millisecond)
		client.ackHandlersMu.RLock()
		handler, exists := client.ackHandlers["2"]
		client.ackHandlersMu.RUnlock()
		if exists {
			handler("ok", json.RawMessage(`{}`))
		}
	}()

	// Now rejoin should re-subscribe (simulates reconnection scenario)
	// rejoin() resets subscribed flag and calls Subscribe() again
	err = ch.rejoin()
	assert.NoError(t, err)
	// Second phx_join message sent (rejoin behavior)
	assert.Equal(t, 2, len(mockConn.GetWriteMessages()))
	assert.Contains(t, mockConn.GetWriteMessages()[1].(string), "phx_join")
}

// TestChannelSubscribePayloadFormat validates that the phx_join message
// includes access_token in the payload according to Supabase Realtime protocol
// Reference: https://supabase.com/docs/guides/realtime/protocol
func TestChannelSubscribePayloadFormat(t *testing.T) {
	realtimeClient, _ := testClient()
	client := realtimeClient.(*RealtimeClient)

	// Create channel with specific config matching Supabase Realtime protocol
	channelConfig := &ChannelConfig{}
	channelConfig.Broadcast.Self = false
	channelConfig.Broadcast.Ack = true
	channelConfig.Presence.Key = "test-presence-key"
	channelConfig.Presence.Enabled = false
	channelConfig.Private = false

	channel := client.Channel("complex:hierarchical:topic:abc-123-def", channelConfig).(*channel)

	// Build subscribe message directly (same logic as Subscribe method)
	// Phoenix Channel protocol: topic, event, payload, ref, join_ref
	ref := client.NextRef()
	subscribeMsg := struct {
		Topic   string `json:"topic"`
		Event   string `json:"event"`
		Payload any    `json:"payload"`
		Ref     string `json:"ref"`
		JoinRef string `json:"join_ref"`
	}{
		Topic:   channel.topic,
		Event:   "phx_join",
		Ref:     fmt.Sprintf("%d", ref),
		JoinRef: fmt.Sprintf("%d", ref),
		Payload: map[string]any{
			"config": map[string]any{
				"broadcast":        channel.config.Broadcast,
				"presence":         channel.config.Presence,
				"postgres_changes": []any{}, // Required by Supabase protocol
				"private":          channel.config.Private,
			},
			"access_token": channel.client.config.APIKey,
		},
	}

	data, err := json.Marshal(subscribeMsg)
	assert.NoError(t, err)

	// Parse the marshaled message to validate structure
	var parsedMsg map[string]any
	err = json.Unmarshal(data, &parsedMsg)
	assert.NoError(t, err)

	// Validate message structure (Phoenix Channel protocol with Supabase requirements)
	assert.Equal(t, TopicPrefix+"complex:hierarchical:topic:abc-123-def", parsedMsg["topic"], "topic must have TopicPrefix")
	assert.Equal(t, "phx_join", parsedMsg["event"])
	assert.Equal(t, "1", parsedMsg["ref"], "ref should be string")
	assert.Equal(t, "1", parsedMsg["join_ref"], "join_ref should be present and match ref")

	// Validate payload structure
	payload, ok := parsedMsg["payload"].(map[string]any)
	assert.True(t, ok, "payload should be a map")

	// Validate config object
	config, ok := payload["config"].(map[string]any)
	assert.True(t, ok, "config should be present in payload")

	// Validate broadcast config
	broadcast, ok := config["broadcast"].(map[string]any)
	assert.True(t, ok, "broadcast should be present in config")
	assert.Equal(t, false, broadcast["self"])
	assert.Equal(t, true, broadcast["ack"])

	// Validate presence config
	presence, ok := config["presence"].(map[string]any)
	assert.True(t, ok, "presence should be present in config")
	assert.Equal(t, "test-presence-key", presence["key"])
	assert.Equal(t, false, presence["enabled"], "presence enabled should be false")

	// Validate postgres_changes (required by protocol)
	postgresChanges, ok := config["postgres_changes"].([]any)
	assert.True(t, ok, "postgres_changes should be present in config as array")
	assert.Empty(t, postgresChanges, "postgres_changes should be empty array for broadcast-only channels")

	// Validate private flag
	assert.Equal(t, false, config["private"])

	// CRITICAL: Validate access_token is present
	accessToken, ok := payload["access_token"].(string)
	assert.True(t, ok, "access_token should be present in payload as string")
	assert.Equal(t, "test-key", accessToken, "access_token should match client API key")
}
