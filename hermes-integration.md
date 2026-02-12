# Hermes Integration for Alexandria

## Stream/Subject Additions

Alexandria subscribes to these existing subjects:
- `swarm.discovery.>` — Auto-persist as knowledge (category: discovery)
- `swarm.task.*.completed` — Persist as knowledge (category: event)
- `swarm.task.*.failed` — Persist as knowledge (category: lesson)
- `swarm.agent.*.started` — Track agent lifecycle
- `swarm.agent.*.stopped` — Track agent sleep time

Alexandria publishes these new subjects:
- `swarm.vault.knowledge.created`
- `swarm.vault.knowledge.updated`
- `swarm.vault.knowledge.searched`
- `swarm.vault.secret.accessed`
- `swarm.vault.secret.rotated`
- `swarm.vault.secret.rotation_due`
- `swarm.vault.briefing.generated`

## JetStream Consumer Config

```json
{
  "stream": "DISCOVERIES",
  "durable_name": "alexandria-knowledge-capture",
  "deliver_policy": "all",
  "ack_policy": "explicit",
  "max_deliver": 3,
  "ack_wait": "30s"
}
```

## Required Hermes Streams

Ensure these JetStream streams exist:
- `DISCOVERIES` — for `swarm.discovery.>` subjects
- `TASKS` — for `swarm.task.>` subjects
- `AGENTS` — for `swarm.agent.>` subjects
- `VAULT` — for `swarm.vault.>` subjects (new)
