// Package client is the CLI's only network code: GET a /api/v1 path with the
// workspace token and the CLI's User-Agent, and hand back status, body and
// the server's advertised CLI versions as data. It decides nothing; the
// query layer interprets responses.
package client

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/lastridge/shipglance-cli/internal/auth"
)

// The version headers the server sends on every /api/v1 response once CLI
// releases are published.
const (
	HeaderLatest  = "X-Shipglance-Cli-Latest"
	HeaderMinimum = "X-Shipglance-Cli-Minimum"
)

// maxBody bounds a response read; v1 bodies are small, and a runaway body is
// a server fault, not something to buffer without limit.
const maxBody = 8 << 20

// Response is one answered request.
type Response struct {
	Status      int
	ContentType string
	Body        []byte
	LatestCLI   string
	MinimumCLI  string
}

// Client sends authenticated GETs to one workspace.
type Client struct {
	target    auth.Target
	userAgent string
	http      *http.Client
}

// UserAgent is the exact string the server's version gate parses.
func UserAgent(cliVersion, goos, goarch, mode string) string {
	return fmt.Sprintf("shipglance-cli/%s (%s/%s; %s)", cliVersion, goos, goarch, mode)
}

func New(target auth.Target, userAgent string, httpClient *http.Client) *Client {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 30 * time.Second}
	}
	return &Client{target: target, userAgent: userAgent, http: httpClient}
}

// Get requests path (e.g. "/api/v1/status") with query. An error means no
// HTTP response arrived at all (DNS, connection, timeout): a transport
// failure, distinct from any answer the server gave.
func (c *Client) Get(ctx context.Context, path string, query url.Values) (Response, error) {
	target := c.target.BaseURL + path
	if len(query) > 0 {
		target += "?" + query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return Response{}, err
	}
	req.Header.Set("Authorization", "Bearer "+c.target.Token)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", c.userAgent)
	resp, err := c.http.Do(req)
	if err != nil {
		return Response{}, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBody))
	if err != nil {
		return Response{}, err
	}
	return Response{
		Status:      resp.StatusCode,
		ContentType: resp.Header.Get("Content-Type"),
		Body:        body,
		LatestCLI:   resp.Header.Get(HeaderLatest),
		MinimumCLI:  resp.Header.Get(HeaderMinimum),
	}, nil
}
