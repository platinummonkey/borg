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
// PRIVMSG messages are broadcast to all clients in the same channel.
type IRCServer struct {
	listener  net.Listener
	addr      string
	tlsConfig *tls.Config

	mu      sync.Mutex
	clients []*mockClient
	closed  bool

	// channels tracks channel membership: channel name -> set of mockClient pointers.
	channels map[string]map[*mockClient]struct{}

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

	mu       sync.Mutex
	joinedCh map[string]struct{}
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
		channels:  make(map[string]map[*mockClient]struct{}),
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
	_ = s.listener.Close()
	s.mu.Lock()
	for _, c := range s.clients {
		_ = c.conn.Close()
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
			conn:     conn,
			server:   s,
			scanner:  bufio.NewScanner(conn),
			joinedCh: make(map[string]struct{}),
		}

		s.mu.Lock()
		s.clients = append(s.clients, mc)
		s.mu.Unlock()

		go mc.handleConnection()
	}
}

// joinChannel adds a client to a channel's membership list.
func (s *IRCServer) joinChannel(channel string, mc *mockClient) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.channels[channel] == nil {
		s.channels[channel] = make(map[*mockClient]struct{})
	}
	s.channels[channel][mc] = struct{}{}
}

// partChannel removes a client from a channel's membership list.
func (s *IRCServer) partChannel(channel string, mc *mockClient) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if members, ok := s.channels[channel]; ok {
		delete(members, mc)
		if len(members) == 0 {
			delete(s.channels, channel)
		}
	}
}

// removeClient removes a client from all channels.
func (s *IRCServer) removeClient(mc *mockClient) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for ch, members := range s.channels {
		delete(members, mc)
		if len(members) == 0 {
			delete(s.channels, ch)
		}
	}
}

// broadcastToChannel sends a message to all clients in a channel except the sender.
func (s *IRCServer) broadcastToChannel(channel, msg string, sender *mockClient) {
	s.mu.Lock()
	members := make([]*mockClient, 0)
	if ch, ok := s.channels[channel]; ok {
		for mc := range ch {
			if mc != sender {
				members = append(members, mc)
			}
		}
	}
	s.mu.Unlock()

	for _, mc := range members {
		mc.send(msg)
	}
}

func (mc *mockClient) handleConnection() {
	defer func() {
		mc.server.removeClient(mc)
		_ = mc.conn.Close()
	}()

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
				mc.server.joinChannel(channel, mc)
				mc.mu.Lock()
				mc.joinedCh[channel] = struct{}{}
				mc.mu.Unlock()

				joinMsg := fmt.Sprintf(":%s!%s@localhost JOIN %s", mc.nick, mc.user, channel)
				// Send to joiner and broadcast to channel.
				mc.send(joinMsg)
				mc.server.broadcastToChannel(channel, joinMsg, mc)
				mc.send(fmt.Sprintf(":server 353 %s = %s :%s", mc.nick, channel, mc.nick))
				mc.send(fmt.Sprintf(":server 366 %s %s :End of /NAMES list", mc.nick, channel))
			}

		case "PART":
			if len(parts) >= 2 {
				channel := parts[1]
				partMsg := fmt.Sprintf(":%s!%s@localhost PART %s", mc.nick, mc.user, channel)
				mc.send(partMsg)
				mc.server.broadcastToChannel(channel, partMsg, mc)
				mc.server.partChannel(channel, mc)
				mc.mu.Lock()
				delete(mc.joinedCh, channel)
				mc.mu.Unlock()
			}

		case "PRIVMSG":
			if len(parts) >= 3 {
				target := parts[1]
				msg := strings.TrimPrefix(strings.Join(parts[2:], " "), ":")
				fullMsg := fmt.Sprintf(":%s!%s@localhost PRIVMSG %s :%s", mc.nick, mc.user, target, msg)

				if strings.HasPrefix(target, "#") {
					// Channel message: echo to sender and broadcast to other members.
					mc.send(fullMsg)
					mc.server.broadcastToChannel(target, fullMsg, mc)
				} else {
					// Private message: send to the target nick.
					mc.server.sendToNick(target, fullMsg)
				}
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

// sendToNick sends a message to a specific nick (for private messages).
func (s *IRCServer) sendToNick(nick, msg string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, c := range s.clients {
		if c.nick == nick {
			c.send(msg)
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
	_ = mc.conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
	_, _ = mc.conn.Write([]byte(msg + "\r\n"))
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
