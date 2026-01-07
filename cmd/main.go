// cmd is a simple command-line utility which uses the powerwall
// module to access Tesla's Fleet API for Powerwall 3.
//
// Environment variables required:
//
//	ACCESS_TOKEN  - Tesla Fleet API access token
//	REFRESH_TOKEN - Tesla Fleet API refresh token
//	CLIENT_ID     - OAuth client ID for your registered Fleet API app
//	SITE_ID       - Energy site ID (optional, will auto-select first site)
//
// Example usage:
//
//	export ACCESS_TOKEN="your-token"
//	export REFRESH_TOKEN="your-refresh-token"
//	export CLIENT_ID="your-client-id"
//
//	go run ./cmd/main.go products      # List energy sites
//	go run ./cmd/main.go status        # Real-time system status
//	go run ./cmd/main.go aggregates    # Power flow data
//
// This is mainly intended as a simple way to test the library functions and as an example of use.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/blampe/powerwall"
	"github.com/jessevdk/go-flags"
	log "github.com/sirupsen/logrus"
)

var options struct {
	Debug  bool  `long:"debug" description:"Enable debug messages"`
	SiteID int64 `long:"site-id" description:"Energy site ID (overrides SITE_ID env var)"`
	Args   struct {
		Command string   `positional-arg-name:"command" description:"Available commands: 'products', 'status', 'site_info', 'aggregates', 'soe', 'grid_status', 'telemetry_history', 'energy_history', 'backup_history', 'calendar_history', 'set_backup_reserve', 'set_storm_mode', 'set_site_name', 'operation', 'system_status', 'sitemaster', 'networks', 'grid_faults', 'meters'"`
		Args    []string `positional-arg-name:"args" description:"Arguments for command (start_date end_date [period] [timezone] for history commands, percentage for backup reserve)"`
	} `positional-args:"true" required:"true"`
}

func logDebug(v ...interface{}) {
	log.Debug(v...)
}

func logError(msg string, err error) {
	log.WithFields(log.Fields{"err": err}).Error(msg)
}

func main() {
	_, err := flags.Parse(&options)
	if err != nil {
		os.Exit(1)
	}

	if options.Debug {
		log.SetLevel(log.DebugLevel)
	}
	powerwall.SetLogFunc(logDebug)
	powerwall.SetErrFunc(logError)

	// Get credentials from environment
	accessToken := os.Getenv("ACCESS_TOKEN")
	refreshToken := os.Getenv("REFRESH_TOKEN")
	clientID := os.Getenv("CLIENT_ID")

	if accessToken == "" || refreshToken == "" || clientID == "" {
		fmt.Fprintf(os.Stderr, "Error: ACCESS_TOKEN, REFRESH_TOKEN, and CLIENT_ID environment variables are required\n\n")
		fmt.Fprintf(os.Stderr, "Environment setup:\n")
		fmt.Fprintf(os.Stderr, "  export ACCESS_TOKEN=\"your-tesla-access-token\"\n")
		fmt.Fprintf(os.Stderr, "  export REFRESH_TOKEN=\"your-tesla-refresh-token\"\n")
		fmt.Fprintf(os.Stderr, "  export CLIENT_ID=\"your-oauth-client-id\"\n")
		fmt.Fprintf(os.Stderr, "  export SITE_ID=\"your-energy-site-id\"             # optional\n\n")
		fmt.Fprintf(os.Stderr, "To obtain tokens:\n")
		fmt.Fprintf(os.Stderr, "1. Tesla mobile app developer console, or\n")
		fmt.Fprintf(os.Stderr, "2. Third-party OAuth tools like tesla-auth, or\n")
		fmt.Fprintf(os.Stderr, "3. Implement your own OAuth 2.0 PKCE flow\n\n")
		fmt.Fprintf(os.Stderr, "For help: ./cmd --help\n")
		os.Exit(2)
	}

	// Create Fleet API client
	client := powerwall.NewClient(clientID, accessToken, refreshToken)

	// Determine site ID
	siteID := options.SiteID
	if siteID == 0 {
		if envSiteID := os.Getenv("SITE_ID"); envSiteID != "" {
			siteID, err = strconv.ParseInt(envSiteID, 10, 64)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: Invalid SITE_ID environment variable: %s\n", envSiteID)
				os.Exit(2)
			}
		}
	}

	// Auto-select site if not specified
	if siteID == 0 {
		products, err := client.GetEnergyProducts()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error getting energy products: %s\n", err)
			os.Exit(2)
		}
		if len(products) == 0 {
			fmt.Fprintf(os.Stderr, "Error: No energy products found\n")
			os.Exit(2)
		}
		siteID = products[0].EnergyProductID
		fmt.Fprintf(os.Stderr, "Auto-selected energy site: %s (ID: %d)\n",
			products[0].SiteName, siteID)
	}

	// Select the site
	err = client.SelectEnergySite(siteID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error selecting energy site: %s\n", err)
		os.Exit(2)
	}

	// Execute commands
	switch options.Args.Command {
	case "products":
		result, err := client.GetEnergyProducts()
		if err != nil {
			handleError(err)
		}
		writeResult(result)

	case "status":
		result, err := client.GetStatus()
		if err != nil {
			handleError(err)
		}
		writeResult(result)

	case "site_info":
		result, err := client.GetSiteInfo()
		if err != nil {
			handleError(err)
		}
		writeResult(result)

	case "aggregates":
		result, err := client.GetMetersAggregates()
		if err != nil {
			handleError(err)
		}
		writeResult(result)

	case "soe":
		result, err := client.GetSOE()
		if err != nil {
			handleError(err)
		}
		writeResult(result)

	case "grid_status":
		result, err := client.GetGridStatus()
		if err != nil {
			handleError(err)
		}
		writeResult(result)

	case "telemetry_history":
		if len(options.Args.Args) < 2 {
			fmt.Fprintf(os.Stderr, "Error: telemetry_history requires start_date and end_date arguments\n")
			fmt.Fprintf(os.Stderr, "Example: telemetry_history 2023-12-01 2023-12-31\n")
			os.Exit(3)
		}
		startDate := options.Args.Args[0]
		endDate := options.Args.Args[1]
		var timeZone string
		if len(options.Args.Args) > 2 {
			timeZone = options.Args.Args[2]
		}
		result, err := client.GetTelemetryHistory(startDate, endDate, timeZone)
		if err != nil {
			handleError(err)
		}
		writeResult(result)

	case "energy_history":
		if len(options.Args.Args) < 3 {
			fmt.Fprintf(os.Stderr, "Error: energy_history requires start_date, end_date, and period arguments\n")
			fmt.Fprintf(os.Stderr, "Example: energy_history 2023-12-01 2023-12-31 day\n")
			os.Exit(3)
		}
		startDate := options.Args.Args[0]
		endDate := options.Args.Args[1]
		period := options.Args.Args[2]
		var timeZone string
		if len(options.Args.Args) > 3 {
			timeZone = options.Args.Args[3]
		}
		result, err := client.GetEnergyHistory(startDate, endDate, period, timeZone)
		if err != nil {
			handleError(err)
		}
		writeResult(result)

	case "backup_history":
		if len(options.Args.Args) < 3 {
			fmt.Fprintf(os.Stderr, "Error: backup_history requires start_date, end_date, and period arguments\n")
			fmt.Fprintf(os.Stderr, "Example: backup_history 2023-12-01 2023-12-31 day\n")
			os.Exit(3)
		}
		startDate := options.Args.Args[0]
		endDate := options.Args.Args[1]
		period := options.Args.Args[2]
		var timeZone string
		if len(options.Args.Args) > 3 {
			timeZone = options.Args.Args[3]
		}
		result, err := client.GetBackupHistory(startDate, endDate, period, timeZone)
		if err != nil {
			handleError(err)
		}
		writeResult(result)

	case "calendar_history":
		if len(options.Args.Args) < 4 {
			fmt.Fprintf(os.Stderr, "Error: calendar_history requires kind, start_date, end_date, and period arguments\n")
			fmt.Fprintf(os.Stderr, "Example: calendar_history energy 2023-12-01 2023-12-31 week\n")
			os.Exit(3)
		}
		kind := options.Args.Args[0]
		startDate := options.Args.Args[1]
		endDate := options.Args.Args[2]
		period := options.Args.Args[3]
		var timeZone string
		if len(options.Args.Args) > 4 {
			timeZone = options.Args.Args[4]
		}
		result, err := client.GetCalendarHistory(kind, startDate, endDate, period, timeZone)
		if err != nil {
			handleError(err)
		}
		writeResult(result)

	case "set_backup_reserve":
		if len(options.Args.Args) == 0 {
			fmt.Fprintf(os.Stderr, "Error: set_backup_reserve requires percentage argument (0-100)\n")
			os.Exit(3)
		}
		percent, err := strconv.Atoi(options.Args.Args[0])
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: invalid percentage: %s\n", options.Args.Args[0])
			os.Exit(3)
		}
		err = client.SetBackupReserve(percent)
		if err != nil {
			handleError(err)
		}
		fmt.Printf("Backup reserve set to %d%%\n", percent)

	case "set_storm_mode":
		if len(options.Args.Args) == 0 {
			fmt.Fprintf(os.Stderr, "Error: set_storm_mode requires true/false argument\n")
			os.Exit(3)
		}
		enabled := strings.ToLower(options.Args.Args[0]) == "true"
		err = client.SetStormMode(enabled)
		if err != nil {
			handleError(err)
		}
		status := "disabled"
		if enabled {
			status = "enabled"
		}
		fmt.Printf("Storm mode %s\n", status)

	case "set_site_name":
		if len(options.Args.Args) == 0 {
			fmt.Fprintf(os.Stderr, "Error: set_site_name requires name argument\n")
			os.Exit(3)
		}
		name := strings.Join(options.Args.Args, " ")
		err = client.SetSiteName(name)
		if err != nil {
			handleError(err)
		}
		fmt.Printf("Site name set to: %s\n", name)

	// Test unsupported operations
	case "operation":
		result, err := client.GetOperation()
		if err != nil {
			handleError(err)
		}
		writeResult(result)

	case "system_status":
		result, err := client.GetSystemStatus()
		if err != nil {
			handleError(err)
		}
		writeResult(result)

	case "sitemaster":
		result, err := client.GetSitemaster()
		if err != nil {
			handleError(err)
		}
		writeResult(result)

	case "networks":
		result, err := client.GetNetworks()
		if err != nil {
			handleError(err)
		}
		writeResult(result)

	case "grid_faults":
		result, err := client.GetGridFaults()
		if err != nil {
			handleError(err)
		}
		writeResult(result)

	case "meters":
		category := "site"
		if len(options.Args.Args) > 0 {
			category = options.Args.Args[0]
		}
		result, err := client.GetMeters(category)
		if err != nil {
			handleError(err)
		}
		writeResult(result)

	default:
		fmt.Fprintf(os.Stderr, "Error: Unknown command: %v\n", options.Args.Command)
		fmt.Fprintf(os.Stderr, "\nAvailable commands:\n")
		fmt.Fprintf(os.Stderr, "\nCore API:\n")
		fmt.Fprintf(os.Stderr, "  products                      - List energy products/sites\n")
		fmt.Fprintf(os.Stderr, "  status                        - Real-time system status\n")
		fmt.Fprintf(os.Stderr, "  site_info                     - Site configuration\n")
		fmt.Fprintf(os.Stderr, "  aggregates                    - Power flow data\n")
		fmt.Fprintf(os.Stderr, "  soe                           - Battery state of energy\n")
		fmt.Fprintf(os.Stderr, "  grid_status                   - Grid connection status\n")
		fmt.Fprintf(os.Stderr, "\nHistorical Data:\n")
		fmt.Fprintf(os.Stderr, "  power_history [period]        - Power history (day,week)\n")
		fmt.Fprintf(os.Stderr, "  energy_history [period]       - Energy history (day,week,month,year,lifetime)\n")
		fmt.Fprintf(os.Stderr, "  calendar_history <date> <period> - Historical data by date (YYYY-MM-DD)\n")
		fmt.Fprintf(os.Stderr, "\nControl Commands:\n")
		fmt.Fprintf(os.Stderr, "  set_backup_reserve <percent>  - Set backup reserve percentage (0-100)\n")
		fmt.Fprintf(os.Stderr, "  set_storm_mode <true|false>   - Enable/disable Storm Watch\n")
		fmt.Fprintf(os.Stderr, "  set_site_name <name>          - Set site display name\n")
		fmt.Fprintf(os.Stderr, "\nUnsupported (will show errors):\n")
		fmt.Fprintf(os.Stderr, "  operation, system_status, sitemaster, networks, grid_faults, meters\n")
		os.Exit(3)
	}
}

func handleError(err error) {
	// Handle different error types with appropriate messages
	switch e := err.(type) {
	case powerwall.UnsupportedError:
		fmt.Fprintf(os.Stderr, "Unsupported operation: %s\n", e.Reason)
		os.Exit(4)
	case powerwall.TokenExpiredError:
		fmt.Fprintf(os.Stderr, "Token expired: %s\n", e.Error())
		fmt.Fprintf(os.Stderr, "Please refresh your ACCESS_TOKEN and REFRESH_TOKEN\n")
		os.Exit(5)
	case powerwall.RateLimitError:
		fmt.Fprintf(os.Stderr, "Rate limit exceeded: %s\n", e.Error())
		os.Exit(6)
	default:
		fmt.Fprintf(os.Stderr, "API error: %s\n", err.Error())
		os.Exit(1)
	}
}

func writeResult(value interface{}) {
	b, err := json.MarshalIndent(value, "", "    ")
	if err != nil {
		panic(err)
	}
	fmt.Println(string(b))
}
