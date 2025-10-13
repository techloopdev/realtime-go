package realtime

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	"nhooyr.io/websocket"
)

// Conn represents a WebSocket connection interface
type Conn interface {
	Close(code websocket.StatusCode, reason string) error
	Read(ctx context.Context) (websocket.MessageType, []byte, error)
	Write(ctx context.Context, messageType websocket.MessageType, data []byte) error
	Ping(ctx context.Context) error
	SetReadLimit(limit int64)
	SetWriteLimit(limit int64)
}

// RealtimeClient represents the main client for Supabase Realtime
type RealtimeClient struct {
	config         *Config
	conn           Conn
	channels       map[string]*channel
	mu             sync.RWMutex
	authToken      string
	ref            int
	refMu          sync.Mutex
	connCtx        context.Context
	connCancel     context.CancelFunc
	reconnMu       sync.Mutex
	isReconnecting bool
	logger         *log.Logger
	ackHandlers    map[int]func(string, json.RawMessage)
	ackHandlersMu  sync.RWMutex
}

// NewRealtimeClient creates a new RealtimeClient instance
func NewRealtimeClient(projectRef string, apiKey string) IRealtimeClient {
	config := NewConfig()
	config.URL = fmt.Sprintf(
		"wss://%s.supabase.co/realtime/v1/websocket?apikey=%s&log_level=info&vsn=1.0.0",
		projectRef,
		apiKey,
	)
	config.APIKey = apiKey

	return &RealtimeClient{
		config:      config,
		channels:    make(map[string]*channel),
		logger:      log.Default(),
		ackHandlers: make(map[int]func(string, json.RawMessage)),
	}
}

// Connect establishes a connection to the Supabase Realtime server
func (c *RealtimeClient) Connect(ctx context.Context) error {
	c.reconnMu.Lock()
	if c.isReconnecting {
		c.reconnMu.Unlock()
		return fmt.Errorf("client is already reconnecting")
	}
	c.reconnMu.Unlock()

	opts := &websocket.DialOptions{
		HTTPHeader: make(map[string][]string),
	}

	if c.config.APIKey != "" {
		opts.HTTPHeader["apikey"] = []string{c.config.APIKey}
	}
	if c.authToken != "" {
		opts.HTTPHeader["Authorization"] = []string{fmt.Sprintf("Bearer %s", c.authToken)}
	}

	conn, _, err := websocket.Dial(ctx, c.config.URL, opts)
	if err != nil {
		return fmt.Errorf("failed to connect: %w", err)
	}

	// Create connection-scoped context
	c.connCtx, c.connCancel = context.WithCancel(context.Background())

	// Wrap the websocket.Conn in our Conn interface
	c.conn = &websocketConnWrapper{conn}
	go c.handleMessages(c.connCtx)
	go c.startHeartbeat(c.connCtx)

	return nil
}

// websocketConnWrapper wraps *websocket.Conn to implement our Conn interface
type websocketConnWrapper struct {
	*websocket.Conn
}

func (w *websocketConnWrapper) SetWriteLimit(limit int64) {
	// No-op as websocket.Conn doesn't have this method
}

// Disconnect closes the connection to the Supabase Realtime server
func (c *RealtimeClient) Disconnect() error {
	// Cancel connection context to stop goroutines gracefully
	if c.connCancel != nil {
		c.connCancel()
	}

	if c.conn != nil {
		return c.conn.Close(websocket.StatusNormalClosure, "Closing the connection")
	}
	return nil
}

// Channel creates a new channel for realtime subscriptions
func (c *RealtimeClient) Channel(topic string, config *ChannelConfig) Channel {
	c.mu.Lock()
	defer c.mu.Unlock()

	if ch, exists := c.channels[topic]; exists {
		return ch
	}

	ch := newChannel(topic, config, c)
	c.channels[topic] = ch
	return ch
}

// SetAuth sets the authentication token for the client
func (c *RealtimeClient) SetAuth(token string) error {
	c.authToken = token
	return nil
}

// GetChannels returns all active channels
func (c *RealtimeClient) GetChannels() map[string]Channel {
	c.mu.RLock()
	defer c.mu.RUnlock()
	channels := make(map[string]Channel)
	for topic, ch := range c.channels {
		channels[topic] = ch
	}
	return channels
}

// RemoveChannel removes a channel from the client
func (c *RealtimeClient) RemoveChannel(ch Channel) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if ch, ok := ch.(*channel); ok {
		if _, exists := c.channels[ch.topic]; exists {
			delete(c.channels, ch.topic)
			return ch.Unsubscribe()
		}
	}
	return fmt.Errorf("channel not found")
}

// RemoveAllChannels removes all channels from the client
func (c *RealtimeClient) RemoveAllChannels() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	for _, ch := range c.channels {
		if err := ch.Unsubscribe(); err != nil {
			c.logger.Printf("Error unsubscribing from channel %s: %v", ch.topic, err)
		}
	}
	c.channels = make(map[string]*channel)
	return nil
}

func (c *RealtimeClient) handleMessages(ctx context.Context) {
	defer func() {
		c.logger.Printf("handleMessages goroutine terminated")
	}()

	for {
		select {
		case <-ctx.Done():
			return
		default:
			_, data, err := c.conn.Read(ctx)
			if err != nil {
				if ctx.Err() != nil {
					return
				}
				c.logger.Printf("WebSocket read error: %v", err)
				if c.config.AutoReconnect {
					go c.reconnect()
				}
				return
			}

			var msg Message
			if err := json.Unmarshal(data, &msg); err != nil {
				c.logger.Printf("Error unmarshaling message: %v", err)
				continue
			}

			// Handle phx_reply events (ACK responses)
			if msg.Event == "phx_reply" {
				c.ackHandlersMu.RLock()
				handler, exists := c.ackHandlers[msg.Ref]
				c.ackHandlersMu.RUnlock()

				if exists {
					var replyPayload struct {
						Status   string          `json:"status"`
						Response json.RawMessage `json:"response"`
					}
					if err := json.Unmarshal(msg.Payload, &replyPayload); err != nil {
						c.logger.Printf("Error unmarshaling phx_reply payload: %v", err)
					} else {
						handler(replyPayload.Status, replyPayload.Response)
					}
				}
				continue
			}

			// Handle different message types
			switch msg.Type {
			case "broadcast":
				c.handleBroadcast(msg)
			case "presence":
				c.handlePresence(msg)
			case "postgres_changes":
				c.handlePostgresChanges(msg)
			}
		}
	}
}

func (c *RealtimeClient) startHeartbeat(ctx context.Context) {
	defer func() {
		c.logger.Printf("Heartbeat goroutine terminated")
	}()

	ticker := time.NewTicker(c.config.HBInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := c.SendHeartbeat(); err != nil {
				c.logger.Printf("Error sending heartbeat: %v", err)
				if c.config.AutoReconnect {
					go c.reconnect()
				}
				return
			}
		}
	}
}

// SendHeartbeat sends a heartbeat message to the server
func (c *RealtimeClient) SendHeartbeat() error {
	heartbeatMsg := struct {
		Type  string `json:"type"`
		Topic string `json:"topic"`
		Event string `json:"event"`
		Ref   int    `json:"ref"`
	}{
		Type:  "heartbeat",
		Topic: "phoenix",
		Event: "heartbeat",
		Ref:   c.NextRef(),
	}

	data, err := json.Marshal(heartbeatMsg)
	if err != nil {
		return err
	}
	return c.conn.Write(context.Background(), websocket.MessageText, data)
}

func (c *RealtimeClient) reconnect() {
	c.reconnMu.Lock()
	if c.isReconnecting {
		c.reconnMu.Unlock()
		return
	}
	c.isReconnecting = true
	c.reconnMu.Unlock()

	defer func() {
		c.reconnMu.Lock()
		c.isReconnecting = false
		c.reconnMu.Unlock()
	}()

	backoff := c.config.InitialBackoff
	for i := 0; i < c.config.MaxRetries; i++ {
		ctx, cancel := context.WithTimeout(context.Background(), c.config.Timeout)
		err := c.Connect(ctx)
		cancel()

		if err == nil {
			// Sequential rejoin with error handling
			c.mu.RLock()
			channels := make([]*channel, 0, len(c.channels))
			for _, ch := range c.channels {
				channels = append(channels, ch)
			}
			c.mu.RUnlock()

			successCount := 0
			failureCount := 0
			startTime := time.Now()

			c.logger.Printf("[RECONNECT] Starting rejoin for %d channels", len(channels))

			for _, ch := range channels {
				rejoinStart := time.Now()
				if err := ch.rejoin(); err != nil {
					c.logger.Printf("[REJOIN_FAIL] channel=%s error=%v latency=%dms",
						ch.topic, err, time.Since(rejoinStart).Milliseconds())
					failureCount++

					// Retry once
					time.Sleep(500 * time.Millisecond)
					retryStart := time.Now()
					if retryErr := ch.rejoin(); retryErr != nil {
						c.logger.Printf("[REJOIN_RETRY_FAIL] channel=%s error=%v latency=%dms",
							ch.topic, retryErr, time.Since(retryStart).Milliseconds())
					} else {
						c.logger.Printf("[REJOIN_RETRY_SUCCESS] channel=%s latency=%dms",
							ch.topic, time.Since(retryStart).Milliseconds())
						successCount++
					}
				} else {
					c.logger.Printf("[REJOIN_SUCCESS] channel=%s latency=%dms",
						ch.topic, time.Since(rejoinStart).Milliseconds())
					successCount++
				}
			}

			c.logger.Printf("[RECONNECT_COMPLETE] success=%d failure=%d total_latency=%dms",
				successCount, failureCount, time.Since(startTime).Milliseconds())
			return
		}

		c.logger.Printf("Reconnection attempt %d failed: %v", i+1, err)
		if i < c.config.MaxRetries-1 {
			time.Sleep(backoff)
			backoff *= 2
			if backoff > 60*time.Second {
				backoff = 60 * time.Second
			}
		}
	}

	c.logger.Printf("Failed to reconnect after %d attempts", c.config.MaxRetries)
}

// NextRef returns the next reference number for messages
func (c *RealtimeClient) NextRef() int {
	c.refMu.Lock()
	defer c.refMu.Unlock()
	c.ref++
	return c.ref
}

// registerAckHandler registers a handler for ACK responses with the given ref
func (c *RealtimeClient) registerAckHandler(ref int, handler func(string, json.RawMessage)) {
	c.ackHandlersMu.Lock()
	defer c.ackHandlersMu.Unlock()
	c.ackHandlers[ref] = handler
}

// unregisterAckHandler removes the ACK handler for the given ref
func (c *RealtimeClient) unregisterAckHandler(ref int) {
	c.ackHandlersMu.Lock()
	defer c.ackHandlersMu.Unlock()
	delete(c.ackHandlers, ref)
}

func (c *RealtimeClient) handleBroadcast(msg Message) {
	c.mu.RLock()
	ch, exists := c.channels[msg.Topic]
	c.mu.RUnlock()

	if !exists {
		return
	}

	ch.mu.RLock()
	callbacks, exists := ch.callbacks["broadcast:"+msg.Event]
	ch.mu.RUnlock()

	if !exists {
		return
	}

	for _, callback := range callbacks {
		if cb, ok := callback.(func(json.RawMessage)); ok {
			cb(msg.Payload)
		}
	}
}

func (c *RealtimeClient) handlePresence(msg Message) {
	c.mu.RLock()
	ch, exists := c.channels[msg.Topic]
	c.mu.RUnlock()

	if !exists {
		return
	}

	var presenceEvent PresenceEvent
	if err := json.Unmarshal(msg.Payload, &presenceEvent); err != nil {
		c.logger.Printf("Error unmarshaling presence event: %v", err)
		return
	}

	ch.mu.RLock()
	callbacks, exists := ch.callbacks["presence"]
	ch.mu.RUnlock()

	if !exists {
		return
	}

	for _, callback := range callbacks {
		if cb, ok := callback.(func(PresenceEvent)); ok {
			cb(presenceEvent)
		}
	}
}

func (c *RealtimeClient) handlePostgresChanges(msg Message) {
	c.mu.RLock()
	ch, exists := c.channels[msg.Topic]
	c.mu.RUnlock()

	if !exists {
		return
	}

	var changeEvent PostgresChangeEvent
	if err := json.Unmarshal(msg.Payload, &changeEvent); err != nil {
		c.logger.Printf("Error unmarshaling postgres change event: %v", err)
		return
	}

	ch.mu.RLock()
	callbacks, exists := ch.callbacks["postgres_changes:"+changeEvent.Type]
	ch.mu.RUnlock()

	if !exists {
		return
	}

	for _, callback := range callbacks {
		if cb, ok := callback.(func(PostgresChangeEvent)); ok {
			cb(changeEvent)
		}
	}
}

// SetConn sets the WebSocket connection for testing purposes
func (c *RealtimeClient) SetConn(conn Conn) {
	c.conn = conn
}

// ProcessMessage processes a single message from the WebSocket connection
func (c *RealtimeClient) ProcessMessage(msg interface{}) {
	// Convert the message to a map
	msgMap, ok := msg.(map[string]interface{})
	if !ok {
		return
	}

	// Get the message type
	msgType, ok := msgMap["type"].(string)
	if !ok {
		return
	}

	// Get the topic
	topic, ok := msgMap["topic"].(string)
	if !ok {
		return
	}

	// Get the channel
	channel, ok := c.channels[topic]
	if !ok {
		return
	}

	// Process the message based on type
	switch msgType {
	case "broadcast":
		event, ok := msgMap["event"].(string)
		if !ok {
			return
		}
		payload, err := json.Marshal(msgMap["payload"])
		if err != nil {
			return
		}

		// Handle both broadcast and message callbacks
		channel.mu.RLock()
		callbacks, exists := channel.callbacks["broadcast:"+event]
		if exists {
			for _, callback := range callbacks {
				if cb, ok := callback.(func(json.RawMessage)); ok {
					cb(payload)
				}
			}
		}

		// Also handle as a general message
		messageCallbacks, exists := channel.callbacks["message"]
		if exists {
			message := Message{
				Type:    msgType,
				Topic:   topic,
				Event:   event,
				Payload: payload,
			}
			for _, callback := range messageCallbacks {
				if cb, ok := callback.(func(Message)); ok {
					cb(message)
				}
			}
		}
		channel.mu.RUnlock()

	case "presence":
		key, ok := msgMap["key"].(string)
		if !ok {
			return
		}
		channel.mu.RLock()
		callbacks, exists := channel.callbacks["presence"]
		channel.mu.RUnlock()
		if exists {
			for _, callback := range callbacks {
				if cb, ok := callback.(func(PresenceEvent)); ok {
					cb(PresenceEvent{Key: key})
				}
			}
		}
	case "postgres_changes":
		table, ok := msgMap["table"].(string)
		if !ok {
			return
		}
		schema, ok := msgMap["schema"].(string)
		if !ok {
			return
		}
		channel.mu.RLock()
		callbacks, exists := channel.callbacks["postgres_changes:INSERT"]
		channel.mu.RUnlock()
		if exists {
			for _, callback := range callbacks {
				if cb, ok := callback.(func(PostgresChangeEvent)); ok {
					cb(PostgresChangeEvent{
						Type:   "INSERT",
						Table:  table,
						Schema: schema,
					})
				}
			}
		}
	}
}
