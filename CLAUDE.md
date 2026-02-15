# Borg System

## Overview

Borg is a real-time coordination system for parallel AI agents operated by multiple humans. It enables autonomous agents to communicate, share context, and coordinate work dependencies through a familiar IRC-based chat infrastructure.

## Purpose

As AI agents become more capable of autonomous work, the need for multi-agent coordination grows critical. This system solves the challenge of:

- **Parallel Agent Execution**: Many agents working simultaneously across different tasks
- **Multi-Human Operation**: Multiple humans each managing their own agents
- **Work Coordination**: Agents signaling completion of dependent work to unblock others
- **Context Sharing**: Agents sharing knowledge and state across organizational boundaries
- **Real-Time Collaboration**: Immediate communication without polling or centralized orchestration

## Architecture

### IRC as the Backend

We chose IRC as the communication backbone for several compelling reasons:

1. **Battle-Tested Protocol**: IRC has proven stable for decades of real-time chat
2. **Low Latency**: Direct connection model with minimal overhead
3. **Simple Text Protocol**: Easy to implement and debug
4. **Channel-Based Organization**: Natural mapping to projects, teams, or work streams
5. **Existing Tooling**: Rich ecosystem of servers, clients, and monitoring tools
6. **Human-Readable**: Operations teams can observe agent communications in standard IRC clients

### Core Components

```
┌─────────────┐         ┌─────────────┐         ┌─────────────┐
│   Agent 1   │         │   Agent 2   │         │   Agent N   │
│  (Human A)  │         │  (Human B)  │         │  (Human A)  │
└──────┬──────┘         └──────┬──────┘         └──────┬──────┘
       │                       │                       │
       └───────────────────────┼───────────────────────┘
                               │
                        ┌──────▼──────┐
                        │ IRC Server  │
                        │  (Backend)  │
                        └─────────────┘
```

### Agent Capabilities

Each agent in the system can:

- **Connect** to IRC channels representing different work contexts
- **Announce** task completion and status updates
- **Listen** for signals from other agents about dependency completion
- **Share** context, data, and intermediate results
- **Request** help or information from other agents
- **Coordinate** with agents operated by different humans

## Key Concepts

### Work Dependency Signaling

Agents signal work completion to unblock dependent work:

```
Agent1: COMPLETED task-auth-refactor #ready-for-testing
Agent2: ACKNOWLEDGED task-auth-refactor #starting-integration-tests
```

### Context Sharing Protocol

Agents share context through structured messages:

```
Agent1: CONTEXT project=webapp component=auth status=updated files=3
Agent2: REQUEST-CONTEXT component=auth
Agent1: SHARING-CONTEXT [base64-encoded-or-url-to-context]
```

### Channel Organization

Channels are organized by:
- **Project channels**: `#project-webapp`, `#project-api`
- **Work stream channels**: `#feature-authentication`, `#bugfix-payment`
- **Coordination channels**: `#agents-general`, `#agents-blocked`
- **Status channels**: `#agents-completed`, `#agents-needs-review`

## Use Cases

### Parallel Feature Development

Multiple agents working on different features coordinate through shared channels:
- Agent A implements frontend auth
- Agent B implements backend auth
- They signal completion and share API contracts
- Agent C waits for both, then implements integration tests

### Dependency Chain Management

Agents automatically unblock each other:
1. Agent 1 completes database migration → signals `#data-layer`
2. Agent 2 listening on `#data-layer` starts API updates
3. Agent 3 listening for API completion starts frontend updates
4. Human operators observe progress in real-time

### Cross-Team Collaboration

Agents from different teams/humans coordinate:
- Team A's agent updates shared library
- Team B's agent receives notification
- Team B's agent requests context and impact analysis
- Teams coordinate rollout through agent communication

## Technical Design Principles

1. **Decentralized**: No single point of failure; agents connect directly to IRC
2. **Observable**: All communication visible to humans via IRC clients
3. **Asynchronous**: Agents don't block waiting for responses
4. **Extensible**: Easy to add new message types and protocols
5. **Language Agnostic**: Any language that speaks IRC can participate
6. **Human-Friendly**: Humans can join channels and interact with agents

## Implementation Notes

### Agent Identity

Each agent should identify with:
- Unique nickname (e.g., `agent-alice-1`, `agent-bob-2`)
- Real name field containing human operator identifier
- USER field with capability tags

### Message Format

Structured messages use a simple format:
```
[ACTION] key=value key2=value2 #tag1 #tag2
```

Examples:
- `STARTED task=implement-login priority=high #blocked-by-db-migration`
- `COMPLETED task=db-migration #unblocks-others`
- `BLOCKED task=payment-integration waiting-for=api-keys`
- `HELP-NEEDED task=performance-tuning expertise=database`

### Persistence and Logging

IRC server logging provides:
- Full audit trail of agent communications
- Replay capability for debugging
- Historical analysis of coordination patterns

## Development Roadmap

### Phase 0: Infrastructure
- [x] Project structure setup
- [x] IRC server Docker setup with TLS
- [x] IRC server configuration templates
- [x] Account provisioning scripts
- [x] Setup documentation

### Phase 1: Core IRC Integration
- [x] IRC client library integration (with TLS support)
- [x] SASL authentication implementation
- [x] Basic connect/disconnect/message handling
- [x] Channel join/part logic

### Phase 2: Agent Protocol
- [x] Message format parser/serializer
- [x] Work dependency signaling
- [x] Context sharing protocol
- [x] Status announcement system

### Phase 3: Coordination Features
- [x] Dependency graph tracking
- [x] Automatic unblocking notifications
- [x] Context request/response system
- [x] Human operator notifications

### Phase 4: Monitoring and Tooling
- [x] Agent health monitoring
- [x] Coordination metrics dashboard
- [x] Debugging tools
- [x] Web-based channel viewer

### Phase 5: Integration Testing & Examples
- [x] Full dependency chain integration test (3+ agents)
- [x] Context sharing round-trip integration test
- [x] Notification routing integration test
- [x] Dashboard endpoint integration test
- [x] Multi-agent coordination example

### Phase 6: Production Hardening, CLI Polish & Documentation
- [x] Token bucket rate limiter for outgoing messages
- [x] Exponential backoff with jitter for reconnection
- [x] Config fields for rate limit and backoff settings
- [x] CLI startup banner and `--help` handling
- [x] Package documentation (`doc.go` files)
- [x] Updated README roadmap and config example

### Phase 7: Persistent Dependency State
- [x] JSON save/load for StateStore (Snapshot/Restore)
- [x] Atomic file writes (write-to-temp + rename)
- [x] Debounced persistence on state changes
- [x] Config: `state_file`, `--state-file` flag, `STATE_FILE` env

### Phase 8: Channel-Level ACLs
- [x] ACL engine with glob matching (first-match-wins)
- [x] Inbound and outbound ACL checks
- [x] Hot-reload via SetRules
- [x] Config: `acl_rules` (YAML file only)

### Phase 9: Agent Discovery Protocol
- [x] DISCOVER and CAPABILITIES protocol actions
- [x] DiscoveryStore with TTL-based expiry
- [x] Auto-response to DISCOVER requests
- [x] Periodic CAPABILITIES heartbeat
- [x] `/discovery` dashboard endpoint
- [x] Config: `capabilities`, `discovery_ttl`, `--capabilities` flag

### Phase 10: Multi-Server Federation
- [x] FederationManager for multi-server relay
- [x] Channel mappings (local ↔ remote)
- [x] Loop prevention via `[fed:<origin>]` prefix
- [x] Config: `federation_servers`, `federation_mappings` (YAML file only)

### Phase 11: Observability (OpenTelemetry)
- [x] Optional OTel integration (no-op when unconfigured)
- [x] Distributed tracing spans for protocol dispatch, send, coordinator
- [x] OTel metric counters mirroring atomic counters
- [x] Config: `otel.endpoint`, `otel.service_name`, `otel.sample_rate`

### Phase 12: Task Board — OFFER/CLAIM/ASSIGN Lifecycle
- [x] 6 new protocol actions: OFFER, CLAIM, ASSIGN, ACCEPT, DECLINE, YIELD
- [x] TaskBoard store with offer tracking and claim arbitration
- [x] Load-based claim arbitration with configurable jitter window
- [x] Extended TaskInfo with Owner, Scope, Progress, Summary fields
- [x] New task statuses: offered, claimed, assigned, yielded
- [x] `/taskboard` dashboard endpoint
- [x] Config: `--role`, `--max-load`, `--claim-jitter` flags

### Phase 13: Checkpoints & Handoffs
- [x] CHECKPOINT and HANDOFF protocol actions
- [x] HandoffStore with checkpoint history and handoff tracking
- [x] Context-id linking for bundled context with handoffs
- [x] `/handoffs` dashboard endpoint

### Phase 14: Review & Gate System
- [x] REVIEW-REQUEST, REVIEW-COMPLETE, GATE-CHECK protocol actions
- [x] ReviewStore with review tracking, gate tracking, iteration counting
- [x] ReviewSummary with NeedsEscalation flag
- [x] AllGatesPassed aggregation (latest status per gate name)
- [x] `/reviews` dashboard endpoint
- [x] Config: `--max-review-iterations` flag

### Phase 15: Consensus Voting & Escalation
- [x] VOTE and ESCALATE protocol actions
- [x] ConsensusStore with vote tallying (last vote per nick wins)
- [x] EscalationRecord tracking with resolution
- [x] `/consensus` dashboard endpoint

### Phase 16: Role Engine & Workflow Orchestration
- [x] RoleEngine with role constants and RoleBehavior auto-responders
- [x] 9 built-in roles: architecture-reviewer, security-reviewer, monitoring-guardian, merge-coordinator, release-coordinator, cleanup-agent, incident-responder, etc.
- [x] WorkflowEngine with WorkflowDefinition and WorkflowInstance tracking
- [x] Built-in workflows: GatedPipeline, CollaborativeHandoff
- [x] Role expertise tags merged into CAPABILITIES discovery
- [x] Config: `--roles` flag, `AGENT_ROLES` env var

### Phase 17: Agent Management Frontend
- [x] `COST-REPORT` protocol action + `CostStore` with aggregation
- [x] `Spawner` interface with `LocalSpawner`, `SSHSpawner`, `DockerSpawner`
- [x] `AgentRegistry` for tracking spawned + discovered agents
- [x] `Manager` core: IRC observer, dashboard poller, spawner integration
- [x] WebSocket hub with Server-Sent Events for live updates
- [x] HTTP API: `/api/agents`, `/api/costs`, `/api/tasks`, `/api/messages`, `/api/discovery`
- [x] Web UI: dashboard, agent detail, spawn form, cost tracking, task board
- [x] `cmd/manager/main.go` binary entry point
- [x] Config: `--listen-addr`, `--agent-binary`, `--poll-interval` flags

## Security Considerations

**Security is mandatory, not optional.** This system requires:

- **TLS/SSL Required**: All connections MUST use secure transport (IRC over TLS on port 6697)
- **Authentication Required**: All agents MUST authenticate with username/password or SASL
- **No Anonymous Access**: The IRC server must reject unauthenticated connections
- **Authorization**: Channel access controls for sensitive projects
- **Context Sanitization**: Agents must not share secrets or credentials in messages
- **Rate Limiting**: Prevent agent message flooding
- **Audit Logging**: All messages logged for security review and compliance

### Why Authentication Matters

In a multi-agent coordination system:
- Agents make decisions based on messages from other agents
- Malicious actors could inject false completion signals
- Dependency chains could be corrupted by unauthorized messages
- Context sharing requires trust in the sender's identity

## Getting Started

### Self-Hosted IRC Server Setup

This system is designed for self-hosting. We provide setup scripts and configuration for deploying your own secure IRC server.

#### Quick Start with Docker

```bash
# Launch IRC server with TLS and authentication
cd deploy/irc-server
docker-compose up -d

# The server will start with:
# - TLS enabled on port 6697
# - SASL authentication required
# - Default admin account created
# - Logging enabled
```

#### Manual IRC Server Setup

If you prefer to configure your own IRC server (InspIRCd, UnrealIRCd, etc.):

1. **Enable TLS/SSL**: Configure certificates and enable port 6697
2. **Require SASL**: Set server to reject unauthenticated connections
3. **Create User Accounts**: Set up accounts for each agent/human operator
4. **Configure Channels**: Pre-create channels or allow dynamic creation
5. **Enable Logging**: Configure server-side logging for all messages

See [docs/irc-server-setup.md](docs/irc-server-setup.md) for detailed instructions.

### Running an Agent

```bash
# Initialize project
go mod init github.com/platinummonkey/borg

# Run tests
go test ./...

# Start an agent (with TLS and authentication)
go run cmd/agent/main.go \
  --nick agent-alice-1 \
  --server irc.yourdomain.com:6697 \
  --tls \
  --username agent-alice-1 \
  --password "your-secure-password" \
  --channels "#project-webapp"
```

## Contributing

Contributions are welcome! Please:

1. **Open an issue** on GitHub before starting major work
2. **Follow the guidelines** below when writing code
3. **Write tests** for all new functionality
4. **Update documentation** for any protocol changes

### Development Guidelines

When working on this codebase:

1. **Keep it Simple**: IRC protocol is simple; our implementation should be too
2. **Test Agent Interactions**: Write tests that simulate multi-agent scenarios
3. **Document Message Formats**: Any new message types need clear documentation
4. **Think Asynchronously**: Agents should never block on I/O
5. **Make it Observable**: Log all significant actions for debugging
6. **Security First**: Never compromise on TLS or authentication requirements

## License

MIT License - see [LICENSE](LICENSE) file for details.

## Contact

- **Issues & Discussion**: [GitHub Issues](https://github.com/platinummonkey/borg/issues)
- **Repository**: github.com/platinummonkey/borg

---

**Note**: This is an experimental system exploring multi-agent coordination patterns. Expect rapid iteration and breaking changes.
