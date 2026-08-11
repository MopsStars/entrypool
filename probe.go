package main

import (
	"net"
	"strconv"
	"time"
)

func tcpOK(ip string, port int, timeout time.Duration) bool {
	addr := net.JoinHostPort(ip, strconv.Itoa(port))
	conn, err := net.DialTimeout("tcp", addr, timeout)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}

func hostAlive(ip string, ports []int, timeout time.Duration, need int) bool {
	ok := 0
	for _, port := range ports {
		if tcpOK(ip, port, timeout) {
			ok++
			if ok >= need {
				return true
			}
		}
	}
	return false
}
