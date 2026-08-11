package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type rwClient struct {
	base   string
	token  string
	dryRun bool
	http   *http.Client
}

func newRW(base, token string, dryRun bool) *rwClient {
	return &rwClient{
		base:   base,
		token:  token,
		dryRun: dryRun,
		http:   &http.Client{Timeout: 60 * time.Second},
	}
}

func (c *rwClient) do(method, path string, body any) (json.RawMessage, int, error) {
	var rdr io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, 0, err
		}
		rdr = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, c.base+path, rdr)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	return raw, resp.StatusCode, nil
}

func (c *rwClient) hosts() ([]map[string]any, error) {
	raw, code, err := c.do("GET", "/api/hosts", nil)
	if err != nil {
		return nil, err
	}
	if code != 200 {
		return nil, fmt.Errorf("remnawave GET /api/hosts -> %d: %s", code, truncate(string(raw), 600))
	}
	var wrap struct {
		Response json.RawMessage `json:"response"`
	}
	if err := json.Unmarshal(raw, &wrap); err != nil {
		return nil, err
	}
	payload := wrap.Response
	if len(payload) == 0 {
		payload = raw
	}
	var list []map[string]any
	if err := json.Unmarshal(payload, &list); err != nil {
		// sometimes response is object with nested list — try raw array
		if err2 := json.Unmarshal(raw, &list); err2 != nil {
			return nil, fmt.Errorf("unexpected hosts payload: %w", err)
		}
	}
	return list, nil
}

func (c *rwClient) patchHost(body map[string]any) error {
	if c.dryRun {
		fmt.Printf("rw dry-run patch uuid=%v address=%v\n", body["uuid"], body["address"])
		return nil
	}
	raw, code, err := c.do("PATCH", "/api/hosts", body)
	if err != nil {
		return err
	}
	if code != 200 && code != 201 {
		return fmt.Errorf("remnawave PATCH /api/hosts -> %d: %s", code, truncate(string(raw), 600))
	}
	return nil
}

func entryHostMatch(addr, entryName string, pool []string) bool {
	addr = strings.TrimSpace(addr)
	if addr == entryName {
		return true
	}
	for _, ip := range pool {
		if addr == ip {
			return true
		}
	}
	return false
}

func syncRemnawave(cfg *Config, desired []string) error {
	c := newRW(cfg.RemnawaveURL, cfg.RemnawaveToken, cfg.DryRun)

	var target string
	switch cfg.Mode {
	case ModePool:
		// Always keep subscription on the shared name; DNS decides the pool.
		target = cfg.EntryName
	case ModeFailover:
		switch {
		case len(desired) >= 2:
			target = cfg.EntryName
		case len(desired) == 1:
			target = desired[0]
		default:
			target = cfg.EntryName
		}
	default:
		target = cfg.EntryName
	}

	hosts, err := c.hosts()
	if err != nil {
		return err
	}
	var targets []map[string]any
	for _, h := range hosts {
		addr, _ := h["address"].(string)
		if entryHostMatch(addr, cfg.EntryName, cfg.EntryIPs) {
			targets = append(targets, h)
		}
	}
	fmt.Printf("remnawave entry-hosts=%d target_address=%s dry_run=%v mode=%s\n",
		len(targets), target, cfg.DryRun, cfg.Mode)

	for _, h := range targets {
		cur, _ := h["address"].(string)
		remark, _ := h["remark"].(string)
		port := h["port"]
		if cur == target {
			fmt.Printf("rw keep %s :%v address=%s\n", remark, port, cur)
			continue
		}
		fmt.Printf("rw set %s :%v %s -> %s\n", remark, port, cur, target)
		body := buildHostPatch(h, target)
		if err := c.patchHost(body); err != nil {
			return err
		}
	}
	return nil
}

func buildHostPatch(h map[string]any, address string) map[string]any {
	inbound, _ := h["inbound"].(map[string]any)
	if inbound == nil {
		inbound = map[string]any{}
	}
	nodes, _ := h["nodes"].([]any)
	if nodes == nil {
		nodes = []any{}
	}
	exclSquads, _ := h["excludedInternalSquads"].([]any)
	if exclSquads == nil {
		exclSquads = []any{}
	}
	exclTypes, _ := h["excludeFromSubscriptionTypes"].([]any)
	if exclTypes == nil {
		exclTypes = []any{}
	}
	return map[string]any{
		"uuid":                         h["uuid"],
		"remark":                       h["remark"],
		"address":                      address,
		"port":                         h["port"],
		"path":                         h["path"],
		"sni":                          h["sni"],
		"host":                         h["host"],
		"alpn":                         h["alpn"],
		"fingerprint":                  h["fingerprint"],
		"isDisabled":                   h["isDisabled"],
		"securityLayer":                h["securityLayer"],
		"xhttpExtraParams":             h["xhttpExtraParams"],
		"muxParams":                    h["muxParams"],
		"sockoptParams":                h["sockoptParams"],
		"finalMask":                    h["finalMask"],
		"serverDescription":            h["serverDescription"],
		"pinnedPeerCertSha256":         h["pinnedPeerCertSha256"],
		"verifyPeerCertByName":         h["verifyPeerCertByName"],
		"shuffleHost":                  h["shuffleHost"],
		"mihomoX25519":                 h["mihomoX25519"],
		"mihomoIpVersion":              h["mihomoIpVersion"],
		"tags":                         h["tags"],
		"isHidden":                     h["isHidden"],
		"overrideSniFromAddress":       h["overrideSniFromAddress"],
		"keepSniBlank":                 h["keepSniBlank"],
		"vlessRouteId":                 h["vlessRouteId"],
		"xrayJsonTemplateUuid":         h["xrayJsonTemplateUuid"],
		"excludedInternalSquads":       exclSquads,
		"excludeFromSubscriptionTypes": exclTypes,
		"nodes":                        nodes,
		"inbound": map[string]any{
			"configProfileUuid":        inbound["configProfileUuid"],
			"configProfileInboundUuid": inbound["configProfileInboundUuid"],
		},
	}
}
