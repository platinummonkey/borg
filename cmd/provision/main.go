package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/platinummonkey/agent-chat/pkg/ircclient"
)

// Set via ldflags at build time.
var (
	version   = "dev"
	commit    = "unknown"
	buildTime = "unknown"
)

type provisionConfig struct {
	Server      string
	Username    string
	Password    string
	OperName    string
	OperPass    string
	TLSInsecure bool
}

func main() {
	if len(os.Args) > 1 && os.Args[1] == "--version" {
		fmt.Printf("agent-provision %s (commit: %s, built: %s)\n", version, commit, buildTime)
		os.Exit(0)
	}

	server := flag.String("server", envOrDefault("PROVISION_SERVER", ""), "IRC server host:port")
	username := flag.String("username", envOrDefault("PROVISION_USERNAME", ""), "Admin SASL username")
	password := flag.String("password", envOrDefault("PROVISION_PASSWORD", ""), "Admin SASL password")
	operName := flag.String("oper-name", envOrDefault("PROVISION_OPER_NAME", "admin"), "Oper name")
	operPass := flag.String("oper-pass", envOrDefault("PROVISION_OPER_PASS", ""), "Oper password")
	tlsInsecure := flag.Bool("tls-insecure", false, "Skip TLS certificate verification")

	flag.Usage = printUsage
	flag.Parse()

	args := flag.Args()
	if len(args) == 0 {
		printUsage()
		os.Exit(1)
	}

	if *server == "" {
		fatal("--server is required (or set PROVISION_SERVER)")
	}
	if *username == "" {
		fatal("--username is required (or set PROVISION_USERNAME)")
	}
	if *password == "" {
		fatal("--password is required (or set PROVISION_PASSWORD)")
	}
	if *operPass == "" {
		fatal("--oper-pass is required (or set PROVISION_OPER_PASS)")
	}

	cfg := &provisionConfig{
		Server:      *server,
		Username:    *username,
		Password:    *password,
		OperName:    *operName,
		OperPass:    *operPass,
		TLSInsecure: *tlsInsecure,
	}

	cmd := args[0]
	cmdArgs := args[1:]

	var err error
	switch cmd {
	case "create":
		err = runCreate(cfg, cmdArgs)
	case "list":
		err = runList(cfg)
	case "info":
		err = runInfo(cfg, cmdArgs)
	case "delete":
		err = runDelete(cfg, cmdArgs)
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n", cmd)
		printUsage()
		os.Exit(1)
	}

	if err != nil {
		fatal(err.Error())
	}
}

func printUsage() {
	fmt.Fprintf(os.Stderr, `Usage: agent-provision [flags] <command> [command-flags]

Commands:
  create    Register a new account
  list      List all registered accounts
  info      Show account details
  delete    Unregister an account

Global flags:
`)
	flag.PrintDefaults()
	fmt.Fprintf(os.Stderr, `
Command flags:
  create:
    --nick               Account nickname to register
    --account-password   Password for the new account

  info:
    --nick               Account nickname to look up

  delete:
    --nick               Account nickname to remove
`)
}

func envOrDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func fatal(msg string) {
	fmt.Fprintf(os.Stderr, "error: %s\n", msg)
	os.Exit(1)
}

// provisioner manages an IRC connection for account operations.
type provisioner struct {
	client  ircclient.Client
	replies chan string
}

func newProvisioner(cfg *provisionConfig) (*provisioner, error) {
	ircCfg := ircclient.DefaultConfig()
	ircCfg.Server = cfg.Server
	ircCfg.Nick = cfg.Username
	ircCfg.Username = cfg.Username
	ircCfg.Password = cfg.Password
	ircCfg.TLSInsecureSkipVerify = cfg.TLSInsecure
	ircCfg.Reconnect = false
	ircCfg.Channels = nil

	client, err := ircclient.NewClient(ircCfg)
	if err != nil {
		return nil, fmt.Errorf("create IRC client: %w", err)
	}

	p := &provisioner{
		client:  client,
		replies: make(chan string, 100),
	}

	client.OnMessage(func(ev ircclient.MessageEvent) {
		if strings.EqualFold(ev.Nick, "NickServ") {
			p.replies <- ev.Message
		}
	})

	return p, nil
}

// connectAndOper connects to the IRC server and gains oper privileges.
func connectAndOper(cfg *provisionConfig) (*provisioner, error) {
	p, err := newProvisioner(cfg)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := p.client.Connect(ctx); err != nil {
		return nil, fmt.Errorf("connect to %s: %w", cfg.Server, err)
	}

	// Gain oper privileges. The colon prefix ensures passwords with spaces
	// are treated as a single IRC parameter.
	p.client.SendRaw(fmt.Sprintf("OPER %s :%s", cfg.OperName, cfg.OperPass))
	time.Sleep(time.Second)

	return p, nil
}

// collectReplies reads NickServ replies until no new messages arrive within the idle timeout.
func (p *provisioner) collectReplies(idle time.Duration) []string {
	var lines []string
	timer := time.NewTimer(idle)
	defer timer.Stop()
	for {
		select {
		case msg := <-p.replies:
			lines = append(lines, msg)
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			timer.Reset(idle)
		case <-timer.C:
			return lines
		}
	}
}

// execNickServ sends a NickServ command and prints the response.
func (p *provisioner) execNickServ(command string) {
	p.client.SendMessage("NickServ", command)
	replies := p.collectReplies(2 * time.Second)
	if len(replies) == 0 {
		fmt.Println("No response from NickServ (command may have succeeded — verify with 'info')")
		return
	}
	for _, line := range replies {
		fmt.Println(line)
	}
}

func runCreate(cfg *provisionConfig, args []string) error {
	fs := flag.NewFlagSet("create", flag.ExitOnError)
	nick := fs.String("nick", "", "Account nickname to register")
	accountPass := fs.String("account-password", "", "Password for the new account")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *nick == "" {
		return fmt.Errorf("--nick is required for create")
	}
	if *accountPass == "" {
		return fmt.Errorf("--account-password is required for create")
	}

	p, err := connectAndOper(cfg)
	if err != nil {
		return err
	}
	defer p.client.Disconnect()

	p.execNickServ(fmt.Sprintf("SAREGISTER %s * %s", *nick, *accountPass))
	return nil
}

func runList(cfg *provisionConfig) error {
	p, err := connectAndOper(cfg)
	if err != nil {
		return err
	}
	defer p.client.Disconnect()

	p.execNickServ("LIST *")
	return nil
}

func runInfo(cfg *provisionConfig, args []string) error {
	fs := flag.NewFlagSet("info", flag.ExitOnError)
	nick := fs.String("nick", "", "Account nickname to look up")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *nick == "" {
		return fmt.Errorf("--nick is required for info")
	}

	p, err := connectAndOper(cfg)
	if err != nil {
		return err
	}
	defer p.client.Disconnect()

	p.execNickServ(fmt.Sprintf("INFO %s", *nick))
	return nil
}

func runDelete(cfg *provisionConfig, args []string) error {
	fs := flag.NewFlagSet("delete", flag.ExitOnError)
	nick := fs.String("nick", "", "Account nickname to remove")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *nick == "" {
		return fmt.Errorf("--nick is required for delete")
	}

	p, err := connectAndOper(cfg)
	if err != nil {
		return err
	}
	defer p.client.Disconnect()

	p.execNickServ(fmt.Sprintf("SADROP %s", *nick))
	return nil
}
