// Package hermes provides NATS client integration for the Hermes message bus.
package hermes

import (
	"fmt"
	"log/slog"

	"github.com/nats-io/nats.go"
)

// Client wraps a NATS connection for Hermes integration.
type Client struct {
	conn   *nats.Conn
	js     nats.JetStreamContext
	logger *slog.Logger
}

// NewClient creates a new Hermes NATS client.
func NewClient(url string, logger *slog.Logger) (*Client, error) {
	nc, err := nats.Connect(url,
		nats.Name("alexandria"),
		nats.RetryOnFailedConnect(true),
		nats.MaxReconnects(-1),
		nats.DisconnectErrHandler(func(_ *nats.Conn, err error) {
			if err != nil {
				logger.Warn("NATS disconnected", "error", err)
			}
		}),
		nats.ReconnectHandler(func(_ *nats.Conn) {
			logger.Info("NATS reconnected")
		}),
	)
	if err != nil {
		return nil, fmt.Errorf("connecting to NATS: %w", err)
	}

	js, err := nc.JetStream()
	if err != nil {
		nc.Close()
		return nil, fmt.Errorf("creating JetStream context: %w", err)
	}

	return &Client{
		conn:   nc,
		js:     js,
		logger: logger,
	}, nil
}

// JetStream returns the JetStream context.
func (c *Client) JetStream() nats.JetStreamContext {
	return c.js
}

// Conn returns the underlying NATS connection.
func (c *Client) Conn() *nats.Conn {
	return c.conn
}

// Close closes the NATS connection.
func (c *Client) Close() {
	if c.conn != nil {
		c.conn.Close()
	}
}

// IsConnected returns true if the NATS connection is active.
func (c *Client) IsConnected() bool {
	return c.conn != nil && c.conn.IsConnected()
}
