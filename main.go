package main

import (
	"fmt"
	"os"
	"path/filepath"
)

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(2)
	}

	cfgPath := envOr("ENTRYPOOL_CONFIG", defaultConfigPath)
	args := os.Args[1:]
	if args[0] == "-c" || args[0] == "--config" {
		if len(args) < 3 {
			fatalf("usage: entrypool -c /etc/entrypool/config.env <command>")
		}
		cfgPath = args[1]
		args = args[2:]
	}
	if len(args) < 1 {
		printUsage()
		os.Exit(2)
	}
	cmd := args[0]

	switch cmd {
	case "version", "--version", "-v":
		fmt.Printf("entrypool %s (%s)\n", Version, Commit)
		return
	case "help", "-h", "--help":
		printUsage()
		return
	case "init-config":
		if err := initConfig(cfgPath); err != nil {
			fatalf("%v", err)
		}
		return
	}

	cfg, err := loadConfig(cfgPath)
	if err != nil {
		if _, statErr := os.Stat(cfgPath); os.IsNotExist(statErr) {
			fatalf("config not found: %s (run: entrypool init-config)", cfgPath)
		}
		fatalf("config: %v", err)
	}

	switch cmd {
	case "doctor":
		if err := doctor(cfg); err != nil {
			fmt.Fprintf(os.Stderr, "entrypool: %v\n", err)
			os.Exit(1)
		}
	case "run":
		if err := runOnce(cfg); err != nil {
			fatalf("%v", err)
		}
	case "status":
		if err := printStatus(cfg); err != nil {
			fatalf("%v", err)
		}
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n", cmd)
		printUsage()
		os.Exit(2)
	}
}

func initConfig(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	if _, err := os.Stat(path); err == nil {
		fmt.Printf("already exists: %s\n", path)
		return nil
	}
	b, err := os.ReadFile("config.example.env")
	if err != nil {
		b = []byte(fallbackExampleConfig)
	}
	if err := os.WriteFile(path, b, 0o600); err != nil {
		return err
	}
	fmt.Printf("wrote %s — fill ENTRY_IPS and tokens\n", path)
	return nil
}

const fallbackExampleConfig = `# entrypool config (chmod 600)
MODE=pool
ENTRY_NAME=entry.mops.network
ENTRY_IPS=1.2.3.4,5.6.7.8,9.10.11.12
PROVIDERS=cloudflare
PROBE_PORTS=25565,30120,27915,30220,25575,30121,27916,30221,8443,2083
PROBE_TIMEOUT=3
PROBE_NEED_OK=1
FAIL_THRESHOLD=3
OK_THRESHOLD=2
STATE_PATH=/var/lib/entrypool/state.json
CF_API_TOKEN=
CF_ZONE_NAME=mops.network
DNS_TTL=60
REMNAWAVE_URL=
REMNAWAVE_API_TOKEN=
TELEGRAM_BOT_TOKEN=
TELEGRAM_CHAT_ID=
`

func printUsage() {
	fmt.Printf(`entrypool %s — RU entry pool health (kuzrelay-style)

Usage:
  entrypool doctor              probe all ENTRY_IPS
  entrypool run                 one check cycle (timer calls this)
  entrypool status              show last state
  entrypool init-config         write /etc/entrypool/config.env
  entrypool version

  entrypool -c /path/config.env <command>

Install (as root, from repo):
  sudo sh install.sh

`, Version)
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "entrypool: "+format+"\n", args...)
	os.Exit(1)
}
