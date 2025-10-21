package realtime

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

// ChannelState represents the current state of a channel
type ChannelState int

const (
	ChannelStateClosed ChannelState = iota
	ChannelStateJoining
	ChannelStateLeaving
	ChannelStateErrored
	ChannelStateJoined
)

// TopicPrefix is the required prefix for all Supabase Realtime channel topics
// as per Phoenix Channel protocol specification
const TopicPrefix = "realtime:"

// SubscribeState represents the subscription state
type SubscribeState int

const (
	SubscribeStateSubscribed SubscribeState = iota
	SubscribeStateChannelError
	SubscribeStateTimedOut
	SubscribeStateClosed
)

// ChannelConfig represents the configuration for a channel
type ChannelConfig struct {
	Broadcast struct {
		Ack  bool `json:"ack"`
		Self bool `json:"self"`
	} `json:"broadcast"`
	Presence struct {
		Key     string `json:"key"`
		Enabled bool   `json:"enabled"`
	} `json:"presence"`
	Private bool `json:"private"`
}

// IRealtimeClient represents the main client interface for Supabase Realtime
type IRealtimeClient interface {
	// Connect establishes a connection to the Supabase Realtime server
	Connect(ctx context.Context) error

	// Disconnect closes the connection to the Supabase Realtime server
	Disconnect() error

	// Channel creates a new channel for realtime subscriptions
	Channel(topic string, config *ChannelConfig) Channel

	// SetAuth sets the authentication token for the client
	SetAuth(token string) error

	// GetChannels returns all active channels
	GetChannels() map[string]Channel

	// RemoveChannel removes a channel from the client
	RemoveChannel(channel Channel) error

	// RemoveAllChannels removes all channels from the client
	RemoveAllChannels() error

	// ProcessMessage processes a single message from the WebSocket connection (for testing)
	ProcessMessage(msg any)
}

// Channel represents a realtime channel for subscriptions
type Channel interface {
	// Subscribe subscribes to the channel
	Subscribe(ctx context.Context, callback func(SubscribeState, error)) error

	// Unsubscribe unsubscribes from the channel
	Unsubscribe() error

	// OnMessage registers a callback for receiving messages
	OnMessage(callback func(Message))

	// OnPresence registers a callback for presence events
	OnPresence(callback func(PresenceEvent))

	// Track registers the current client's presence
	Track(payload any) error

	// Untrack removes the current client's presence
	Untrack() error

	// OnBroadcast registers a callback for broadcast events
	OnBroadcast(event string, callback func(json.RawMessage)) error

	// SendBroadcast sends a broadcast message
	SendBroadcast(event string, payload any) error

	// OnPostgresChange registers a callback for Postgres CDC events
	OnPostgresChange(event string, callback func(PostgresChangeEvent)) error

	// GetState returns the current state of the channel
	GetState() ChannelState

	// GetTopic returns the full topic with TopicPrefix
	GetTopic() string

	// GetShortTopic returns the topic without TopicPrefix
	GetShortTopic() string
}

// FlexibleRef is a type that can unmarshal from both string and number
type FlexibleRef string

// UnmarshalJSON implements custom unmarshaling to handle both string and number refs
func (f *FlexibleRef) UnmarshalJSON(data []byte) error {
	// Try to unmarshal as string first
	var s string
	if err := json.Unmarshal(data, &s); err == nil {
		*f = FlexibleRef(s)
		return nil
	}

	// Try to unmarshal as number
	var n json.Number
	if err := json.Unmarshal(data, &n); err == nil {
		*f = FlexibleRef(n.String())
		return nil
	}

	// Both failed - return error
	return fmt.Errorf("ref must be string or number, got: %s", string(data))
}

// String returns the string representation
func (f FlexibleRef) String() string {
	return string(f)
}

// Message represents a realtime message
type Message struct {
	Type    string          `json:"type"`
	Topic   string          `json:"topic"`
	Event   string          `json:"event"`
	Ref     FlexibleRef     `json:"ref,omitempty"` // Accepts both string and number from Phoenix
	Payload json.RawMessage `json:"payload"`
}

// PresenceEvent represents a presence event
type PresenceEvent struct {
	Type            string                 `json:"type"`
	Key             string                 `json:"key"`
	NewPresence     map[string]any `json:"new_presence,omitempty"`
	CurrentPresence map[string]any `json:"current_presence,omitempty"`
}

// PostgresChangeEvent represents a Postgres CDC event
type PostgresChangeEvent struct {
	Type    string          `json:"type"`
	Table   string          `json:"table"`
	Schema  string          `json:"schema"`
	Payload json.RawMessage `json:"payload"`
}

// Config represents the configuration for the RealtimeClient
type Config struct {
	URL            string
	APIKey         string
	AuthToken      string
	AutoReconnect  bool
	HBInterval     time.Duration
	MaxRetries     int
	InitialBackoff time.Duration
	Timeout        time.Duration
}

// NewConfig creates a new Config with default values
func NewConfig() *Config {
	return &Config{
		URL:            "wss://realtime.supabase.com",
		AutoReconnect:  true,
		HBInterval:     30 * time.Second,
		MaxRetries:     5,
		InitialBackoff: time.Second,
		Timeout:        30 * time.Second,
	}
}
