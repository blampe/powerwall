// Primary client implementation using Tesla Fleet API
//
// Functions for client creation and management:
//
//	NewClient(clientID, accessToken, refreshToken) - Creates Fleet API client
//	(*Client) RefreshToken()
//	(*Client) SetRefreshToken()
//	(*Client) GetRefreshToken()
//	(*Client) IsTokenExpired()
//	(*Client) SetRateLimit()
//	(*Client) GetRateLimitStatus()
//	(*Client) GetAPIUsageStats()

package powerwall

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

const (
	// Tesla Fleet API base URL
	FleetAPIBaseURL = "https://fleet-api.prd.na.vn.cloud.tesla.com"

	// OAuth endpoints
	TokenURL = "https://fleet-auth.prd.vn.cloud.tesla.com/oauth2/v3/token"
)

var logFunc = func(v ...interface{}) {}

// SetLogFunc registers a callback function which can be used for debug logging
// of the powerwall library.  The provided function should accept arguments in
// the same format as Printf/Sprintf/etc.  Note that log lines passed to this
// function are *not* newline-terminated, so you will need to add newlines if
// you want to put them out directly to stdout/stderr, etc.
func SetLogFunc(f func(...interface{})) {
	logFunc = f
}

var errFunc = func(string, error) {}

// SetErrFunc registers a callback function which will be called with
// additional information when certain errors occur.  This can be useful if you
// don't want full debug logging, but still want to log additional information
// that might be helpful when troubleshooting, for example, API message format
// errors, etc.
func SetErrFunc(f func(string, error)) {
	errFunc = f
}

// Client represents a connection to Tesla's Fleet API for Powerwall 3
type Client struct {
	accessToken     string
	refreshToken    string
	clientID        string
	tokenExpiry     time.Time
	httpClient      *http.Client
	selectedSiteID  int64
	rateLimitConfig RateLimitConfig

	// Rate limiting
	rateLimitMutex  sync.Mutex
	lastRequestTime time.Time
	requestQueue    chan struct{}
}

// NewClient creates a new Fleet API client using OAuth access and refresh tokens and client ID.
// Users must obtain these tokens through Tesla's mobile app, third-party tools,
// or their own OAuth 2.0 PKCE implementation.
func NewClient(clientID, accessToken, refreshToken string, options ...func(c *Client)) *Client {
	httpClient := &http.Client{
		Timeout: 30 * time.Second, // Fleet API can be slower than local gateway
	}

	c := &Client{
		accessToken:  accessToken,
		refreshToken: refreshToken,
		clientID:     clientID,
		httpClient:   httpClient,
		rateLimitConfig: RateLimitConfig{
			RealtimeDataRPM: 60, // Tesla's limit for live data
			CommandsRPM:     30, // Tesla's limit for commands
			MaxMonthlyCost:  10, // $10 free tier
		},
		requestQueue: make(chan struct{}, 1), // Single-threaded requests
	}

	// Apply options
	for _, option := range options {
		if option != nil {
			option(c)
		}
	}

	c.logf("New Fleet API client created")
	return c
}

// WithHttpClient sets the HTTP client to use for all requests
func WithHttpClient(httpClient *http.Client) func(c *Client) {
	return func(c *Client) {
		c.httpClient = httpClient
	}
}

func (c *Client) logf(format string, v ...interface{}) {
	logFunc(fmt.Sprintf("{FleetAPI %p} ", c) + fmt.Sprintf(format, v...))
}

func (c *Client) jsonError(api string, data []byte, err error) {
	msg := fmt.Sprintf("Error unmarshalling Fleet API '%s' response %s", api, string(data))
	errFunc(msg, err)
}

// RefreshToken refreshes the OAuth access token using the refresh token
func (c *Client) RefreshToken() error {
	c.logf("Refreshing OAuth access token using client_id: %s", c.clientID)

	data := url.Values{}
	data.Set("grant_type", "refresh_token")
	data.Set("refresh_token", c.refreshToken)
	data.Set("client_id", c.clientID) // Use the configured client ID

	req, err := http.NewRequest("POST", TokenURL, strings.NewReader(data.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	if resp.StatusCode != 200 {
		c.logf("Token refresh failed: status=%d body=%s", resp.StatusCode, string(body))
		return TokenExpiredError{
			Token:     "refresh",
			ExpiresAt: time.Now(),
		}
	}

	var tokenResp struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    int    `json:"expires_in"`
		TokenType    string `json:"token_type"`
	}

	err = json.Unmarshal(body, &tokenResp)
	if err != nil {
		c.jsonError("token_refresh", body, err)
		return err
	}

	c.accessToken = tokenResp.AccessToken
	if tokenResp.RefreshToken != "" {
		c.refreshToken = tokenResp.RefreshToken
	}
	c.tokenExpiry = time.Now().Add(time.Duration(tokenResp.ExpiresIn) * time.Second)

	c.logf("Token refresh successful, expires at %s", c.tokenExpiry.Format(time.RFC3339))
	return nil
}

// SetRefreshToken sets the refresh token
func (c *Client) SetRefreshToken(token string) {
	c.refreshToken = token
	c.logf("Set refresh token")
}

// GetRefreshToken returns the current refresh token for persistence
func (c *Client) GetRefreshToken() string {
	return c.refreshToken
}

// IsTokenExpired checks if the access token needs refresh
func (c *Client) IsTokenExpired() bool {
	// Consider token expired 5 minutes before actual expiry for safety
	return time.Now().Add(5 * time.Minute).After(c.tokenExpiry)
}

// GetAuthToken returns the current OAuth access token
func (c *Client) GetAuthToken() string {
	return c.accessToken
}

// SetAuthToken sets the OAuth access token
func (c *Client) SetAuthToken(token string) {
	c.accessToken = token
	c.logf("Set access token")
}

// SetRateLimit configures client-side rate limiting
func (c *Client) SetRateLimit(requestsPerMinute int) {
	c.rateLimitMutex.Lock()
	defer c.rateLimitMutex.Unlock()

	c.rateLimitConfig.RealtimeDataRPM = requestsPerMinute
	c.logf("Set rate limit to %d requests per minute", requestsPerMinute)
}

// GetRateLimitStatus returns current rate limit information (placeholder)
func (c *Client) GetRateLimitStatus() (remaining int, resetTime time.Time, err error) {
	// TODO: Implement based on Tesla's actual rate limit headers
	return 60, time.Now().Add(time.Minute), nil
}

// GetAPIUsageStats returns API usage and cost information (placeholder)
func (c *Client) GetAPIUsageStats() (requestCount int, cost float64, err error) {
	// TODO: Implement based on Tesla's actual usage tracking
	return 0, 0.0, nil
}

// rateLimitWait implements client-side rate limiting
func (c *Client) rateLimitWait() {
	c.rateLimitMutex.Lock()
	defer c.rateLimitMutex.Unlock()

	// Calculate minimum interval between requests
	interval := time.Duration(60/c.rateLimitConfig.RealtimeDataRPM) * time.Second

	// Wait if necessary
	if elapsed := time.Since(c.lastRequestTime); elapsed < interval {
		waitTime := interval - elapsed
		c.logf("Rate limiting: waiting %v before next request", waitTime)
		time.Sleep(waitTime)
	}

	c.lastRequestTime = time.Now()
}

// doFleetRequest performs an HTTP request to Tesla Fleet API with authentication and rate limiting
func (c *Client) doFleetRequest(method, endpoint string, payload []byte) ([]byte, error) {
	// Rate limiting
	c.rateLimitWait()

	// Check and refresh token if needed
	if c.IsTokenExpired() {
		c.logf("Access token expired, refreshing...")
		err := c.RefreshToken()
		if err != nil {
			return nil, err
		}
	}

	// Build URL
	apiURL := FleetAPIBaseURL + endpoint

	// Create request
	var req *http.Request
	var err error

	if payload != nil {
		req, err = http.NewRequest(method, apiURL, bytes.NewBuffer(payload))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Content-Type", "application/json")
	} else {
		req, err = http.NewRequest(method, apiURL, nil)
		if err != nil {
			return nil, err
		}
	}

	// Add authorization header
	req.Header.Set("Authorization", "Bearer "+c.accessToken)
	req.Header.Set("User-Agent", "go-powerwall/v2.0")

	c.logf("Fleet API request: method=%s url=%s", method, apiURL)

	// Execute request
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	// Handle various error conditions
	switch resp.StatusCode {
	case 200, 201:
		c.logf("Fleet API request successful: status=%d", resp.StatusCode)
		return body, nil

	case 401:
		c.logf("Fleet API authentication failed: status=%d body=%s", resp.StatusCode, string(body))
		return nil, TokenExpiredError{
			Token:     "access",
			ExpiresAt: c.tokenExpiry,
		}

	case 429:
		c.logf("Fleet API rate limited: status=%d body=%s", resp.StatusCode, string(body))
		// Try to extract retry-after header
		retryAfter := 60
		if retryHeader := resp.Header.Get("Retry-After"); retryHeader != "" {
			fmt.Sscanf(retryHeader, "%d", &retryAfter)
		}
		return nil, RateLimitError{
			Endpoint:   endpoint,
			Limit:      c.rateLimitConfig.RealtimeDataRPM,
			Remaining:  0,
			ResetTime:  time.Now().Add(time.Duration(retryAfter) * time.Second),
			RetryAfter: retryAfter,
		}

	default:
		c.logf("Fleet API request failed: status=%d body=%s", resp.StatusCode, string(body))
		return nil, ApiError{
			URL:        *req.URL,
			StatusCode: resp.StatusCode,
			Body:       body,
		}
	}
}

// apiGetJson performs a GET request and unmarshals JSON response
func (c *Client) apiGetJson(endpoint string, result interface{}) error {
	respData, err := c.doFleetRequest("GET", endpoint, nil)
	if err != nil {
		return err
	}

	err = json.Unmarshal(respData, result)
	if err != nil {
		c.jsonError(endpoint, respData, err)
		return err
	}
	return nil
}

// apiPostJson performs a POST request with JSON payload and unmarshals JSON response
func (c *Client) apiPostJson(endpoint string, payload interface{}, result interface{}) error {
	var payloadData []byte
	var err error

	if payload != nil {
		payloadData, err = json.Marshal(payload)
		if err != nil {
			return err
		}
	}

	respData, err := c.doFleetRequest("POST", endpoint, payloadData)
	if err != nil {
		return err
	}

	if result != nil {
		err = json.Unmarshal(respData, result)
		if err != nil {
			c.jsonError(endpoint, respData, err)
			return err
		}
	}
	return nil
}
