package twitch

import (
	"context"
	"durkalive/app/config"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/nicklaw5/helix/v2"
	"github.com/samber/do"
)

type Client struct {
	cfg        *config.Config
	userClient *helix.Client
}

func NewClient(di *do.Injector) (*Client, error) {
	ctx := do.MustInvoke[context.Context](di)
	cfg := do.MustInvoke[*config.Config](di)

	httpClient := &http.Client{
		Timeout: 10 * time.Second,
	}

	helixClient, err := helix.NewClientWithContext(ctx, &helix.Options{
		ClientID:     cfg.Twitch.ClientID,
		ClientSecret: cfg.Twitch.ClientSecret,
		RefreshToken: cfg.Twitch.RefreshToken,
		HTTPClient:   httpClient,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create helix client: %v", err)
	}

	resp, err := helixClient.RefreshUserAccessToken(cfg.Twitch.RefreshToken)
	if err != nil {
		return nil, fmt.Errorf("failed to get access token: %v", err)
	}
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("failed to get token: status %d: %s", resp.StatusCode, resp.ErrorMessage)
	}

	accessToken := resp.Data.AccessToken
	helixClient.SetUserAccessToken(accessToken)

	return &Client{
		cfg:        cfg,
		userClient: helixClient,
	}, nil
}

func (c *Client) GetUserIDByUsername(username string) (string, error) {
	resp, err := c.userClient.GetUsers(&helix.UsersParams{
		Logins: []string{username},
	})
	if err != nil {
		return "", fmt.Errorf("failed to get user info: %v", err)
	}
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("failed to get user info: status %d: %s", resp.StatusCode, resp.ErrorMessage)
	}

	if len(resp.Data.Users) == 0 {
		return "", fmt.Errorf("failed to get user info: no users found")
	}

	return resp.Data.Users[0].ID, nil
}

func (c *Client) SendMessage(channel, text string) error {
	broadcasterID, err := c.GetUserIDByUsername(channel)
	if err != nil {
		return fmt.Errorf("failed to get broadcaster id: %v", err)
	}

	senderID, err := c.GetUserIDByUsername(c.cfg.Twitch.Username)
	if err != nil {
		return fmt.Errorf("failed to get sender id: %v", err)
	}

	resp, err := c.userClient.SendChatMessage(&helix.SendChatMessageParams{
		BroadcasterID: broadcasterID,
		SenderID:      senderID,
		Message:       text,
	})
	if err != nil {
		return fmt.Errorf("failed to send message: %v", err)
	}
	if resp.StatusCode != 200 {
		return fmt.Errorf("failed to send message: status %d: %s", resp.StatusCode, resp.ErrorMessage)
	}

	return nil
}

func (c *Client) GetStreamStartedAt(username string) (time.Time, error) {
	userID, err := c.GetUserIDByUsername(username)
	if err != nil {
		return time.Time{}, fmt.Errorf("failed to get user id: %v", err)
	}

	resp, err := c.userClient.GetStreams(&helix.StreamsParams{
		UserIDs: []string{userID},
	})
	if err != nil {
		return time.Time{}, fmt.Errorf("failed to get stream info: %v", err)
	}
	if resp.StatusCode != 200 {
		return time.Time{}, fmt.Errorf("failed to get stream info: status %d: %s", resp.StatusCode, resp.ErrorMessage)
	}

	if len(resp.Data.Streams) == 0 {
		return time.Time{}, fmt.Errorf("stream is not live")
	}

	stream := resp.Data.Streams[0]

	return stream.StartedAt, nil
}

func (c *Client) refreshToken() {
	slog.Debug("Refreshing twitch access token",
		slog.String("username", c.cfg.Twitch.Username),
	)

	resp, err := c.userClient.RefreshUserAccessToken(c.cfg.Twitch.RefreshToken)
	if err != nil {
		slog.Error("Failed to refresh user access token", slog.Any("error", err))
		return
	}
	if resp.StatusCode != 200 {
		slog.Error("Failed to refresh access token", slog.Int("status", resp.StatusCode), slog.String("error", resp.ErrorMessage))
		return
	}

	c.userClient.SetUserAccessToken(resp.Data.AccessToken)

	slog.Debug("Twitch access token refreshed successfully",
		slog.String("username", c.cfg.Twitch.Username),
	)
}

func (c *Client) RunRefreshLoop(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Minute)
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
