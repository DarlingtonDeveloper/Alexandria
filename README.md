# Alexandria

**The Memory of the Swarm** — Persistent knowledge, secrets, and context for the Warren agent swarm.

Alexandria (formerly "Vault" in the spec) provides:
- **Knowledge Store** — Structured entries with tags, embeddings, source attribution, and temporal metadata
- **Knowledge Graph** — Entity-relationship layer connecting people, projects, decisions, and events
- **Secret Management** — Fernet-encrypted credential storage with scoped access and audit
- **Context Rehydration** — Automated wake-up briefings for agents assembled from knowledge and events
- **Semantic Search** — pgvector embeddings for natural language queries across all swarm knowledge
- **Event-Driven Capture** — Hermes (NATS) subscriber that auto-persists discoveries and events

## Quick Start

### Prerequisites
- Go 1.24+
- PostgreSQL with pgvector extension (Supabase)
- NATS server (optional, for Hermes integration)

### Run the Migration

Apply the schema to your Supabase PostgreSQL database:

```bash
psql $DATABASE_URL < migrations/001_alexandria_schema.sql
```

### Configure Environment

```bash
export DATABASE_URL="postgresql://postgres:password@db.uaubofpmokvumbqpeymz.supabase.co:5432/postgres"
export ENCRYPTION_KEY="your-fernet-key-here"  # Generate with: python -c "from cryptography.fernet import Fernet; print(Fernet.generate_key().decode())"
export NATS_URL="nats://localhost:4222"        # Optional
export EMBEDDING_BACKEND="local"               # "local" (default), "simple", or "openai"
export EMBEDDING_SIDECAR_URL="http://localhost:8501"  # Required if EMBEDDING_BACKEND=local
export OPENAI_API_KEY=""                       # Required if EMBEDDING_BACKEND=openai
```

### Build & Run

```bash
go build -o alexandria cmd/alexandria/main.go
./alexandria
```

### Docker

```bash
docker build -t alexandria .
docker run -p 8500:8500 \
  -e DATABASE_URL="..." \
  -e ENCRYPTION_KEY="..." \
  alexandria
```

### Local Embeddings Sidecar

The default embedding backend (`local`) uses a Python sidecar running `all-MiniLM-L6-v2` (384 dimensions). Start it alongside Alexandria:

```bash
cd embeddings-sidecar
docker build -t alexandria-embeddings .
docker run -p 8501:8501 alexandria-embeddings
```

The model is baked into the Docker image at build time — no runtime downloads needed.

## API Reference

Base URL: `http://localhost:8500/api/v1`

All requests should include `X-Agent-ID` header identifying the calling agent.

### Health
| Method | Path | Description |
|--------|------|-------------|
| GET | `/health` | Health check |
| GET | `/stats` | Knowledge/secret counts, uptime |

### Knowledge
| Method | Path | Description |
|--------|------|-------------|
| POST | `/knowledge` | Create entry |
| GET | `/knowledge` | List (with filters: category, scope, source_agent, limit, offset) |
| GET | `/knowledge/{id}` | Get by ID |
| PUT | `/knowledge/{id}` | Update |
| DELETE | `/knowledge/{id}` | Soft-delete |
| POST | `/knowledge/search` | Semantic search |
| POST | `/knowledge/batch` | Batch create (up to 100) |

### Secrets
| Method | Path | Description |
|--------|------|-------------|
| POST | `/secrets` | Create |
| GET | `/secrets` | List names (no values) |
| GET | `/secrets/{name}` | Get decrypted value (scoped, audited) |
| PUT | `/secrets/{name}` | Update value |
| DELETE | `/secrets/{name}` | Delete |
| POST | `/secrets/{name}/rotate` | Rotate (saves history) |

### Briefings
| Method | Path | Description |
|--------|------|-------------|
| GET | `/briefings/{agent_id}?since=ISO8601&max_items=50` | Generate wake-up briefing |

### Boot Context
| Method | Path | Description |
|--------|------|-------------|
| GET | `/context/{agent_id}` | Generate agent-specific boot context (returns `text/markdown`) |

The boot context endpoint assembles a markdown document with sections for the agent's owner, known people, peer agents, accessible secrets/channels, operational rules, and infrastructure services. Each agent has a profile that controls scope (e.g. `kai` sees everything, `lily` is scoped to owner `mike-a`).

### Knowledge Graph
| Method | Path | Description |
|--------|------|-------------|
| GET | `/graph/entities` | List entities (filter by type) |
| POST | `/graph/entities` | Create entity |
| GET | `/graph/entities/{id}` | Get entity with relationships |
| GET | `/graph/entities/{id}/related?depth=2` | Traverse related entities |
| POST | `/graph/relationships` | Create relationship |

### Response Format

Success:
```json
{"data": {...}, "meta": {"timestamp": "..."}}
```

Error:
```json
{"error": {"code": "...", "message": "..."}, "meta": {"timestamp": "..."}}
```

### Rate Limits
- Knowledge endpoints: 100 req/min per agent
- Secret reads: 10 req/min per agent
- Briefing generation: 5 req/min per agent

## Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `DATABASE_URL` | (required) | PostgreSQL connection string |
| `ALEXANDRIA_PORT` | 8500 | HTTP port |
| `ALEXANDRIA_LOG_LEVEL` | info | Log level (info, debug) |
| `ENCRYPTION_KEY` | | Fernet encryption key |
| `ENCRYPTION_KEY_PATH` | /run/secrets/vault_encryption_key | Path to key file |
| `NATS_URL` | nats://localhost:4222 | NATS server URL |
| `EMBEDDING_BACKEND` | local | Embedding provider (local, simple, openai) |
| `EMBEDDING_SIDECAR_URL` | http://localhost:8501 | Local sidecar URL |
| `OPENAI_API_KEY` | | OpenAI API key |
| `OPENAI_EMBEDDING_MODEL` | text-embedding-3-small | OpenAI model |
| `JWT_SECRET` | | JWT verification secret (Phase 2) |
| `KNOWLEDGE_RATE_LIMIT` | 100 | Knowledge req/min |
| `SECRET_RATE_LIMIT` | 10 | Secret req/min |
| `BRIEFING_RATE_LIMIT` | 5 | Briefing req/min |

## Architecture

See [ARCHITECTURE.md](ARCHITECTURE.md) for detailed component diagrams and design decisions.
