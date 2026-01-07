package powerwall

import (
	"fmt"
	"net/url"
	"time"
)

// ApiError indicates that something unexpected occurred with the HTTP API
// call.  This usually occurs when the endpoint returns an unexpected status
// code.
type ApiError struct {
	URL        url.URL
	StatusCode int
	Body       []byte
}

func (e ApiError) Error() string {
	return fmt.Sprintf("API call to %s returned unexpected status code %d (%#v)", e.URL.String(), e.StatusCode, string(e.Body))
}

// AuthFailure is returned when the client was unable to perform a request
// because it was not able to login using the provided email and password.
type AuthFailure struct {
	URL       url.URL
	ErrorText string
	Message   string
}

func (e AuthFailure) Error() string {
	return fmt.Sprintf("Authentication Failed: %s (%s)", e.ErrorText, e.Message)
}

// Fleet API specific error types

// TokenExpiredError indicates that the OAuth token has expired and needs refresh
type TokenExpiredError struct {
	Token     string    `json:"token_type"` // "access" or "refresh"
	ExpiresAt time.Time `json:"expires_at"`
}

func (e TokenExpiredError) Error() string {
	return fmt.Sprintf("OAuth token expired: %s token expired at %s", e.Token, e.ExpiresAt.Format(time.RFC3339))
}

// RateLimitError indicates that the Fleet API rate limit has been exceeded
type RateLimitError struct {
	Endpoint   string    `json:"endpoint"`
	Limit      int       `json:"limit"`
	Remaining  int       `json:"remaining"`
	ResetTime  time.Time `json:"reset_time"`
	RetryAfter int       `json:"retry_after"` // Seconds
}

func (e RateLimitError) Error() string {
	return fmt.Sprintf("Rate limit exceeded for %s: %d/%d remaining, resets at %s (retry after %ds)",
		e.Endpoint, e.Remaining, e.Limit, e.ResetTime.Format(time.RFC3339), e.RetryAfter)
}

// UnsupportedError indicates that a requested operation is not available in Fleet API
type UnsupportedError struct {
	Operation string
	Reason    string
}

func (e UnsupportedError) Error() string {
	return fmt.Sprintf("Operation '%s' not supported: %s", e.Operation, e.Reason)
}

// EnergyProductError indicates an error specific to an energy product/site
type EnergyProductError struct {
	EnergyProductID int64  `json:"energy_site_id"`
	ErrorType       string `json:"error"`
	Message         string `json:"message"`
}

func (e EnergyProductError) Error() string {
	return fmt.Sprintf("Energy site %d error (%s): %s", e.EnergyProductID, e.ErrorType, e.Message)
}
