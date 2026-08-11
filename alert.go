package main

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

func telegramAlert(cfg *Config, text string) {
	if cfg.TelegramBotToken == "" || cfg.TelegramChatID == "" {
		fmt.Printf("alert(no telegram configured): %s\n", text)
		return
	}
	form := url.Values{}
	form.Set("chat_id", cfg.TelegramChatID)
	form.Set("text", text)
	form.Set("disable_web_page_preview", "true")
	client := &http.Client{Timeout: 20 * time.Second}
	resp, err := client.PostForm(
		"https://api.telegram.org/bot"+cfg.TelegramBotToken+"/sendMessage",
		form,
	)
	if err != nil {
		fmt.Printf("telegram alert failed: %v\n", err)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		fmt.Printf("telegram alert http %d\n", resp.StatusCode)
		return
	}
	fmt.Println("telegram alert sent")
}

func formatAlert(cfg *Config, st *State, desired []string, allDown bool) string {
	var b strings.Builder
	b.WriteString("entrypool\n")
	b.WriteString("mode=" + string(cfg.Mode) + "\n")
	b.WriteString("host=" + cfg.EntryName + "\n")
	for _, ip := range cfg.EntryIPs {
		e := st.Entries[ip]
		logic := "DOWN"
		probe := false
		if e != nil {
			probe = e.Probe
			if e.Logical {
				logic = "up"
			}
		}
		fmt.Fprintf(&b, "%s logical=%s probe=%v\n", ip, logic, probe)
	}
	b.WriteString("desired -> " + strings.Join(desired, ", ") + "\n")
	if len(cfg.Providers) > 0 {
		b.WriteString("providers=" + strings.Join(cfg.Providers, ",") + "\n")
	}
	if allDown {
		b.WriteString("WARNING: all entries down\n")
	}
	return b.String()
}
