package main

import (
	"sync"

	"github.com/gorcon/rcon"
)

// RconClient is a lazy, auto-reconnecting wrapper around a single RCON connection.
type RconClient struct {
	addr     string
	password string

	mu   sync.Mutex
	conn *rcon.Conn
}

func NewRconClient(addr, password string) *RconClient {
	return &RconClient{addr: addr, password: password}
}

func (c *RconClient) connectLocked() error {
	conn, err := rcon.Dial(c.addr, c.password)
	if err != nil {
		return err
	}
	c.conn = conn
	return nil
}

// Exec runs a command, reconnecting once if the connection went stale
// (game server restart, idle timeout).
func (c *RconClient) Exec(cmd string) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.conn == nil {
		if err := c.connectLocked(); err != nil {
			return "", err
		}
	}
	out, err := c.conn.Execute(cmd)
	if err == nil {
		return out, nil
	}
	c.conn.Close()
	c.conn = nil
	if err := c.connectLocked(); err != nil {
		return "", err
	}
	return c.conn.Execute(cmd)
}
