# Warren Integration for Alexandria

## Service Configuration (add to Warren's services config)

```yaml
alexandria:
  image: alexandria:latest
  port: 8500
  health_check: /api/v1/health
  policy: on-demand
  wake_timeout: 30s
  secrets:
    - database_url
    - vault_encryption_key
    - openai_api_key
  environment:
    ALEXANDRIA_PORT: "8500"
    ALEXANDRIA_LOG_LEVEL: "info"
    EMBEDDING_BACKEND: "simple"
    NATS_URL: "nats://warren_hermes:4222"
```

## Wake-up Briefing Injection

During agent wake sequence, Warren should:

1. Scale Alexandria replicas 0 → 1 (if not running)
2. Wait for health check at `/api/v1/health`
3. `GET /api/v1/briefings/{agent_id}?since={last_sleep_time}`
4. Inject briefing JSON into agent context (file mount at `/vault/briefing.json` or startup message)
5. Agent boots, reads briefing, has context

## Example Wake Sequence Code

```python
briefing = requests.get(
    f"http://alexandria:8500/api/v1/briefings/{agent_id}",
    params={"since": last_sleep_time.isoformat()},
    headers={"X-Agent-ID": "warren"}
).json()
```
