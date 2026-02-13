#!/bin/bash
# Generate TLS certificates for IRC server

set -e

SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"
TLS_DIR="$SCRIPT_DIR/tls"

echo "========================================"
echo "IRC Server TLS Certificate Generator"
echo "========================================"
echo ""

# Create TLS directory
mkdir -p "$TLS_DIR"

# Check if certificates already exist
if [ -f "$TLS_DIR/server.crt" ] && [ -f "$TLS_DIR/server.key" ]; then
    echo "⚠️  Certificates already exist in $TLS_DIR"
    read -p "Do you want to overwrite them? (y/N): " -n 1 -r
    echo
    if [[ ! $REPLY =~ ^[Yy]$ ]]; then
        echo "Aborted."
        exit 0
    fi
    echo ""
fi

# Ask for certificate details
echo "Certificate Configuration"
echo "-------------------------"
read -p "Domain name (e.g., irc.example.com): " DOMAIN
if [ -z "$DOMAIN" ]; then
    DOMAIN="agent-chat.local"
    echo "Using default: $DOMAIN"
fi

read -p "Country Code (e.g., US): " COUNTRY
COUNTRY=${COUNTRY:-US}

read -p "State/Province: " STATE
STATE=${STATE:-State}

read -p "City: " CITY
CITY=${CITY:-City}

read -p "Organization: " ORG
ORG=${ORG:-AgentChat}

read -p "Validity in days (default: 365): " DAYS
DAYS=${DAYS:-365}

echo ""
echo "Generating certificates..."
echo ""

# Generate self-signed certificate
openssl req -x509 \
    -newkey rsa:4096 \
    -keyout "$TLS_DIR/server.key" \
    -out "$TLS_DIR/server.crt" \
    -days "$DAYS" \
    -nodes \
    -subj "/C=$COUNTRY/ST=$STATE/L=$CITY/O=$ORG/CN=$DOMAIN"

# Set appropriate permissions
chmod 600 "$TLS_DIR/server.key"
chmod 644 "$TLS_DIR/server.crt"

echo ""
echo "✅ Certificates generated successfully!"
echo ""
echo "Location: $TLS_DIR"
echo "  - Certificate: $TLS_DIR/server.crt"
echo "  - Private Key: $TLS_DIR/server.key"
echo ""
echo "⚠️  IMPORTANT NOTES:"
echo "  1. These are SELF-SIGNED certificates for testing only"
echo "  2. For production, use Let's Encrypt or a trusted CA"
echo "  3. The private key is sensitive - never commit it to git"
echo "  4. Certificates expire in $DAYS days"
echo ""

# Show certificate info
echo "Certificate Details:"
echo "--------------------"
openssl x509 -in "$TLS_DIR/server.crt" -noout -subject -dates
echo ""

# Check if docker-compose is available
if command -v docker-compose &> /dev/null; then
    echo "✅ docker-compose found"
    echo ""
    echo "Next steps:"
    echo "  1. Review/edit ircd.yaml if needed"
    echo "  2. Generate admin password: docker run --rm ergochat/ergo:stable genpasswd"
    echo "  3. Update admin password in ircd.yaml"
    echo "  4. Start server: docker-compose up -d"
    echo ""
else
    echo "⚠️  docker-compose not found"
    echo "   Install docker-compose to run the IRC server"
    echo ""
fi

echo "For production deployment with Let's Encrypt:"
echo "  See docs/irc-server-setup.md for instructions"
echo ""
