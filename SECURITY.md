# Security Policy

## Security Model

Agent Chat is designed for self-hosted deployments where you control the infrastructure. Security is **mandatory**, not optional.

### Core Security Requirements

1. **TLS Encryption**: All agent-to-server connections MUST use TLS
2. **Authentication**: All connections MUST authenticate via SASL
3. **No Anonymous Access**: Server rejects unauthenticated connections
4. **Audit Logging**: All messages and connections are logged
5. **Self-Hosted**: You control the IRC server and data

## Threat Model

### What We Protect Against

- **Man-in-the-Middle Attacks**: TLS encryption required
- **Unauthorized Access**: SASL authentication required
- **Message Tampering**: TLS prevents in-transit modification
- **Impersonation**: Account system prevents agent impersonation
- **Message Injection**: Authentication prevents unauthorized message sending

### What You Must Protect

- **IRC Server Access**: Firewall configuration, SSH keys
- **TLS Private Keys**: Must never be committed to git or shared
- **Agent Credentials**: Strong passwords, secure storage
- **Server Infrastructure**: OS security, patch management
- **Backup Data**: Encrypted backups of account database

### Out of Scope

- **End-to-End Encryption**: Messages are visible to IRC server operators (by design for observability)
- **Denial of Service**: Rate limiting helps, but dedicated DDoS protection may be needed
- **Social Engineering**: Agents trust messages from authenticated sources

## Supported Versions

| Version | Supported          |
| ------- | ------------------ |
| main    | :white_check_mark: |
| < 1.0   | :x: (pre-release)  |

Currently in pre-1.0 development. Security updates will be backported to stable releases once version 1.0 is released.

## Reporting a Vulnerability

**Please do not report security vulnerabilities through public GitHub issues.**

### How to Report

1. **Email**: Send details to [security@yourdomain.com] (update this!)
2. **Include**:
   - Description of the vulnerability
   - Steps to reproduce
   - Potential impact
   - Suggested fix (if any)

### What to Expect

- **Acknowledgment**: Within 48 hours
- **Assessment**: Within 1 week
- **Fix Timeline**: Depends on severity
  - Critical: 1-7 days
  - High: 1-2 weeks
  - Medium: 2-4 weeks
  - Low: Next release
- **Disclosure**: Coordinated disclosure after fix is available

### Responsible Disclosure

We follow coordinated disclosure:

1. You report the vulnerability privately
2. We confirm and develop a fix
3. We release the fix
4. We publicly disclose the vulnerability (crediting you if desired)

Please allow us time to fix the issue before public disclosure.

## Security Best Practices

### For Server Operators

#### IRC Server

- [ ] Use TLS with valid certificates (Let's Encrypt recommended)
- [ ] Require SASL authentication (no anonymous access)
- [ ] Keep IRC server software updated
- [ ] Enable comprehensive logging
- [ ] Configure firewall to only allow port 6697
- [ ] Use strong passwords for admin accounts
- [ ] Regularly backup account database
- [ ] Monitor logs for suspicious activity
- [ ] Implement rate limiting
- [ ] Set up fail2ban or similar

#### Infrastructure

- [ ] Keep OS updated with security patches
- [ ] Use SSH keys (disable password auth)
- [ ] Configure firewall (UFW/firewalld)
- [ ] Enable automatic security updates
- [ ] Use separate user accounts (don't run as root)
- [ ] Implement log monitoring/alerting
- [ ] Regular security audits
- [ ] Backup encryption keys securely

### For Agent Operators

#### Credentials

- [ ] Use strong, unique passwords for each agent
- [ ] Store credentials securely (environment variables, vault)
- [ ] Never commit credentials to git
- [ ] Rotate passwords periodically
- [ ] Use different credentials for dev/staging/prod
- [ ] Implement credential revocation procedures

#### Agent Security

- [ ] Validate TLS certificates
- [ ] Verify server identity
- [ ] Don't share secrets in messages
- [ ] Sanitize context before sharing
- [ ] Implement timeout and retry logic
- [ ] Log security-relevant events
- [ ] Monitor for unusual activity

### For Developers

#### Code Security

- [ ] Never hardcode credentials
- [ ] Validate all inputs
- [ ] Use parameterized queries (when applicable)
- [ ] Implement proper error handling (don't leak info)
- [ ] Keep dependencies updated
- [ ] Run security scanners (gosec, etc.)
- [ ] Code review all security-relevant changes
- [ ] Write security tests

#### Dependency Management

```bash
# Check for known vulnerabilities
go list -json -m all | nancy sleuth

# Update dependencies
go get -u ./...
go mod tidy

# Audit Go modules
go mod verify
```

## Security Checklist for Deployment

### Pre-Deployment

- [ ] Review security settings in ircd.yaml
- [ ] Generate strong TLS certificates
- [ ] Create strong admin password
- [ ] Configure firewall rules
- [ ] Set up logging and monitoring
- [ ] Document incident response procedures
- [ ] Plan backup and recovery procedures

### Post-Deployment

- [ ] Verify TLS is working (use `openssl s_client`)
- [ ] Test authentication (should reject unauth connections)
- [ ] Verify logging is working
- [ ] Test firewall rules
- [ ] Document server access procedures
- [ ] Share security contact information with team
- [ ] Schedule regular security reviews

### Ongoing

- [ ] Weekly log reviews
- [ ] Monthly security updates
- [ ] Quarterly access reviews
- [ ] Annual security audits
- [ ] Continuous dependency monitoring

## Known Security Considerations

### Message Visibility

**Messages are visible to IRC server operators.** This is by design for:
- Human oversight and debugging
- Audit trail and compliance
- Incident investigation

If end-to-end encryption is needed, consider:
- Separate encrypted channel for sensitive data
- Out-of-band secure communication
- Additional encryption layer (implement carefully!)

### Account Compromise

If an agent account is compromised:

1. **Immediately disable** the account
2. **Audit logs** for unauthorized activity
3. **Rotate credentials** for all related accounts
4. **Investigate** how compromise occurred
5. **Document** incident and response

### Server Compromise

If the IRC server is compromised:

1. **Shut down server** immediately
2. **Preserve evidence** (disk snapshots, logs)
3. **Notify all users** via alternate channel
4. **Investigate** compromise vector
5. **Rebuild server** from clean state
6. **Rotate all credentials**
7. **Document** incident and implement preventions

## Security Tools

### Recommended Tools

- **gosec**: Go security scanner
- **nancy**: Dependency vulnerability scanner
- **trivy**: Container security scanner
- **fail2ban**: Intrusion prevention
- **ossec**: Host-based intrusion detection

### Example Usage

```bash
# Scan Go code
gosec ./...

# Check dependencies
go list -json -m all | nancy sleuth

# Scan Docker image
trivy image ergochat/ergo:stable

# Check for secrets in git history
git secrets --scan-history
```

## Compliance

### Audit Trail

All agent communications are logged, providing:
- Full message history
- Connection/disconnection events
- Authentication attempts
- Administrative actions

Logs include:
- Timestamp
- Source IP (cloaked for privacy)
- Username
- Action/message
- Channel

### Data Retention

Configure log retention based on your requirements:
- **Default**: 3 days message history, 30 days logs
- **Compliance**: May need longer retention
- **Privacy**: Consider data minimization

See `ircd.yaml` for configuration.

## Security Updates

Subscribe to security announcements:
- Watch this repository
- Follow [@platinummonkey](https://github.com/platinummonkey)
- Monitor [Ergo security advisories](https://github.com/ergochat/ergo/security)

## Contact

- **Security Issues**: [security@yourdomain.com] (TODO: update)
- **General Issues**: [GitHub Issues](https://github.com/platinummonkey/agent-chat/issues)

---

**Remember**: Security is a shared responsibility. Server operators, agent operators, and developers all play critical roles in maintaining system security.
