# PIANO DI REFACTORING - realtime-go v0.1.7-dam

**Data Creazione**: 2025-10-20
**Contesto**: Consolidamento fix Phoenix Channel Protocol per production readiness
**Repository**: `https://github.com/techloopdev/realtime-go` (fork di `supabase-community/realtime-go`)

---

## 📊 STATO ATTUALE

### ✅ FIX COMPLETATI (da committare)

#### **FIX 1: Test Failures - ackHandlers Type Mismatch** ✅
**File**: `realtime/channel_test.go`
**Modifiche applicate**:
- Linea 25: `client.ackHandlers[1]` → `client.ackHandlers["1"]`
- Linea 70: `client.ackHandlers[1]` → `client.ackHandlers["1"]`
- Linea 285: `client.ackHandlers[1]` → `client.ackHandlers["1"]`
- Linea 300: `client.ackHandlers[2]` → `client.ackHandlers["2"]`

**Rationale**: `ackHandlers` modificato da `map[int]` a `map[string]` per compatibilità Phoenix Protocol.

---

#### **FIX 2: Terminologia Domain-Specific nei Test** ✅
**File**: `realtime/channel_test.go`
**Modifiche applicate**:
- Linea 331: `"video:test-device-id"` → `"complex:hierarchical:topic:abc-123-def"`
- Linea 367: `"realtime:video:test-device-id"` → `"realtime:complex:hierarchical:topic:abc-123-def"`

**Rationale**: Rimozione terminologia domain-specific (video/device) e sostituzione con topic generico complesso che dimostra supporto per gerarchie elaborate.

---

#### **FIX 3 PARZIALE: Rimozione Log DEBUG** ✅
**File**: `realtime/channel.go`
**Modifiche applicate**:
- Linee 122-123: Rimosso commento e log `[DEBUG_PAYLOAD]`

**Rationale**: Log DEBUG troppo verbose per production.

---

## ⏳ FIX RIMANENTI (da completare)

### **FIX 3 COMPLETO: Rimuovere TUTTI i Log DEBUG**

**File**: `realtime/realtime_client.go`
**Occorrenze da rimuovere** (10 totali):

```
Linea 240-241:
// DEBUG: Log ALL incoming messages
c.logger.Printf("[DEBUG_RX_ALL] Received: %s", string(data))

Linea 269-272:
// DEBUG: Log received message for broadcast debugging
if msg.Event == "broadcast" {
    c.logger.Printf("[DEBUG_RX_BROADCAST] event=%s type=%s topic=%s", ...)
}

Linea 451-460:
// DEBUG: Log all registered channels
c.logger.Printf("[DEBUG_BROADCAST_LOOKUP] Looking for channel: %s", msg.Topic)
c.logger.Printf("[DEBUG_BROADCAST_LOOKUP] Registered channels: %v", ...)

Linea 463:
c.logger.Printf("[DEBUG_BROADCAST] Channel not found: %s", msg.Topic)

Linea 504:
c.logger.Printf("[DEBUG_BROADCAST] Calling %d handlers for event: %s", ...)
```

**LOG DA MANTENERE** (errori reali):
- Linea 475: `Error parsing broadcast payload: %v` ✅
- Linea 486: `Error decoding Base64 payload: %v` ✅
- Linea 500: `No handler for event: %s` ✅

**Azione**: Rimuovere completamente log e commenti DEBUG, mantenere solo log di errori reali.

---

### **FIX 4: Riscrivere Commenti "CRITICAL" Professionalmente**

**File**: `realtime/realtime_client.go`

**PRIMA** (linee 318-325):
```go
// Phoenix Channel protocol: heartbeat event
// CRITICAL: All Phoenix messages MUST include a "payload" field
heartbeatMsg := struct {
    Topic   string         `json:"topic"`
    Event   string         `json:"event"`
    Payload map[string]any `json:"payload"`
    Ref     int            `json:"ref"`
}{
    Topic:   "phoenix",
    Event:   "heartbeat",
    Payload: map[string]any{}, // Empty payload required by Phoenix protocol
    Ref:     c.NextRef(),
}
```

**DOPO**:
```go
// Phoenix protocol requires payload field in all messages
heartbeatMsg := struct {
    Topic   string         `json:"topic"`
    Event   string         `json:"event"`
    Payload map[string]any `json:"payload"`
    Ref     int            `json:"ref"`
}{
    Topic:   "phoenix",
    Event:   "heartbeat",
    Payload: map[string]any{},
    Ref:     c.NextRef(),
}
```

**File**: `realtime/channel.go`

**PRIMA** (linee 188-200):
```go
// Phoenix Channel protocol: phx_leave event for unsubscribe
// CRITICAL: All Phoenix messages MUST include a "payload" field
unsubscribeMsg := struct {
    Topic   string         `json:"topic"`
    Event   string         `json:"event"`
    Payload map[string]any `json:"payload"`
    Ref     string         `json:"ref"`
}{
    Topic:   ch.topic,
    Event:   "phx_leave",
    Payload: map[string]any{}, // Empty payload required by Phoenix protocol
    Ref:     fmt.Sprintf("%d", ch.client.NextRef()),
}
```

**DOPO**:
```go
// Phoenix protocol requires payload field in all messages
unsubscribeMsg := struct {
    Topic   string         `json:"topic"`
    Event   string         `json:"event"`
    Payload map[string]any `json:"payload"`
    Ref     string         `json:"ref"`
}{
    Topic:   ch.topic,
    Event:   "phx_leave",
    Payload: map[string]any{},
    Ref:     fmt.Sprintf("%d", ch.client.NextRef()),
}
```

**Rationale**:
- Rimuovere "CRITICAL" (troppo enfatico per production code)
- Un solo commento conciso (rimuovere ridondanza)
- Rimuovere commenti inline ridondanti
- Professionale, non allarmistico

---

### **FIX 5: Rimuovere Commenti Inline Ridondanti**

**File**: `realtime/channel.go`

**PRIMA** (linee 247-264):
```go
// Supabase Realtime broadcast protocol
// Reference: Official JavaScript library format
broadcastMsg := struct {
    Topic   string `json:"topic"`
    Event   string `json:"event"`
    Payload any    `json:"payload"`
    Ref     string `json:"ref"`
}{
    Topic: ch.topic,
    Event: "broadcast", // Always "broadcast" for Supabase
    Payload: map[string]any{
        "type":    "broadcast",
        "event":   event,
        "payload": payload,
    },
    Ref: fmt.Sprintf("%d", ch.client.NextRef()),
}
```

**DOPO**:
```go
// Phoenix Channel broadcast message format
// Reference: https://supabase.com/docs/guides/realtime/protocol
broadcastMsg := struct {
    Topic   string `json:"topic"`
    Event   string `json:"event"`
    Payload any    `json:"payload"`
    Ref     string `json:"ref"`
}{
    Topic: ch.topic,
    Event: "broadcast",
    Payload: map[string]any{
        "type":    "broadcast",
        "event":   event,
        "payload": payload,
    },
    Ref: fmt.Sprintf("%d", ch.client.NextRef()),
}
```

**Altri commenti inline da rimuovere**:
- Linea 109: `// Empty array per protocol` (ridondante)
- Ogni `// Empty payload required` (ovvio dal codice)

**Rationale**: Commenti inline devono aggiungere valore, non ripetere codice ovvio.

---

### **FIX 6: FlexibleRef Error Handling**

**File**: `realtime/types.go`

**PRIMA** (linee 106-130):
```go
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

    // If both fail, set to empty
    *f = ""
    return nil  // ← PROBLEMA: Nasconde l'errore
}
```

**DOPO**:
```go
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
```

**Rationale**: Errori di unmarshaling devono essere visibili, non nascosti con `return nil`.

---

### **FIX 7: Aggiungere GetShortTopic() Helper**

**File**: `realtime/channel.go`

**Aggiungere dopo GetTopic()** (circa linea 346):
```go
// GetTopic returns the full topic with "realtime:" prefix
func (c *channel) GetTopic() string {
    return c.topic
}

// GetShortTopic returns the topic without "realtime:" prefix
func (c *channel) GetShortTopic() string {
    return strings.TrimPrefix(c.topic, "realtime:")
}
```

**File**: `realtime/types.go`

**Aggiornare interfaccia Channel** (circa linea 103):
```go
type Channel interface {
    Subscribe(ctx context.Context, callback func(SubscribeState, error)) error
    Unsubscribe() error
    OnBroadcast(event string, callback func(json.RawMessage)) error
    SendBroadcast(event string, payload any) error
    OnPostgresChange(event string, callback func(PostgresChangeEvent)) error
    OnMessage(callback func(Message))
    Track(payload any) error
    Untrack() error
    GetTopic() string
    GetShortTopic() string  // NEW
}
```

**Rationale**:
- Backward compatibility per codice che usa `GetTopic()`
- Alcuni use case potrebbero aver bisogno del topic senza prefix
- Breaking change mitigato

---

### **FIX 8: Aggiornare CHANGELOG**

**File**: `CHANGELOG.md`

**Aggiungere in `[Unreleased]` PRIMA di `### Added`**:

```markdown
## [v0.1.7-dam] - 2025-10-20

### Fixed
- **BLOCKER**: Phoenix Channel Protocol compliance - server crashes resolved
  - Added required `payload` field to `heartbeat` messages (prevents GenServer crash)
  - Added required `payload` field to `phx_leave` messages (prevents connection errors)
  - Fixed broadcast message format with nested payload structure per Supabase protocol
  - Implements Base64 decoding for Supabase broadcast payloads
  - All Phoenix messages now include mandatory fields per protocol specification
  - Prevents: `Phoenix.Socket.InvalidMessageError: missing key "payload"`
  - Reference: https://hexdocs.pm/phoenix/Phoenix.Socket.Message.html
- **CRITICAL**: FlexibleRef type for Phoenix protocol compatibility
  - Handles both string and number `ref` values from Phoenix server responses
  - Prevents unmarshaling errors: `cannot unmarshal number into Go struct field`
  - Custom UnmarshalJSON implementation with proper error handling
- **BREAKING**: Topic prefix "realtime:" now added automatically to all channels
  - Internal implementation detail transparent to most API consumers
  - `GetTopic()` now returns full topic with "realtime:" prefix
  - Added `GetShortTopic()` helper for backward compatibility
  - Required by Supabase Realtime protocol specification

### Changed
- Removed verbose DEBUG logging for production readiness
  - Removed `[DEBUG_RX_ALL]`, `[DEBUG_PAYLOAD]`, `[DEBUG_BROADCAST_*]` logs
  - Maintained error logging for troubleshooting (parsing errors, Base64 failures)
  - Production-ready log levels following Go best practices 2025
- Improved code comment quality and professionalism
  - Removed emphatic "CRITICAL" markers (replaced with technical descriptions)
  - Eliminated redundant inline comments
  - Comments now describe current behavior, not implementation history
  - Follows Effective Go comment guidelines

### Testing
- Fixed test suite for Phoenix protocol changes
  - Updated `ackHandlers` map access from `int` to `string` keys
  - Replaced domain-specific test data with generic hierarchical topics
  - All 38 unit tests passing with protocol-compliant changes
```

---

## 🧪 TEST PLAN (dopo completamento fix)

### **1. Test Unitari**
```bash
cd c:/dev/dam/realtime-go
go test ./... -v
```

**Aspettative**: Tutti i test passano (38/38)

### **2. Race Detector**
```bash
go test -race ./...
```

**Aspettative**: 0 data races

### **3. Test Integrazione Phoenix Protocol (60s)**
```bash
cd c:/dev/dam/dam-poc-injector
go run test-heartbeat-fix.go
```

**Aspettative**:
- 60 secondi connessione stabile
- 2 heartbeat inviati con successo
- Nessun errore `missing key "payload"`
- Graceful shutdown pulito (no StatusInternalError)

### **4. Test Cross-Language Go ↔ JavaScript**
```bash
# Terminal 1
cd c:/dev/dam/dam-poc-injector
go run test-go-for-browser.go

# Terminal 2 (browser)
# Aprire test-bidirectional.html
```

**Aspettative**:
- Messaggi Go → JavaScript ricevuti correttamente
- Messaggi JavaScript → Go ricevuti correttamente
- Base64 e JSON payload gestiti correttamente

### **5. Verifica Log Supabase**
Controllare log Supabase per assenza errori:
- ❌ NO `Phoenix.Socket.InvalidMessageError`
- ❌ NO `missing key "payload"`
- ✅ SOLO messaggi broadcast ricevuti correttamente

---

## 📝 COMMIT STRATEGY

### **Commit 1: Fix test failures e terminologia**
```bash
git add realtime/channel_test.go
git commit -m "test: fix ackHandlers type mismatch and remove domain-specific terminology

- Update ackHandlers map access from int to string keys (Phoenix protocol)
- Replace 'video:test-device-id' with generic 'complex:hierarchical:topic:abc-123-def'
- Demonstrates support for complex topic hierarchies
- Maintains domain-agnostic test examples"
```

### **Commit 2: Remove DEBUG logging**
```bash
git add realtime/channel.go realtime/realtime_client.go
git commit -m "refactor: remove verbose DEBUG logging for production readiness

- Remove [DEBUG_RX_ALL], [DEBUG_PAYLOAD], [DEBUG_BROADCAST_*] logs
- Maintain error logs for troubleshooting (parsing, Base64 decoding)
- Follow Go logging best practices 2025
- Production-ready log levels"
```

### **Commit 3: Improve code comments quality**
```bash
git add realtime/channel.go realtime/realtime_client.go
git commit -m "docs: improve code comment quality and professionalism

- Remove emphatic 'CRITICAL' markers
- Eliminate redundant inline comments
- Comments describe current behavior, not history
- Follow Effective Go comment guidelines
- Professional technical descriptions"
```

### **Commit 4: Fix FlexibleRef error handling**
```bash
git add realtime/types.go
git commit -m "fix: improve FlexibleRef error handling

- Return error instead of nil when unmarshaling fails
- Prevents silent failures in ref parsing
- Proper error propagation for debugging"
```

### **Commit 5: Add GetShortTopic() helper**
```bash
git add realtime/channel.go realtime/types.go
git commit -m "feat: add GetShortTopic() helper for backward compatibility

- Returns topic without 'realtime:' prefix
- Mitigates breaking change from automatic prefix
- Maintains API flexibility for existing consumers"
```

### **Commit 6: Update CHANGELOG**
```bash
git add CHANGELOG.md
git commit -m "docs: update CHANGELOG for v0.1.7-dam release

- Document Phoenix Channel Protocol compliance fixes
- Document breaking changes and migration path
- Add FlexibleRef type improvements
- Describe production readiness improvements"
```

---

## 🚀 RELEASE PROCESS

### **1. Tag Release**
```bash
git tag v0.1.7-dam
git push origin v0.1.7-dam
```

### **2. Create GitHub Release**
```bash
gh release create v0.1.7-dam \
  --title "v0.1.7-dam - Phoenix Protocol Compliance & Production Hardening" \
  --notes "## Highlights

- **BLOCKER FIX**: Phoenix Channel Protocol compliance - eliminates server crashes
- **Production Ready**: Removed verbose DEBUG logging
- **Quality Improvements**: Professional code comments and error handling
- **Backward Compatibility**: GetShortTopic() helper for migration

See CHANGELOG.md for complete details.

## Breaking Changes
- \`GetTopic()\` now returns full topic with \"realtime:\" prefix
- Use \`GetShortTopic()\` for topic without prefix

## Migration
No action required for most users. If you use \`GetTopic()\` for string matching:
\`\`\`go
// Before
if channel.GetTopic() == \"my-channel\" { ... }

// After - Option 1
if channel.GetShortTopic() == \"my-channel\" { ... }

// After - Option 2
if channel.GetTopic() == \"realtime:my-channel\" { ... }
\`\`\`"
```

---

## 🎯 DELIVERABLES FINALI

### **Modifiche Codice**
- ✅ `realtime/channel_test.go` - test fixes
- ✅ `realtime/channel.go` - log cleanup, comment quality, GetShortTopic()
- ✅ `realtime/realtime_client.go` - log cleanup, comment quality
- ✅ `realtime/types.go` - FlexibleRef error handling, Channel interface update
- ✅ `CHANGELOG.md` - versione v0.1.7-dam documentata

### **Testing**
- ✅ 38/38 test unitari passano
- ✅ 0 data races
- ✅ 60s test integrazione Phoenix Protocol senza errori
- ✅ Cross-language Go ↔ JavaScript funzionante
- ✅ Log Supabase puliti (no Phoenix errors)

### **Versioning**
- ✅ Tag: `v0.1.7-dam`
- ✅ GitHub Release con note complete
- ✅ CHANGELOG aggiornato

---

## 📚 RIFERIMENTI

### **Phoenix Channel Protocol**
- Documentazione: https://hexdocs.pm/phoenix/Phoenix.Socket.Message.html
- Supabase Realtime: https://supabase.com/docs/guides/realtime/protocol

### **Go Best Practices 2025**
- Effective Go: https://go.dev/doc/effective_go
- Go Code Review Comments: https://go.dev/wiki/CodeReviewComments
- Logging Best Practices: https://uptrace.dev/blog/golang-logging

### **Project CLAUDE.md Rules**
- Comments Document Current State, NOT Change History
- No Gold-Plating - pragmatic engineering
- Surgical Modifications - precise, targeted changes
- Professional Git Commit Standards (Conventional Commits)

---

## ✅ CHECKLIST FINALE

Prima del commit finale, verificare:

- [ ] Tutti i 38 test unitari passano
- [ ] Race detector pulito (0 races)
- [ ] Test integrazione 60s stabile
- [ ] Cross-language communication funzionante
- [ ] Log Supabase senza errori Phoenix
- [ ] CHANGELOG aggiornato con v0.1.7-dam
- [ ] Nessun log DEBUG rimasto nel codice
- [ ] Commenti professionali e concisi
- [ ] FlexibleRef error handling corretto
- [ ] GetShortTopic() implementato
- [ ] Nessuna terminologia domain-specific nei test
- [ ] Commit messages seguono Conventional Commits
- [ ] Tag v0.1.7-dam creato
- [ ] GitHub Release pubblicato

---

**Fine Piano di Refactoring**
**Prossima Sessione**: Applicare fix rimanenti 3-8, eseguire test, committare
