package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

const cfAPI = "https://api.cloudflare.com/client/v4"

type cfClient struct {
	token  string
	http   *http.Client
	dryRun bool
}

func newCF(token string, dryRun bool) *cfClient {
	return &cfClient{
		token:  token,
		dryRun: dryRun,
		http:   &http.Client{Timeout: 60 * time.Second},
	}
}

func (c *cfClient) do(method, path string, body any) (json.RawMessage, error) {
	var rdr io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		rdr = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, cfAPI+path, rdr)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	var wrap struct {
		Success bool            `json:"success"`
		Errors  any             `json:"errors"`
		Result  json.RawMessage `json:"result"`
	}
	if err := json.Unmarshal(raw, &wrap); err != nil {
		return nil, fmt.Errorf("cloudflare %s %s: bad json: %s", method, path, truncate(string(raw), 400))
	}
	if !wrap.Success {
		return nil, fmt.Errorf("cloudflare %s %s -> %d: %s", method, path, resp.StatusCode, truncate(string(raw), 600))
	}
	return wrap.Result, nil
}

func (c *cfClient) resolveZoneID(zoneName, configured string) (string, error) {
	if configured != "" {
		return configured, nil
	}
	q := url.Values{"name": {zoneName}}
	res, err := c.do("GET", "/zones?"+q.Encode(), nil)
	if err != nil {
		return "", err
	}
	var zones []struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(res, &zones); err != nil {
		return "", err
	}
	if len(zones) == 0 {
		return "", fmt.Errorf("cloudflare zone not found: %s", zoneName)
	}
	return zones[0].ID, nil
}

type cfDNSRecord struct {
	ID      string `json:"id"`
	Type    string `json:"type"`
	Name    string `json:"name"`
	Content string `json:"content"`
	TTL     int    `json:"ttl"`
	Proxied bool   `json:"proxied"`
}

func (c *cfClient) listA(zoneID, name string) ([]cfDNSRecord, error) {
	q := url.Values{"type": {"A"}, "name": {name}, "per_page": {"100"}}
	res, err := c.do("GET", "/zones/"+zoneID+"/dns_records?"+q.Encode(), nil)
	if err != nil {
		return nil, err
	}
	var recs []cfDNSRecord
	if err := json.Unmarshal(res, &recs); err != nil {
		return nil, err
	}
	return recs, nil
}

func (c *cfClient) upsertA(zoneID, name, ip string, ttl int, existing []cfDNSRecord) error {
	for _, rec := range existing {
		if rec.Content == ip {
			if rec.TTL == ttl && !rec.Proxied {
				fmt.Printf("cf keep A %s -> %s\n", name, ip)
				return nil
			}
			if c.dryRun {
				fmt.Printf("cf dry-run patch A %s -> %s\n", name, ip)
				return nil
			}
			_, err := c.do("PATCH", "/zones/"+zoneID+"/dns_records/"+rec.ID, map[string]any{
				"ttl":     ttl,
				"proxied": false,
				"content": ip,
			})
			if err != nil {
				return err
			}
			fmt.Printf("cf patched A %s -> %s ttl=%d\n", name, ip, ttl)
			return nil
		}
	}
	if c.dryRun {
		fmt.Printf("cf dry-run create A %s -> %s\n", name, ip)
		return nil
	}
	_, err := c.do("POST", "/zones/"+zoneID+"/dns_records", map[string]any{
		"type":    "A",
		"name":    name,
		"content": ip,
		"ttl":     ttl,
		"proxied": false,
	})
	if err != nil {
		return err
	}
	fmt.Printf("cf created A %s -> %s ttl=%d\n", name, ip, ttl)
	return nil
}

func (c *cfClient) deleteA(zoneID, ip string, existing []cfDNSRecord) error {
	for _, rec := range existing {
		if rec.Content != ip {
			continue
		}
		if c.dryRun {
			fmt.Printf("cf dry-run delete A -> %s\n", ip)
			return nil
		}
		_, err := c.do("DELETE", "/zones/"+zoneID+"/dns_records/"+rec.ID, nil)
		if err != nil {
			return err
		}
		fmt.Printf("cf deleted A -> %s\n", ip)
		return nil
	}
	return nil
}

func syncCloudflare(cfg *Config, desired []string) error {
	c := newCF(cfg.CFAPIToken, cfg.DryRun)
	zoneID, err := c.resolveZoneID(cfg.CFZoneName, cfg.CFZoneID)
	if err != nil {
		return err
	}
	existing, err := c.listA(zoneID, cfg.EntryName)
	if err != nil {
		return err
	}
	current := map[string]struct{}{}
	for _, r := range existing {
		current[r.Content] = struct{}{}
	}
	fmt.Printf("cf current A=%v desired=%v\n", keys(current), desired)

	pool := map[string]struct{}{}
	for _, ip := range cfg.EntryIPs {
		pool[ip] = struct{}{}
	}

	for _, ip := range desired {
		if err := c.upsertA(zoneID, cfg.EntryName, ip, cfg.DNSTTL, existing); err != nil {
			return err
		}
	}
	existing, err = c.listA(zoneID, cfg.EntryName)
	if err != nil {
		return err
	}
	desiredSet := map[string]struct{}{}
	for _, ip := range desired {
		desiredSet[ip] = struct{}{}
	}
	for _, rec := range existing {
		ip := rec.Content
		if _, keep := desiredSet[ip]; keep {
			continue
		}
		if _, managed := pool[ip]; !managed {
			continue
		}
		if err := c.deleteA(zoneID, ip, existing); err != nil {
			return err
		}
	}
	return nil
}

func keys(m map[string]struct{}) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
