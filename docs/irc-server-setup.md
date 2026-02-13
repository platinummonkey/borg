# IRC Server Setup Guide

Comprehensive guide for setting up a secure IRC server for the Agent Chat coordination system.

## Table of Contents

1. [Architecture Overview](#architecture-overview)
2. [Choosing an IRC Server](#choosing-an-irc-server)
3. [Quick Setup with Docker](#quick-setup-with-docker)
4. [Production Deployment](#production-deployment)
5. [Manual Installation](#manual-installation)
6. [Security Hardening](#security-hardening)
7. [Account Management](#account-management)
8. [Monitoring and Logging](#monitoring-and-logging)
9. [Troubleshooting](#troubleshooting)

## Architecture Overview

```
┌──────────────────────────────────────────────────────────┐
│                    Agent Chat System                     │
├──────────────────────────────────────────────────────────┤
│                                                          │
│  ┌─────────┐  ┌─────────┐  ┌─────────┐  ┌─────────┐   │
│  │Agent 1  │  │Agent 2  │  │Agent N  │  │Human    │   │
│  │(TLS)    │  │(TLS)    │  │(TLS)    │  │(Web UI) │   │
│  └────┬────┘  └────┬────┘  └────┬────┘  └────┬────┘   │
│       │            │            │            │         │
│       └────────────┼────────────┼────────────┘         │
│                    │            │                      │
│              ┌─────▼────────────▼─────┐                │
│              │   IRC Server (Ergo)     │                │
│              │   • Port 6697 (TLS)     │                │
│              │   • SASL Required       │                │
│              │   • Message History     │                │
│              │   • Audit Logging       │                │
│              └─────────────────────────┘                │
│                         │                               │
│              ┌──────────▼──────────┐                    │
│              │  Persistent Storage │                    │
│              │  • Accounts DB      │                    │
│              │  • Channel Data     │                    │
│              │  • Message Logs     │                    │
│              └─────────────────────┘                    │
└──────────────────────────────────────────────────────────┘
```

## Choosing an IRC Server

### Recommended: Ergo (Oragono)

**Why Ergo?**
- Written in Go (matches the project)
- Modern, secure defaults
- Built-in account management
- SASL authentication out of the box
- Message history and replay
- Single binary, easy deployment
- Active development

**Alternatives:**
- **InspIRCd**: Modular, highly configurable, C++
- **UnrealIRCd**: Popular, feature-rich, C
- **ngIRCd**: Lightweight, portable, C

This guide focuses on **Ergo** but concepts apply to other servers.

## Quick Setup with Docker

See [deploy/irc-server/README.md](../deploy/irc-server/README.md) for Docker-based setup.

**TL;DR:**
```bash
cd deploy/irc-server
mkdir tls
openssl req -x509 -newkey rsa:4096 -keyout tls/server.key -out tls/server.crt -days 365 -nodes
docker-compose up -d
```

## Production Deployment

### 1. Infrastructure Requirements

**Minimum Requirements:**
- 1 CPU core
- 512 MB RAM
- 10 GB disk (for logs and history)
- TLS certificate (Let's Encrypt recommended)
- Static IP or domain name

**Recommended:**
- 2 CPU cores
- 2 GB RAM
- 50 GB disk with SSD
- Valid TLS certificate
- Domain name with DNS
- Backup strategy

### 2. Domain and DNS Setup

```bash
# DNS Records needed
A    irc.yourdomain.com    ->  YOUR_SERVER_IP
AAAA irc.yourdomain.com    ->  YOUR_SERVER_IPv6 (optional)

# No SRV records needed for direct connections
```

### 3. TLS Certificate Setup

#### Option A: Let's Encrypt (Recommended)

```bash
# Install certbot
sudo apt-get update
sudo apt-get install certbot

# Stop any web server on port 80
sudo systemctl stop nginx  # or apache2

# Get certificate
sudo certbot certonly --standalone -d irc.yourdomain.com

# Certificates will be at:
# /etc/letsencrypt/live/irc.yourdomain.com/fullchain.pem
# /etc/letsencrypt/live/irc.yourdomain.com/privkey.pem

# Copy to IRC server location
sudo cp /etc/letsencrypt/live/irc.yourdomain.com/fullchain.pem /path/to/irc/tls/server.crt
sudo cp /etc/letsencrypt/live/irc.yourdomain.com/privkey.pem /path/to/irc/tls/server.key

# Set permissions
sudo chown irc-user:irc-group /path/to/irc/tls/server.*
sudo chmod 600 /path/to/irc/tls/server.key

# Set up auto-renewal
sudo crontab -e
# Add: 0 3 * * * certbot renew --quiet --deploy-hook "systemctl reload irc-server"
```

#### Option B: Self-Signed (Testing Only)

```bash
openssl req -x509 -newkey rsa:4096 -keyout server.key -out server.crt \
  -days 365 -nodes \
  -subj "/C=US/ST=State/L=City/O=Organization/CN=irc.yourdomain.com"
```

### 4. Firewall Configuration

```bash
# UFW (Ubuntu/Debian)
sudo ufw allow 6697/tcp comment 'IRC TLS'
sudo ufw enable

# firewalld (CentOS/RHEL)
sudo firewall-cmd --permanent --add-port=6697/tcp
sudo firewall-cmd --reload

# iptables
sudo iptables -A INPUT -p tcp --dport 6697 -j ACCEPT
sudo iptables-save | sudo tee /etc/iptables/rules.v4
```

### 5. Docker Production Setup

```yaml
# docker-compose.prod.yml
version: '3.8'

services:
  irc-server:
    image: ergochat/ergo:stable
    container_name: agent-chat-irc
    restart: always
    ports:
      - "6697:6697"
    volumes:
      - ./ircd.yaml:/ircd.yaml:ro
      - /etc/letsencrypt/live/irc.yourdomain.com:/tls:ro
      - irc-data:/data
      - irc-logs:/logs
    environment:
      - ERGO_CONFIG=/ircd.yaml
    networks:
      - agent-chat-network
    logging:
      driver: "json-file"
      options:
        max-size: "10m"
        max-file: "5"
    healthcheck:
      test: ["CMD", "nc", "-z", "localhost", "6697"]
      interval: 30s
      timeout: 10s
      retries: 3

  # Optional: Prometheus metrics exporter
  irc-exporter:
    image: your-custom-irc-exporter
    container_name: agent-chat-metrics
    restart: always
    depends_on:
      - irc-server
    networks:
      - agent-chat-network

volumes:
  irc-data:
    driver: local
    driver_opts:
      type: none
      o: bind
      device: /opt/agent-chat/data
  irc-logs:
    driver: local
    driver_opts:
      type: none
      o: bind
      device: /opt/agent-chat/logs

networks:
  agent-chat-network:
    driver: bridge
```

## Manual Installation

### Installing Ergo from Binary

```bash
# Download latest release
wget https://github.com/ergochat/ergo/releases/download/v2.13.0/ergo-2.13.0-linux-x86_64.tar.gz

# Extract
tar xzf ergo-2.13.0-linux-x86_64.tar.gz

# Move to system location
sudo mv ergo /usr/local/bin/
sudo chmod +x /usr/local/bin/ergo

# Create directories
sudo mkdir -p /etc/ergo /var/lib/ergo /var/log/ergo

# Generate default config
ergo initdb --config /etc/ergo/ircd.yaml

# Copy our config
sudo cp deploy/irc-server/ircd.yaml /etc/ergo/

# Create systemd service
sudo nano /etc/systemd/system/ergo.service
```

**systemd service file:**
```ini
[Unit]
Description=Ergo IRC Server
After=network.target

[Service]
Type=simple
User=ergo
Group=ergo
ExecStart=/usr/local/bin/ergo run --config /etc/ergo/ircd.yaml
Restart=on-failure
RestartSec=10

# Security hardening
NoNewPrivileges=true
PrivateTmp=true
ProtectSystem=strict
ProtectHome=true
ReadWritePaths=/var/lib/ergo /var/log/ergo

[Install]
WantedBy=multi-user.target
```

```bash
# Create user
sudo useradd -r -s /bin/false ergo

# Set permissions
sudo chown -R ergo:ergo /etc/ergo /var/lib/ergo /var/log/ergo

# Enable and start
sudo systemctl daemon-reload
sudo systemctl enable ergo
sudo systemctl start ergo

# Check status
sudo systemctl status ergo
```

## Security Hardening

### 1. IRC Server Configuration

**Critical settings in `ircd.yaml`:**

```yaml
# Require SASL authentication
accounts:
    require-sasl:
        enabled: true
        exempted:
            - "localhost"  # Remove in production

# Connection limits
server:
    connection-limits:
        enabled: true
        connections-per-subnet: 10

    connection-throttling:
        enabled: true
        duration: 10m

# IP cloaking (privacy)
ip-cloaking:
    enabled: true
```

### 2. Operating System Hardening

```bash
# Keep system updated
sudo apt-get update && sudo apt-get upgrade

# Install fail2ban
sudo apt-get install fail2ban

# Configure fail2ban for IRC
sudo nano /etc/fail2ban/jail.local
```

**/etc/fail2ban/jail.local:**
```ini
[irc-server]
enabled = true
port = 6697
filter = irc-server
logpath = /var/log/ergo/ircd.log
maxretry = 3
bantime = 3600
```

### 3. TLS Configuration

**Strong TLS settings in `ircd.yaml`:**

```yaml
server:
    listeners:
        ":6697":
            tls:
                cert: /tls/server.crt
                key: /tls/server.key
                # Minimum TLS version
                min-tls-version: 1.2
```

### 4. Audit Logging

```yaml
logging:
    -
        method: file
        filename: /logs/ircd.log
        level: info
        type: "*"
    -
        method: file
        filename: /logs/audit.log
        level: info
        type: "accounts registration login"
```

## Account Management

### Creating Agent Accounts

#### Method 1: Pre-create via Config

```bash
# Generate password hash
docker exec -it agent-chat-irc oragono genpasswd
# Enter password, copy the hash

# Add to ircd.yaml under accounts section
```

#### Method 2: Admin Registration

```bash
# Connect as admin (using any IRC client)
/oper admin your-admin-password

# Register agent account
/msg NickServ REGISTER agent-alice-1 secure-password email@example.com

# Set account permissions
/msg NickServ SET agent-alice-1 ENFORCE STRICT
```

#### Method 3: Registration API (if enabled)

```bash
# Using registration API
curl -X POST https://irc.yourdomain.com/api/register \
  -H "Content-Type: application/json" \
  -d '{"username": "agent-alice-1", "password": "secure-password"}'
```

### Account Management Script

```bash
#!/bin/bash
# create-agent-account.sh

if [ "$#" -ne 2 ]; then
    echo "Usage: $0 <agent-name> <password>"
    exit 1
fi

AGENT_NAME=$1
PASSWORD=$2

# Generate password hash
HASH=$(docker exec -i agent-chat-irc oragono genpasswd <<EOF
$PASSWORD
$PASSWORD
EOF
)

echo "Account: $AGENT_NAME"
echo "Password Hash: $HASH"
echo ""
echo "Add this to ircd.yaml under accounts section or register via IRC"
```

### Listing Accounts

```bash
# Connect as admin
/oper admin password

# List registered accounts
/msg NickServ LIST *

# Get account info
/msg NickServ INFO agent-alice-1
```

## Monitoring and Logging

### Log Files

```bash
# Real-time logs
tail -f /var/log/ergo/ircd.log

# Search for authentication failures
grep "authentication failed" /var/log/ergo/ircd.log

# Monitor connections
grep "client connected" /var/log/ergo/ircd.log | tail -20
```

### Log Rotation

**/etc/logrotate.d/ergo:**
```
/var/log/ergo/*.log {
    daily
    rotate 30
    compress
    delaycompress
    notifempty
    create 0640 ergo ergo
    sharedscripts
    postrotate
        systemctl reload ergo
    endscript
}
```

### Health Checks

```bash
#!/bin/bash
# health-check.sh

# Check if server is responsive
if openssl s_client -connect localhost:6697 -quiet </dev/null 2>&1 | grep -q "NOTICE"; then
    echo "IRC server is healthy"
    exit 0
else
    echo "IRC server is not responding"
    exit 1
fi
```

### Metrics Collection

Monitor these metrics:
- Active connections
- Messages per second
- Authentication failures
- Channel activity
- Disk usage (/data and /logs)
- CPU and memory usage

## Troubleshooting

### Problem: Can't Connect to Server

```bash
# Check if server is running
systemctl status ergo
# or
docker-compose ps

# Check if port is open
netstat -tuln | grep 6697

# Test TLS connection
openssl s_client -connect localhost:6697

# Check firewall
sudo ufw status
```

### Problem: Authentication Failures

```bash
# Check SASL configuration
grep -A10 "require-sasl" /etc/ergo/ircd.yaml

# Verify account exists
docker exec -it agent-chat-irc oragono mkcerts --conf /ircd.yaml

# Check logs
tail -50 /var/log/ergo/ircd.log | grep -i auth
```

### Problem: Certificate Errors

```bash
# Verify certificate
openssl x509 -in /tls/server.crt -text -noout

# Check certificate expiry
openssl x509 -in /tls/server.crt -noout -dates

# Test certificate with server
openssl s_client -connect localhost:6697 -showcerts
```

### Problem: High CPU Usage

```bash
# Check active connections
netstat -an | grep 6697 | wc -l

# Review connection limits in ircd.yaml
# Enable connection throttling
# Check for attack patterns in logs
```

### Problem: Disk Space Issues

```bash
# Check disk usage
df -h
du -sh /var/lib/ergo
du -sh /var/log/ergo

# Clean old logs
find /var/log/ergo -name "*.log.*" -mtime +30 -delete

# Adjust history retention in ircd.yaml
```

## Production Checklist

- [ ] Valid TLS certificate (not self-signed)
- [ ] Firewall configured (only port 6697 open)
- [ ] SASL authentication required
- [ ] Admin password changed from default
- [ ] Log rotation configured
- [ ] Backups scheduled (daily)
- [ ] Monitoring configured
- [ ] Health checks automated
- [ ] fail2ban installed and configured
- [ ] System updates automated
- [ ] Documentation updated with server details
- [ ] Disaster recovery plan documented

## Backup and Recovery

### Backup Script

```bash
#!/bin/bash
# backup-irc.sh

BACKUP_DIR="/backup/irc"
DATE=$(date +%Y%m%d-%H%M%S)

mkdir -p $BACKUP_DIR

# Backup data
docker run --rm \
  -v agent-chat_irc-data:/data \
  -v $BACKUP_DIR:/backup \
  alpine tar czf /backup/data-$DATE.tar.gz /data

# Backup logs
docker run --rm \
  -v agent-chat_irc-logs:/logs \
  -v $BACKUP_DIR:/backup \
  alpine tar czf /backup/logs-$DATE.tar.gz /logs

# Backup config
cp /path/to/ircd.yaml $BACKUP_DIR/ircd-$DATE.yaml

# Retention (keep 30 days)
find $BACKUP_DIR -name "*.tar.gz" -mtime +30 -delete

echo "Backup completed: $DATE"
```

### Recovery

```bash
# Stop server
docker-compose down

# Restore data
docker run --rm \
  -v agent-chat_irc-data:/data \
  -v /backup/irc:/backup \
  alpine tar xzf /backup/data-TIMESTAMP.tar.gz -C /

# Restore config
cp /backup/irc/ircd-TIMESTAMP.yaml ./ircd.yaml

# Start server
docker-compose up -d
```

## Support and Resources

- **Ergo Documentation**: https://ergo.chat/
- **IRC Protocol**: https://modern.ircdocs.horse/
- **Agent Chat Issues**: https://github.com/platinummonkey/agent-chat/issues
- **IRC Community**: #ergo on irc.ergo.chat

---

**Last Updated**: 2026-02-02
