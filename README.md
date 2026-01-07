[![Go Reference](https://pkg.go.dev/badge/github.com/blampe/powerwall.svg)](https://pkg.go.dev/github.com/blampe/powerwall)

# powerwall

A Go library for communicating with Tesla Powerwall 3 appliances via Tesla's
Fleet API.

---

**Note:** This library uses Tesla's Fleet API, which requires proper OAuth
authentication and may have usage limits. All of the APIs involved don't
currently incur charges, but Tesla's APIs are subject to change at any time.

---

This library allows clients to easily pull real-time monitoring data,
historical energy data, and send control commands to Tesla Powerwall 3 systems
via Tesla's cloud-based Fleet API. This is **not** the local gateway API, but
rather the official Tesla Fleet API that provides alternative functionality and
multi-site support.

## Prerequisites

To use this library, you need:

1. **Tesla Fleet API Access**: Register for Tesla Fleet API access and obtain OAuth credentials ([myteslamate](https://www.myteslamate.com/tesla-api-application-registration/))
2. **Access Tokens**: Valid `ACCESS_TOKEN` and `REFRESH_TOKEN` from Tesla OAuth flow
3. **Client ID**: Your Fleet API `CLIENT_ID` (from registered OAuth app)
4. **Energy Site ID**: Your Powerwall site ID (optional - library can auto-detect)

## Basic Usage

Create a client instance using OAuth tokens and start making API calls:

```go
import (
	"fmt"
	"github.com/blampe/powerwall"
)

func main() {
	// Create Fleet API client with OAuth tokens
	client := powerwall.NewClient("your-client-id", "your-access-token", "your-refresh-token")

	// Get available energy sites
	products, err := client.GetEnergyProducts()
	if err != nil {
		panic(err)
	}

	// Select the first energy site
	if len(products) > 0 {
		err = client.SelectEnergySite(products[0].EnergyProductID)
		if err != nil {
			panic(err)
		}

		// Get real-time status
		status, err := client.GetStatus()
		if err != nil {
			panic(err)
		}

		fmt.Printf("Site: %s\nSystem timestamp: %s\n",
			products[0].SiteName, status.StartTime.Time.Format("2006-01-02 15:04:05"))

		// Get current battery level
		soe, err := client.GetSOE()
		if err != nil {
			panic(err)
		}

		fmt.Printf("Battery charge: %.1f%%\n", soe.Percentage)
	}
}
```

## Environment Variables

The CLI tool uses these environment variables for configuration:

```bash
export ACCESS_TOKEN="your-tesla-access-token"
export REFRESH_TOKEN="your-tesla-refresh-token"
export CLIENT_ID="your-oauth-client-id"
export SITE_ID="your-energy-site-id"             # optional, will auto-select first site
```

## Command Line Tool

A command-line utility is included for testing and scripting:

```bash
# Build the CLI tool
go build -o powerwall-cmd ./cmd

# List energy sites
./powerwall-cmd products

# Get real-time power flows
./powerwall-cmd aggregates

# Get battery state of charge
./powerwall-cmd soe

# Get historical energy data (5-minute intervals)
./powerwall-cmd energy_history 2024-01-01T00:00:00Z 2024-01-02T00:00:00Z day

# Set backup reserve to 20%
./powerwall-cmd set_backup_reserve 20

# Enable Storm Watch mode
./powerwall-cmd set_storm_mode true
```

## Available API Methods

### Real-time Monitoring
- `GetStatus()` - System status and timestamps
- `GetSiteInfo()` - Installation details and configuration
- `GetMetersAggregates()` - Live power flows (solar, battery, grid, load)
- `GetSOE()` - Battery state of energy (charge percentage)
- `GetGridStatus()` - Grid connection status

### Multi-site Management
- `GetProducts()` - List all products (vehicles + energy sites)
- `GetEnergyProducts()` - List energy sites only
- `SelectEnergySite(id)` - Set active site for subsequent calls

### Historical Data
- `GetEnergyHistory(start, end, period)` - Energy totals with 5-minute granularity
- `GetBackupHistory(start, end, period)` - Backup/outage events
- `GetTelemetryHistory(start, end)` - Charge telemetry data
- `GetCalendarHistory(kind, start, end, period)` - Generic historical data

### Control Commands
- `SetBackupReserve(percent)` - Set backup reserve percentage (0-100)
- `SetStormMode(enabled)` - Enable/disable Storm Watch mode
- `SetSiteName(name)` - Change site display name

## Authentication and Token Management

The client automatically handles OAuth token refresh when needed:

```go
client := powerwall.NewClient(accessToken, refreshToken, clientID)

// Client will automatically refresh tokens when they expire
// Access refreshed tokens if needed:
newAccessToken := client.GetAccessToken()
newRefreshToken := client.GetRefreshToken()
```

## Rate Limiting

The Fleet API has built-in rate limiting. The client automatically handles rate limits with appropriate delays:

- **Real-time data**: 60 requests per minute
- **Control commands**: 30 requests per minute

## Error Handling

The library provides specific error types for different scenarios:

```go
result, err := client.GetStatus()
if err != nil {
	switch e := err.(type) {
	case powerwall.TokenExpiredError:
		// Token expired and refresh failed
		fmt.Println("Authentication failed:", e.Message)
	case powerwall.RateLimitError:
		// Rate limit exceeded
		fmt.Println("Rate limited, retry after:", e.RetryAfter)
	case powerwall.UnsupportedError:
		// Feature not available via Fleet API
		fmt.Println("Unsupported operation:", e.Reason)
	case powerwall.ApiError:
		// General API error
		fmt.Printf("API error %d: %s\n", e.StatusCode, e.Message)
	default:
		// Network or other error
		fmt.Println("Error:", err)
	}
}
```

## Historical Data Examples

Get detailed energy data with 5-minute resolution:

```go
// Get last week's daily energy totals
endDate := time.Now().Format("2006-01-02T15:04:05Z")
startDate := time.Now().AddDate(0, 0, -7).Format("2006-01-02T15:04:05Z")
history, err := client.GetEnergyHistory(startDate, endDate, "day")

if err == nil {
	for _, point := range history.TimeSeries {
		fmt.Printf("%s: Solar: %dWh, Grid: %dWh, Battery: %dWh\n",
			point.Timestamp.Format("2006-01-02 15:04"),
			point.SolarEnergyExported,
			point.GridEnergyImported,
			point.BatteryEnergyExported)
	}
}
```

## Logging

Enable debug logging to troubleshoot API calls:

```go
import log "github.com/sirupsen/logrus"

func logDebug(v ...interface{}) {
	log.Debug(v...)
}

func logError(msg string, err error) {
	log.WithFields(log.Fields{"err": err}).Error(msg)
}

func main() {
	log.SetLevel(log.DebugLevel)
	powerwall.SetLogFunc(logDebug)
	powerwall.SetErrFunc(logError)

	// Your code here...
}
```

## Unsupported Features

Some features from the legacy local gateway API are not available via Tesla's Fleet API:

- Detailed system diagnostics (`GetSystemStatus`)
- Local network configuration (`GetNetworks`)
- Individual meter details (`GetMeters`)
- Grid fault information (`GetGridFaults`)
- Sitemaster clustering data (`GetSitemaster`)
- Low-level operation data (`GetOperation`)

These methods will return `UnsupportedError` when called.

## Contributing

Pull requests are welcome! The Tesla Fleet API is extensive and this library doesn't yet support all available endpoints. Areas for contribution:

- Additional historical data endpoints
- Vehicle integration (if you have both Powerwall and Tesla vehicle)
- Enhanced error handling and retry logic
- Additional control commands as they become available

## License

This library is provided as-is. Tesla's Fleet API terms of service apply to all usage.
