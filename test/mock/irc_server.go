package mock

import (
	"bufio"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"strings"
	"sync"
	"time"
)

// IRCServer is a minimal mock IRC server for unit testing.
// It supports TLS, SASL PLAIN authentication, and basic IRC commands.
type IRCServer struct {
	listener  net.Listener
	addr      string
	tlsConfig *tls.Config

	mu      sync.Mutex
	clients []*mockClient
	closed  bool

	// Accounts maps username to password for SASL auth.
	Accounts map[string]string
}

type mockClient struct {
	conn     net.Conn
	nick     string
	user     string
	server   *IRCServer
	welcomed bool
	scanner  *bufio.Scanner
}

// NewIRCServer creates and starts a mock IRC server with a self-signed TLS cert.
func NewIRCServer() (*IRCServer, error) {
	tlsCfg, err := generateSelfSignedTLS()
	if err != nil {
		return nil, fmt.Errorf("generate TLS config: %w", err)
	}

	listener, err := tls.Listen("tcp", "127.0.0.1:0", tlsCfg)
	if err != nil {
		return nil, fmt.Errorf("listen: %w", err)
	}

	s := &IRCServer{
		listener:  listener,
		addr:      listener.Addr().String(),
		tlsConfig: tlsCfg,
		Accounts:  map[string]string{"testuser": "testpass"},
	}

	go s.acceptLoop()

	return s, nil
}

// Addr returns the server's listen address (host:port).
func (s *IRCServer) Addr() string {
	return s.addr
}

// Close shuts down the mock server.
func (s *IRCServer) Close() {
	s.mu.Lock()
	s.closed = true
	s.mu.Unlock()
	s.listener.Close()
	s.mu.Lock()
	for _, c := range s.clients {
		c.conn.Close()
	}
	s.mu.Unlock()
}

func (s *IRCServer) acceptLoop() {
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			s.mu.Lock()
			closed := s.closed
			s.mu.Unlock()
			if closed {
				return
			}
			continue
		}

		mc := &mockClient{
			conn:    conn,
			server:  s,
			scanner: bufio.NewScanner(conn),
		}

		s.mu.Lock()
		s.clients = append(s.clients, mc)
		s.mu.Unlock()

		go mc.handleConnection()
	}
}

func (mc *mockClient) handleConnection() {
	defer mc.conn.Close()

	saslAuthenticated := false
	capNegotiating := false

	for mc.scanner.Scan() {
		line := mc.scanner.Text()
		parts := strings.Fields(line)
		if len(parts) == 0 {
			continue
		}

		cmd := strings.ToUpper(parts[0])

		switch cmd {
		case "CAP":
			if len(parts) >= 2 {
				subCmd := strings.ToUpper(parts[1])
				switch subCmd {
				case "LS":
					capNegotiating = true
					mc.send(":server CAP * LS :sasl")
				case "REQ":
					mc.send(":server CAP * ACK :sasl")
				case "END":
					capNegotiating = false
					if mc.nick != "" && mc.user != "" && saslAuthenticated {
						mc.sendWelcome()
					}
				}
			}

		case "AUTHENTICATE":
			if len(parts) >= 2 {
				if parts[1] == "PLAIN" {
					mc.send("AUTHENTICATE +")
				} else {
					if mc.server.validateSASL(parts[1]) {
						saslAuthenticated = true
						mc.send(":server 903 * :SASL authentication successful")
					} else {
						mc.send(":server 904 * :SASL authentication failed")
					}
				}
			}

		case "NICK":
			if len(parts) >= 2 {
				mc.nick = parts[1]
				if !capNegotiating && mc.user != "" && saslAuthenticated {
					mc.sendWelcome()
				}
			}

		case "USER":
			if len(parts) >= 2 {
				mc.user = parts[1]
				if !capNegotiating && mc.nick != "" && saslAuthenticated {
					mc.sendWelcome()
				}
			}

		case "JOIN":
			if len(parts) >= 2 {
				channel := parts[1]
				mc.send(fmt.Sprintf(":%s!%s@localhost JOIN %s", mc.nick, mc.user, channel))
				mc.send(fmt.Sprintf(":server 353 %s = %s :%s", mc.nick, channel, mc.nick))
				mc.send(fmt.Sprintf(":server 366 %s %s :End of /NAMES list", mc.nick, channel))
			}

		case "PART":
			if len(parts) >= 2 {
				channel := parts[1]
				mc.send(fmt.Sprintf(":%s!%s@localhost PART %s", mc.nick, mc.user, channel))
			}

		case "PRIVMSG":
			if len(parts) >= 3 {
				target := parts[1]
				msg := strings.Join(parts[2:], " ")
				if strings.HasPrefix(msg, ":") {
					msg = msg[1:]
				}
				mc.send(fmt.Sprintf(":%s!%s@localhost PRIVMSG %s :%s", mc.nick, mc.user, target, msg))
			}

		case "PING":
			token := ""
			if len(parts) >= 2 {
				token = parts[1]
			}
			mc.send(fmt.Sprintf("PONG server %s", token))

		case "QUIT":
			mc.send(fmt.Sprintf("ERROR :Closing link: %s", mc.nick))
			return
		}
	}
}

func (mc *mockClient) sendWelcome() {
	if mc.welcomed {
		return
	}
	mc.welcomed = true
	mc.send(fmt.Sprintf(":server 001 %s :Welcome to the Mock IRC Network %s", mc.nick, mc.nick))
	mc.send(fmt.Sprintf(":server 002 %s :Your host is server, running mock-ircd", mc.nick))
	mc.send(fmt.Sprintf(":server 003 %s :This server was created now", mc.nick))
	mc.send(fmt.Sprintf(":server 004 %s server mock-ircd o o", mc.nick))
	mc.send(fmt.Sprintf(":server 376 %s :End of /MOTD command.", mc.nick))
}

func (mc *mockClient) send(msg string) {
	mc.conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
	mc.conn.Write([]byte(msg + "\r\n"))
}

func (s *IRCServer) validateSASL(encoded string) bool {
	decoded, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return false
	}

	// SASL PLAIN format: authzid\0authcid\0password
	parts := strings.SplitN(string(decoded), "\x00", 3)
	if len(parts) != 3 {
		return false
	}

	username := parts[1]
	password := parts[2]

	expected, ok := s.Accounts[username]
	return ok && expected == password
}

func generateSelfSignedTLS() (*tls.Config, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, err
	}

	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "localhost"},
		NotBefore:    time.Now(),
		NotAfter:     time.Now().Add(24 * time.Hour),
		DNSNames:     []string{"localhost"},
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
	}

	certDER, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		return nil, err
	}

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return nil, err
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})

	cert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		return nil, err
	}

	return &tls.Config{
		Certificates: []tls.Certificate{cert},
	}, nil
}
