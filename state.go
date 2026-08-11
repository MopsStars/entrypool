package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

type entryCounters struct {
	Fail    int  `json:"fail"`
	OK      int  `json:"ok"`
	Logical bool `json:"logical"`
	Probe   bool `json:"probe"`
}

type State struct {
	Entries   map[string]*entryCounters `json:"entries"`
	Desired   []string                  `json:"desired"`
	LastAlert string                    `json:"last_alert"`
	UpdatedAt int64                     `json:"updated_at"`
	Mode      string                    `json:"mode"`
	Providers []string                  `json:"providers"`
}

func loadState(path string, ips []string) *State {
	st := &State{
		Entries: map[string]*entryCounters{},
	}
	for _, ip := range ips {
		st.Entries[ip] = &entryCounters{Logical: true}
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return st
	}
	var raw State
	if err := json.Unmarshal(b, &raw); err != nil {
		return st
	}
	if raw.Entries == nil {
		raw.Entries = map[string]*entryCounters{}
	}
	for _, ip := range ips {
		if e, ok := raw.Entries[ip]; ok && e != nil {
			st.Entries[ip] = e
		} else {
			st.Entries[ip] = &entryCounters{Logical: true}
		}
	}
	st.Desired = raw.Desired
	st.LastAlert = raw.LastAlert
	return st
}

func saveState(path string, st *State) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	st.UpdatedAt = time.Now().Unix()
	b, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, append(b, '\n'), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
