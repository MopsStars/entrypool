package main

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	defaultConfigPath = "/etc/entrypool/config.env"
	defaultStatePath  = "/var/lib/entrypool/state.json"
)

// Mode controls what happens after probes.
//
//	pool     — keep Remnawave on ENTRY_NAME; sync Cloudflare A to all healthy IPs
//	failover — Remnawave: domain if ≥2 up, else single live IP (legacy)
//	alert    — probe + Telegram only (no DNS / panel writes)
type Mode string

const (
	ModePool     Mode = "pool"
	ModeFailover Mode = "failover"
	ModeAlert    Mode = "alert"
)

type Config struct {
	Mode      Mode
	EntryName string
	EntryIPs  []string

	Providers []string // cloudflare, remnawave (ignored in alert mode except none)

	ProbePorts   []int
	ProbeTimeout time.Duration
	ProbeNeedOK  int
	FailThreshold int
	OKThreshold   int

	StatePath string
	DryRun    bool

	RemnawaveURL   string
	RemnawaveToken string

	CFAPIToken string
	CFZoneID   string
	CFZoneName string
	DNSTTL     int

	TelegramBotToken string
	TelegramChatID   string
}

func loadConfig(path string) (*Config, error) {
	envMap := map[string]string{}
	if path != "" {
		if err := loadEnvFile(path, envMap); err != nil && !os.IsNotExist(err) {
			return nil, err
		}
		// Prefer file values, but allow process env to override.
		for k, v := range envMap {
			if os.Getenv(k) == "" {
				_ = os.Setenv(k, v)
			}
		}
	}

	cfg := &Config{
		Mode:          Mode(strings.ToLower(envOr("MODE", "pool"))),
		EntryName:     envOr("ENTRY_NAME", "entry.example.com"),
		StatePath:     envOr("STATE_PATH", defaultStatePath),
		ProbeTimeout:  durationSeconds(envOr("PROBE_TIMEOUT", "3")),
		ProbeNeedOK:   envInt("PROBE_NEED_OK", 1),
		FailThreshold: envInt("FAIL_THRESHOLD", 3),
		OKThreshold:   envInt("OK_THRESHOLD", 2),
		DNSTTL:        envInt("DNS_TTL", 60),
		CFZoneName:    envOr("CF_ZONE_NAME", "example.com"),
		CFZoneID:      envOr("CF_ZONE_ID", ""),
		DryRun:        truthy(envOr("DRY_RUN", "0")),
	}

	ips := splitCSV(envOr("ENTRY_IPS", ""))
	// Back-compat with old Python checker.
	if len(ips) == 0 {
		if tw := strings.TrimSpace(envOr("TW_IP", "")); tw != "" {
			ips = append(ips, tw)
		}
		if bg := strings.TrimSpace(envOr("BEGET_IP", "")); bg != "" {
			ips = append(ips, bg)
		}
	}
	cfg.EntryIPs = uniqueStrings(ips)
	if len(cfg.EntryIPs) == 0 {
		return nil, fmt.Errorf("ENTRY_IPS is empty")
	}

	cfg.ProbePorts = parsePorts(envOr(
		"PROBE_PORTS",
		"25565,30120,27915,30220,25575,30121,27916,30221,8443,2083",
	))
	if len(cfg.ProbePorts) == 0 {
		return nil, fmt.Errorf("PROBE_PORTS is empty")
	}

	switch cfg.Mode {
	case ModePool, ModeFailover, ModeAlert:
	default:
		return nil, fmt.Errorf("unknown MODE %q (pool|failover|alert)", cfg.Mode)
	}

	provRaw := envOr("PROVIDERS", "")
	if provRaw == "" {
		switch cfg.Mode {
		case ModePool:
			provRaw = "cloudflare"
		case ModeFailover:
			provRaw = "remnawave"
		case ModeAlert:
			provRaw = ""
		}
	}
	for _, p := range splitCSV(provRaw) {
		p = strings.ToLower(p)
		if p != "cloudflare" && p != "remnawave" {
			return nil, fmt.Errorf("unknown provider %q", p)
		}
		cfg.Providers = append(cfg.Providers, p)
	}
	if cfg.Mode == ModeAlert {
		cfg.Providers = nil
	}

	cfg.RemnawaveURL = strings.TrimRight(envOr("REMNAWAVE_URL", ""), "/")
	cfg.RemnawaveToken = envOr("REMNAWAVE_API_TOKEN", "")
	cfg.CFAPIToken = envOr("CF_API_TOKEN", "")
	cfg.TelegramBotToken = firstNonEmpty(envOr("TELEGRAM_BOT_TOKEN", ""), envOr("ALERT_BOT_TOKEN", ""))
	cfg.TelegramChatID = firstNonEmpty(envOr("TELEGRAM_CHAT_ID", ""), envOr("ALERT_CHAT_ID", ""))

	for _, p := range cfg.Providers {
		switch p {
		case "cloudflare":
			if cfg.CFAPIToken == "" && !cfg.DryRun {
				return nil, fmt.Errorf("CF_API_TOKEN required for cloudflare provider")
			}
		case "remnawave":
			if (cfg.RemnawaveURL == "" || cfg.RemnawaveToken == "") && !cfg.DryRun {
				return nil, fmt.Errorf("REMNAWAVE_URL and REMNAWAVE_API_TOKEN required for remnawave provider")
			}
		}
	}
	return cfg, nil
}

func loadEnvFile(path string, out map[string]string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "export ") {
			line = strings.TrimSpace(strings.TrimPrefix(line, "export "))
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		k = strings.TrimSpace(k)
		v = strings.TrimSpace(v)
		v = strings.Trim(v, `"'`)
		out[k] = v
	}
	return sc.Err()
}

func envOr(k, def string) string {
	if v := strings.TrimSpace(os.Getenv(k)); v != "" {
		return v
	}
	return def
}

func envInt(k string, def int) int {
	v := strings.TrimSpace(os.Getenv(k))
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return def
	}
	return n
}

func durationSeconds(raw string) time.Duration {
	f, err := strconv.ParseFloat(raw, 64)
	if err != nil || f <= 0 {
		return 3 * time.Second
	}
	return time.Duration(f * float64(time.Second))
}

func truthy(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func splitCSV(raw string) []string {
	raw = strings.ReplaceAll(raw, ";", ",")
	var out []string
	for _, p := range strings.Split(raw, ",") {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func parsePorts(raw string) []int {
	var out []int
	for _, p := range splitCSV(raw) {
		n, err := strconv.Atoi(p)
		if err != nil || n <= 0 || n > 65535 {
			continue
		}
		out = append(out, n)
	}
	return out
}

func uniqueStrings(in []string) []string {
	seen := map[string]struct{}{}
	var out []string
	for _, s := range in {
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	return out
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}
