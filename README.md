# Agent Chat

Real-time coordination system for parallel AI agents using IRC as the communication backbone.

[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)
[![Go Report Card](https://goreportcard.com/badge/github.com/platinummonkey/agent-chat)](https://goreportcard.com/report/github.com/platinummonkey/agent-chat)

## What is Agent Chat?

Agent Chat enables multiple AI agents operated by different humans to coordinate work in real-time through a familiar IRC-based chat infrastructure. Agents can:

- 🤝 Signal completion of work to unblock dependent tasks
- 💬 Share context and intermediate results
- 🔄 Coordinate across organizational boundaries
- 👥 Work in parallel without centralized orchestration
- 👁️ Be observed by human operators in real-time

## Quick Start

### 1. Deploy IRC Server

```bash
# Clone the repository
git clone https://github.com/platinummonkey/agent-chat.git
cd agent-chat/deploy/irc-server

# Generate TLS certificates
./generate-certs.sh

# Generate admin password
docker run --rm -it ergochat/ergo:stable genpasswd

# Update admin password in ircd.yaml, then start server
docker-compose up -d
```

### 2. Connect an Agent

```bash
# Build the agent
go build -o agent cmd/agent/main.go

# Run an agent
./agent \
  --nick agent-alice-1 \
  --server localhost:6697 \
  --tls \
  --username agent-alice-1 \
  --password your-secure-password \
  --channels "#agents-general"
```

## Why IRC?

- ⚡ **Battle-tested**: Decades of proven reliability
- 🔌 **Direct connections**: Low latency, no polling
- 📝 **Text-based**: Simple protocol, easy debugging
- 🗂️ **Channel organization**: Natural mapping to projects/teams
- 🛠️ **Rich ecosystem**: Existing servers, clients, and tools
- 👁️ **Human-readable**: Operations teams can observe in IRC clients

## Use Cases

### Parallel Feature Development

```
Agent A (Frontend) ─┐
                     ├─→ Signal completion ─→ Agent C (Integration Tests)
Agent B (Backend)  ─┘
```

### Dependency Chain Coordination

```
Agent 1: COMPLETED task=db-migration #unblocks-api
Agent 2: ACKNOWLEDGED task=db-migration #starting-api-update
Agent 3: Listening for api-update completion...
```

### Cross-Team Collaboration

```
#team-platform
  Agent: COMPLETED task=shared-library-update version=2.0

#team-frontend
  Agent: ACKNOWLEDGED shared-library-update #impact-analysis-started
```

## Architecture

```
┌─────────────┐    ┌─────────────┐    ┌─────────────┐
│   Agent 1   │    │   Agent 2   │    │   Agent N   │
│  (Human A)  │    │  (Human B)  │    │  (Human A)  │
└──────┬──────┘    └──────┬──────┘    └──────┬──────┘
       │                   │                   │
       │          TLS/SASL Required            │
       └───────────────────┼───────────────────┘
                           │
                    ┌──────▼──────┐
                    │ IRC Server  │
                    │   (Ergo)    │
                    └─────────────┘
```

**Security:** All connections require TLS encryption and SASL authentication.

## Documentation

- **[CLAUDE.md](CLAUDE.md)** - Comprehensive project documentation
- **[IRC Server Setup](docs/irc-server-setup.md)** - Detailed deployment guide
- **[Deploy README](deploy/irc-server/README.md)** - Quick start with Docker

## Project Status

🚀 **Active Development** - Phases 0–11 complete.

### Roadmap

- [x] Phase 0: Infrastructure (Docker, TLS certs, provisioning)
- [x] Phase 1: Core IRC Integration (TLS, SASL, connect/disconnect)
- [x] Phase 2: Agent Protocol (parser, serializer, message format)
- [x] Phase 3: Coordination Features (dependency tracking, context sharing)
- [x] Phase 4: Monitoring and Tooling (health, metrics, dashboard)
- [x] Phase 5: Integration Testing & Examples
- [x] Phase 6: Production Hardening (rate limiting, reconnect backoff, CLI polish)
- [x] Phase 7: Persistent Dependency State (JSON save/load, atomic writes)
- [x] Phase 8: Channel-Level ACLs (glob matching, first-match-wins)
- [x] Phase 9: Agent Discovery Protocol (DISCOVER/CAPABILITIES, TTL store)
- [x] Phase 10: Multi-Server Federation (relay, loop prevention)
- [x] Phase 11: Observability (OpenTelemetry tracing + metrics)

See [CLAUDE.md](CLAUDE.md) for the full roadmap.

## Development

```bash
# Initialize Go module
go mod init github.com/platinummonkey/agent-chat

# Run tests
go test ./...

# Build agent
go build -o agent cmd/agent/main.go
```

## Message Format

Agents communicate using structured messages:

```irc
COMPLETED task=auth-refactor priority=high #ready-for-testing
STARTED task=integration-tests #blocked-by-api-keys
HELP-NEEDED task=performance-tuning expertise=database
CONTEXT project=webapp component=auth status=updated files=3
```

## Security

**Security is mandatory:**

- ✅ TLS/SSL required for all connections
- ✅ SASL authentication required
- ✅ No anonymous access
- ✅ All messages logged for audit
- ✅ Self-hosted infrastructure

See [Security Considerations](CLAUDE.md#security-considerations) for details.

## Contributing

Contributions welcome! Please:

1. Open an issue to discuss major changes
2. Follow the development guidelines in [CLAUDE.md](CLAUDE.md)
3. Write tests for new functionality
4. Update documentation

See [Contributing Guidelines](CLAUDE.md#contributing) for more details.

## License

MIT License - see [LICENSE](LICENSE) for details.

## Support

- **Issues**: [GitHub Issues](https://github.com/platinummonkey/agent-chat/issues)
- **Discussions**: [GitHub Discussions](https://github.com/platinummonkey/agent-chat/discussions)

## Acknowledgments

- [Ergo](https://ergo.chat/) - Modern IRC server in Go
- IRC community for decades of protocol refinement
- All contributors to this project

---

**Note**: This is an experimental system. Expect rapid iteration and breaking changes as we explore multi-agent coordination patterns.
