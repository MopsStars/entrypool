package main

import (
	"fmt"
	"sort"
	"strings"
)

func runOnce(cfg *Config) error {
	st := loadState(cfg.StatePath, cfg.EntryIPs)

	for _, ip := range cfg.EntryIPs {
		e := st.Entries[ip]
		alive := hostAlive(ip, cfg.ProbePorts, cfg.ProbeTimeout, cfg.ProbeNeedOK)
		e.Probe = alive
		if alive {
			e.OK++
			e.Fail = 0
		} else {
			e.Fail++
			e.OK = 0
		}
		if e.Fail >= cfg.FailThreshold {
			e.Logical = false
		} else if e.OK >= cfg.OKThreshold {
			e.Logical = true
		}
		fmt.Printf("probe %s alive=%v fail=%d ok=%d logical=%v\n",
			ip, alive, e.Fail, e.OK, e.Logical)
	}

	desired := decideDesired(cfg.EntryIPs, st)
	prev := append([]string{}, st.Desired...)
	fmt.Printf("desired=%v providers=%v mode=%s dry_run=%v\n",
		desired, cfg.Providers, cfg.Mode, cfg.DryRun)

	for _, p := range cfg.Providers {
		switch p {
		case "cloudflare":
			if err := syncCloudflare(cfg, desired); err != nil {
				return err
			}
		case "remnawave":
			if err := syncRemnawave(cfg, desired); err != nil {
				return err
			}
		}
	}

	upCount := 0
	for _, ip := range cfg.EntryIPs {
		if st.Entries[ip].Logical {
			upCount++
		}
	}
	allDown := upCount == 0
	allUp := upCount == len(cfg.EntryIPs)
	changed := !sameStringSet(prev, desired)
	alertKey := strings.Join(sortedCopy(desired), ",") + "|" + fmt.Sprint(allDown)
	bootstrapOK := len(prev) == 0 && allUp && !allDown

	if (changed || allDown) && !bootstrapOK {
		if st.LastAlert != alertKey {
			telegramAlert(cfg, formatAlert(cfg, st, desired, allDown))
			st.LastAlert = alertKey
		}
	} else if bootstrapOK {
		st.LastAlert = alertKey
		fmt.Println("skip telegram: initial all-up bootstrap")
	}

	st.Desired = desired
	st.Mode = string(cfg.Mode)
	st.Providers = cfg.Providers
	if cfg.DryRun {
		fmt.Printf("dry-run: state not saved (%s)\n", cfg.StatePath)
		return nil
	}
	return saveState(cfg.StatePath, st)
}

func decideDesired(ips []string, st *State) []string {
	var up []string
	for _, ip := range ips {
		if st.Entries[ip] != nil && st.Entries[ip].Logical {
			up = append(up, ip)
		}
	}
	if len(up) == 0 {
		// Keep all in DNS/target so we don't delete everything on total outage.
		return append([]string{}, ips...)
	}
	return up
}

func sameStringSet(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	return strings.Join(sortedCopy(a), ",") == strings.Join(sortedCopy(b), ",")
}

func sortedCopy(in []string) []string {
	out := append([]string{}, in...)
	sort.Strings(out)
	return out
}

func printStatus(cfg *Config) error {
	st := loadState(cfg.StatePath, cfg.EntryIPs)
	fmt.Printf("config: %s\n", defaultConfigPath)
	fmt.Printf("mode=%s entry=%s\n", cfg.Mode, cfg.EntryName)
	fmt.Printf("providers=%v\n", cfg.Providers)
	fmt.Printf("state=%s updated_at=%d\n", cfg.StatePath, st.UpdatedAt)
	fmt.Printf("desired=%v\n", st.Desired)
	for _, ip := range cfg.EntryIPs {
		e := st.Entries[ip]
		if e == nil {
			fmt.Printf("  %s (no state)\n", ip)
			continue
		}
		fmt.Printf("  %s logical=%v fail=%d ok=%d last_probe=%v\n",
			ip, e.Logical, e.Fail, e.OK, e.Probe)
	}
	return nil
}

func doctor(cfg *Config) error {
	fmt.Printf("entrypool %s (%s)\n", Version, Commit)
	fmt.Printf("mode=%s entry=%s ips=%v\n", cfg.Mode, cfg.EntryName, cfg.EntryIPs)
	fmt.Printf("ports=%v timeout=%s need_ok=%d\n", cfg.ProbePorts, cfg.ProbeTimeout, cfg.ProbeNeedOK)
	fmt.Printf("providers=%v\n", cfg.Providers)
	okAll := true
	for _, ip := range cfg.EntryIPs {
		alive := hostAlive(ip, cfg.ProbePorts, cfg.ProbeTimeout, cfg.ProbeNeedOK)
		mark := "FAIL"
		if alive {
			mark = "ok"
		} else {
			okAll = false
		}
		fmt.Printf("  probe %s -> %s\n", ip, mark)
	}
	if cfg.Mode == ModeAlert {
		fmt.Println("providers: none (alert-only)")
	}
	if !okAll {
		return fmt.Errorf("one or more entries failed probe")
	}
	fmt.Println("doctor: ok")
	return nil
}
