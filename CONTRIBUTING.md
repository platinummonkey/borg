# Contributing to Agent Chat

Thank you for your interest in contributing to Agent Chat! This guide will help you get started.

## Getting Started

### Prerequisites

- **Go 1.21+** (see `go.mod` for the exact version)
- **Docker & Docker Compose** (for running the IRC server locally)
- **Git**

### Setting Up Your Development Environment

1. **Fork and clone** the repository:

   ```bash
   git clone https://github.com/<your-username>/borg.git
   cd borg
   ```

2. **Install dependencies**:

   ```bash
   go mod download
   ```

3. **Run the tests** to verify everything works:

   ```bash
   make test
   ```

4. **Start the IRC server** for local development:

   ```bash
   cd deploy/irc-server
   docker-compose up -d
   ```

## How to Contribute

### Reporting Bugs

- Search [existing issues](https://github.com/platinummonkey/borg/issues) first to avoid duplicates.
- Use a clear, descriptive title.
- Include steps to reproduce, expected behavior, and actual behavior.
- Include Go version (`go version`) and OS information.

### Suggesting Features

- Open an issue describing the feature, its use case, and why it benefits the project.
- For major changes, please open an issue **before** starting work so we can discuss the approach.

### Submitting Pull Requests

1. **Create a branch** from `main`:

   ```bash
   git checkout -b feature/your-feature-name
   ```

2. **Make your changes**, following the [coding guidelines](#coding-guidelines) below.

3. **Write or update tests** for your changes.

4. **Run the full test suite**:

   ```bash
   make test
   ```

5. **Commit your changes** with a clear commit message:

   ```bash
   git commit -m "Add feature: brief description of what and why"
   ```

6. **Push your branch** and open a pull request against `main`.

7. **Fill out the PR description** with:
   - A summary of changes
   - Related issue number(s)
   - How to test the changes

## Coding Guidelines

### General Principles

- **Keep it simple.** IRC protocol is simple; our implementation should be too.
- **Think asynchronously.** Agents should never block on I/O.
- **Make it observable.** Log all significant actions for debugging.
- **Security first.** Never compromise on TLS or authentication requirements.

### Go Style

- Follow standard Go conventions and [Effective Go](https://go.dev/doc/effective-go).
- Run `go fmt` and `go vet` before committing.
- Use meaningful variable and function names.
- Keep functions focused and short.

### Testing

- Write tests for all new functionality.
- Use table-driven tests where appropriate.
- Test agent interactions with multi-agent simulation scenarios.
- Place integration tests in `test/integration/`.
- Unit tests live alongside the code they test (`_test.go` files).

### Protocol Changes

- Document any new message types or protocol actions clearly.
- Update `CLAUDE.md` with new phase entries when adding major features.
- Follow the existing message format: `[ACTION] key=value key2=value2 #tag1 #tag2`

### Commit Messages

- Use the imperative mood ("Add feature", not "Added feature").
- Keep the first line under 72 characters.
- Reference related issues where applicable (e.g., `Fixes #42`).

## Project Structure

```
borg/
├── cmd/                    # Binary entry points
│   ├── agent/              # Agent CLI
│   ├── manager/            # Manager frontend/API
│   └── provision/          # Account provisioning
├── internal/               # Private packages
│   ├── agent/              # Core agent logic
│   ├── config/             # Configuration loading
│   ├── cost/               # Cost tracking
│   ├── manager/            # Manager service
│   └── otel/               # OpenTelemetry integration
├── pkg/                    # Public packages
│   └── ircclient/          # IRC client library
├── deploy/                 # Deployment configs
│   └── irc-server/         # Docker-based IRC server
├── test/                   # Integration tests
│   └── integration/
└── examples/               # Example usage
```

## Code Review Process

- All changes require at least one review before merging.
- CI must pass (tests, linting, vet).
- Reviewers will check for correctness, test coverage, and adherence to the guidelines above.

## Security

If you discover a security vulnerability, **do not** open a public issue. Instead, please see [SECURITY.md](SECURITY.md) for responsible disclosure instructions.

## License

By contributing to Agent Chat, you agree that your contributions will be licensed under the [MIT License](LICENSE).

## Questions?

Open an issue with your question or start a discussion on the repository. We're happy to help!
