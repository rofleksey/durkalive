package twitch_irc

import (
	"context"
	"durkalive/app/client/twitch"
	"durkalive/app/config"
	"strings"
	"sync"
	"time"

	"log/slog"

	irc "github.com/gempir/go-twitch-irc/v4"
	"github.com/samber/do"
)

type MessageHandler func(channel, username, messageID, text string, tags map[string]string)

type Client struct {
	cfg       *config.Config
	apiClient *twitch.Client
	ircClient *irc.Client

	mutex             sync.RWMutex
	connectedChannels map[string]bool
	messageHandler    MessageHandler
}

func NewClient(di *do.Injector) (*Client, error) {
	cfg := do.MustInvoke[*config.Config](di)
	apiClient := do.MustInvoke[*twitch.Client](di)

	client := &Client{
		cfg:               cfg,
		apiClient:         apiClient,
		connectedChannels: make(map[string]bool),
	}

	client.ircClient = irc.NewClient(cfg.Twitch.ClientID, "oauth:"+apiClient.AccessToken())
	client.setupIRCListeners()

	return client, nil
}

func (c *Client) setupIRCListeners() {
	c.ircClient.OnPrivateMessage(func(message irc.PrivateMessage) {
		username := strings.ToLower(message.User.Name)
		channel := strings.ToLower(strings.TrimPrefix(message.Channel, "#"))
		text := strings.TrimSpace(message.Message)

		c.mutex.Lock()
		handler := c.messageHandler
		c.mutex.Unlock()

		if handler == nil {
			return
		}

		handler(channel, username, message.ID, text, message.Tags)
	})

	c.ircClient.OnConnect(func() {
		slog.Info("Connected to Twitch IRC")
	})

	c.ircClient.OnReconnectMessage(func(message irc.ReconnectMessage) {
		slog.Info("Reconnecting to Twitch IRC")
	})
}

func (c *Client) Run() error {
	return c.ircClient.Connect()
}

func (c *Client) Disconnect() {
	c.ircClient.Disconnect()
}

func (c *Client) JoinChannel(channel string) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	if c.connectedChannels[channel] {
		return
	}

	c.ircClient.Join(channel)
	c.connectedChannels[channel] = true
}

func (c *Client) LeaveChannel(channel string) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	if !c.connectedChannels[channel] {
		return
	}

	c.ircClient.Depart(channel)
	delete(c.connectedChannels, channel)
}

func (c *Client) SetListener(listener MessageHandler) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	c.messageHandler = listener
}

func (c *Client) RunRefreshLoop(ctx context.Context) {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			c.refreshToken()
		}
	}
}

func (c *Client) refreshToken() {
	c.ircClient.SetIRCToken("oauth:" + c.apiClient.AccessToken())
}
