# Alexandria Architecture

## System Context

```
Warren (infra) → Hermes (NATS) → PromptForge (identity) → Alexandria (memory)
```

Alexandria is the persistent memory layer of the Warren agent swarm. It sits alongside PromptForge (which manages agent identities) and communicates via Hermes (NATS message bus).

## Component Diagram

```
┌──────────────────────────────────────────────────────┐
│                    Alexandria                         │
│                                                       │
│  ┌─────────┐  ┌───────────┐  ┌──────────┐           │
│  │   API   │  │  Briefing │  │  Hermes  │           │
│  │Handlers │  │ Assembler │  │Subscriber│           │
│  └────┬────┘  └─────┬─────┘  └────┬─────┘           │
│       │              │              │                 │
│  ┌────┴──────────────┴──────────────┴────┐           │
│  │              Store Layer              │           │
│  │  Knowledge │ Secrets │ Graph │ Audit  │           │
│  └──────────────────┬───────────────────┘           │
│                      │                               │
│  ┌──────────┐  ┌────┴─────┐  ┌──────────┐          │
│  │Embeddings│  │PostgreSQL│  │Encryption│          │
│  │ Provider │  │  (pgx)   │  │ (Fernet) │          │
│  └─────┬────┘  └──────────┘  └──────────┘          │
│        │                                             │
│  ┌─────┴──────────────────────────────┐              │
│  │  local │ simple │ openai           │              │
│  └─────┬──────────────────────────────┘              │
│                                                       │
│  ┌──────────────────────────────────────┐            │
│  │         Middleware Layer             │            │
│  │  Auth │ Rate Limit │ Logging        │            │
│  └──────────────────────────────────────┘            │
└──────────────────────────────────────────────────────┘
         │              │               │
    ┌────┴────┐   ┌─────┴─────┐  ┌────┴──────────┐
    │Supabase │   │  Hermes   │  │  Embeddings   │
    │PostgreSQL│   │  (NATS)   │  │  Sidecar      │
    └─────────┘   └───────────┘  │ (MiniLM-L6)  │
                                  └───────────────┘
```

## Data Flow

### Knowledge Creation (explicit)
```
Agent → POST /api/v1/knowledge → Generate embedding → Store in PostgreSQL → Publish vault.knowledge.created on NATS
```

### Knowledge Creation (automatic via Hermes)
```
Agent publishes swarm.discovery.* → Alexandria subscriber → Extract content → Generate embedding → Store → Publish vault.knowledge.created
```

### Semantic Search
```
Agent → POST /api/v1/knowledge/search → Generate query embedding → pgvector cosine similarity → Apply relevance decay → Return ranked results
```

### Context Rehydration
```
Warren waking agent → GET /api/v1/briefings/{agent_id} → Query recent events + relevant knowledge + agent context → Return structured briefing
```

## Technology Stack

| Component | Technology | Rationale |
|-----------|-----------|-----------|
| Language | Go | Performance, single binary, matches infrastructure focus |
| Router | chi v5 | Lightweight, idiomatic, middleware support |
| Database | pgx v5 | Direct PostgreSQL driver, best Go Postgres support |
| Vectors | pgvector-go | pgvector support for embedding storage and search |
| Encryption | fernet-go | Symmetric encryption for secrets (AES-128-CBC + HMAC) |
| Message Bus | nats.go | NATS client for Hermes integration |
| Logging | log/slog | Standard library structured logging |

## Design Decisions

1. **pgx over Supabase REST API**: Direct PostgreSQL gives us transactions, prepared statements, and pgvector support without REST overhead.

2. **Local embedding sidecar (default)**: A Python sidecar running `all-MiniLM-L6-v2` via sentence-transformers provides real semantic embeddings (384 dimensions) without external API dependencies. The model is baked into the Docker image at build time. A hash-based simple provider and OpenAI provider are also available as alternatives.

3. **Soft deletes for knowledge**: Entries are marked with `deleted_at` rather than removed, preserving audit trail and allowing recovery.

4. **On-demand with durable consumers**: JetStream durable consumers let Alexandria catch up on missed events after wake-up, enabling Warren's scale-to-zero policy.

5. **Per-agent rate limiting**: In-memory rate limiter with configurable limits per endpoint category prevents abuse without external dependencies.

6. **Trust-based auth (Phase 1)**: Internal overlay network means `X-Agent-ID` header is sufficient. JWT verification planned for Phase 2.

## Database Schema

All tables use `vault_` prefix. See `migrations/001_alexandria_schema.sql` for full schema including:
- `vault_knowledge` — Knowledge entries with embeddings
- `vault_entities` — Knowledge graph entities
- `vault_relationships` — Entity relationships
- `vault_secrets` — Encrypted credentials
- `vault_secret_history` — Rotation history
- `vault_access_log` — Audit trail
- `vault_semantic_search()` — PostgreSQL function for vector search
