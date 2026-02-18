package twitch_live

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/samber/do"
)

const clientId = "kimne78kx3ncx6brgo4mv6wki5h1ko"

type Client struct {
	client *http.Client
}

func NewClient(di *do.Injector) (*Client, error) {
	return &Client{
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}, nil
}

type AccessToken struct {
	Value     string `json:"value"`
	Signature string `json:"signature"`
}

type StreamQuality struct {
	Quality    string `json:"quality"`
	Resolution string `json:"resolution"`
	URL        string `json:"url"`
}

func (c *Client) getAccessToken(ctx context.Context, id string) (*AccessToken, error) {
	type persistedQuery struct {
		Version    int    `json:"version"`
		Sha256Hash string `json:"sha256Hash"`
	}

	type extensions struct {
		PersistedQuery persistedQuery `json:"persistedQuery"`
	}

	type variables struct {
		IsLive     bool   `json:"isLive"`
		Login      string `json:"login"`
		IsVod      bool   `json:"isVod"`
		VodID      string `json:"vodID"`
		PlayerType string `json:"playerType"`
	}

	type requestBody struct {
		OperationName string     `json:"operationName"`
		Extensions    extensions `json:"extensions"`
		Variables     variables  `json:"variables"`
	}

	variablesData := variables{
		Login:      id,
		IsLive:     true,
		PlayerType: "embed",
	}

	requestData := requestBody{
		OperationName: "PlaybackAccessToken",
		Extensions: extensions{
			PersistedQuery: persistedQuery{
				Version:    1,
				Sha256Hash: "0828119ded1c13477966434e15800ff57ddacf13ba1911c129dc2200705b0712",
			},
		},
		Variables: variablesData,
	}

	jsonData, err := json.Marshal(requestData)
	if err != nil {
		return nil, fmt.Errorf("Marshal: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", "https://gql.twitch.tv/gql", strings.NewReader(string(jsonData)))
	if err != nil {
		return nil, fmt.Errorf("NewRequestWithContext: %w", err)
	}

	req.Header.Set("Client-Id", clientId)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("Do: %w", err)
	}

	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("ReadAll: %w", err)
	}

	if resp.StatusCode != 200 {
		var errorResponse struct {
			Message string `json:"message"`
		}
		if err := json.Unmarshal(body, &errorResponse); err != nil {
			return nil, fmt.Errorf("HTTP %d: failed to parse error response", resp.StatusCode)
		}
		return nil, errors.New(errorResponse.Message)
	}

	var response struct {
		Data struct {
			StreamPlaybackAccessToken *AccessToken `json:"streamPlaybackAccessToken"`
			VideoPlaybackAccessToken  *AccessToken `json:"videoPlaybackAccessToken"`
		} `json:"data"`
	}

	if err := json.Unmarshal(body, &response); err != nil {
		return nil, fmt.Errorf("json.Unmarshal", resp.StatusCode)
	}

	return response.Data.StreamPlaybackAccessToken, nil
}

func (c *Client) getPlaylist(ctx context.Context, id string, accessToken *AccessToken) (string, error) {
	url := fmt.Sprintf("https://usher.ttvnw.net/%s/%s.m3u8?client_id=%s&token=%s&sig=%s&allow_source=true&allow_audio_only=true",
		"api/channel/hls",
		id,
		clientId,
		accessToken.Value,
		accessToken.Signature,
	)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return "", fmt.Errorf("NewRequestWithContext: %w", err)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("Do: %w", err)
	}

	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("ReadAll: %w", err)
	}

	switch resp.StatusCode {
	case 200:
		return string(body), nil
	case 404:
		return "", fmt.Errorf("transcode does not exist - the stream is probably offline: %s", string(body))
	default:
		return "", fmt.Errorf("twitch returned status code %d: %s", resp.StatusCode, string(body))
	}
}

func parsePlaylist(playlist string) []StreamQuality {
	parsedPlaylist := []StreamQuality{}
	lines := strings.Split(playlist, "\n")

	for i := 4; i < len(lines); i += 3 {
		if i-2 < 0 || i-1 < 0 || i >= len(lines) {
			continue
		}

		qualityLine := lines[i-2]
		resolutionLine := lines[i-1]
		urlLine := lines[i]

		quality := ""
		if strings.Contains(qualityLine, "NAME=\"") {
			parts := strings.Split(qualityLine, "NAME=\"")
			if len(parts) > 1 {
				quality = strings.Split(parts[1], "\"")[0]
			}
		}

		resolution := ""
		if strings.Contains(resolutionLine, "RESOLUTION=") {
			parts := strings.Split(resolutionLine, "RESOLUTION=")
			if len(parts) > 1 {
				resolution = strings.Split(parts[1], ",")[0]
			}
		}

		parsedPlaylist = append(parsedPlaylist, StreamQuality{
			Quality:    quality,
			Resolution: resolution,
			URL:        urlLine,
		})
	}

	return parsedPlaylist
}

func (c *Client) GetM3U8(ctx context.Context, channel string) ([]StreamQuality, error) {
	accessToken, err := c.getAccessToken(ctx, channel)
	if err != nil {
		return nil, fmt.Errorf("getAccessToken: %w", err)
	}

	playlist, err := c.getPlaylist(ctx, channel, accessToken)
	if err != nil {
		return nil, fmt.Errorf("getPlaylist: %w", err)
	}

	return parsePlaylist(playlist), nil
}
