# Realtime Go Client

Go client library for [Supabase Realtime](https://supabase.com/docs/guides/realtime).

## About This Fork

Production fork of [mstgnz/realtime-go](https://github.com/mstgnz/realtime-go) (based on [supabase-community/realtime-go](https://github.com/supabase-community/realtime-go) v0.1.2) with fixes for long-running deployments.

**Main fixes:**
- Heartbeat survives reconnections (context-based lifecycle)
- Channel rejoin works after disconnect (idempotent subscription)
- No goroutine leaks (proper cancellation)
- No callback accumulation over reconnections (cleanup on unsubscribe)
- Graceful shutdown with resource cleanup
- Structured logging for production debugging

See [CHANGELOG.md](CHANGELOG.md) for complete details.

## Installation

```bash
go get github.com/techloopdev/realtime-go
```

## Usage

```go
package main

import (
    "context"
    "log"
    "time"

    "github.com/techloopdev/realtime-go/realtime"
)

func main() {
    client := realtime.NewRealtimeClient("your-project-ref", "your-anon-key")

    ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
    defer cancel()

    if err := client.Connect(ctx); err != nil {
        log.Fatal(err)
    }
    defer client.Disconnect()

    channel := client.Channel("my-channel", &realtime.ChannelConfig{})

    channel.OnBroadcast("event", func(payload json.RawMessage) {
        log.Printf("Received: %s", payload)
    })

    if err := channel.Subscribe(ctx, nil); err != nil {
        log.Fatal(err)
    }

    select {}
}
```

## Features

- **Broadcast**: Low-latency ephemeral messaging
- **Presence**: Shared state synchronization with CRDTs
- **Postgres CDC**: Database change notifications

## Structured Logging

Events are logged in parsable format:

```
[SUBSCRIBE_START] channel=my-channel ref=1
[SUBSCRIBE_ACK_OK] channel=my-channel ref=1 latency=127ms
[RECONNECT] Starting rejoin for 2 channels
[REJOIN_SUCCESS] channel=my-channel latency=154ms
```

Parse for monitoring:
```bash
grep '[SUBSCRIBE_TIMEOUT]' logs
grep 'latency=' logs | awk -F'latency=' '{print $2}'
```

## Testing

```bash
go test ./realtime/...              # Unit tests
go test -race ./realtime/...        # With race detector
go test -cover ./realtime/...       # With coverage
```

## Breaking Changes

**OnBroadcast semantics** (vs upstream):
- Upstream: Multiple handlers append
- This fork: Last handler wins

If you need multiple handlers:
```go
handlers := []func(json.RawMessage){h1, h2}
channel.OnBroadcast("event", func(payload json.RawMessage) {
    for _, h := range handlers { h(payload) }
})
```

## Examples

See [examples/](examples/) directory for working examples.

## License

MIT - See [LICENSE](LICENSE)
