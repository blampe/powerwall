// Fleet API implementation of all Powerwall methods
//
// Core API methods:
//	(*Client) GetStatus() - Enhanced real-time data
//	(*Client) GetSiteInfo() - Site configuration
//	(*Client) GetMetersAggregates() - Power flow data
//	(*Client) GetSOE() - Battery state of energy
//	(*Client) GetGridStatus() - Grid connection status
//
// Device management:
//	(*Client) GetProducts() - List all products
//	(*Client) GetEnergyProducts() - List energy sites only
//	(*Client) SelectEnergySite() - Set active site
//
// Historical data (WORKING):
//	(*Client) GetEnergyHistory() - Energy data via calendar_history endpoint
//	(*Client) GetBackupHistory() - Backup events via calendar_history endpoint
//	(*Client) GetTelemetryHistory() - Telemetry data via telemetry_history endpoint
//	(*Client) GetCalendarHistory() - Generic calendar_history endpoint
//
// Control commands (WORKING):
//	(*Client) SetBackupReserve() - Set backup percentage
//	(*Client) SetStormMode() - Enable/disable Storm Watch
//	(*Client) SetSiteName() - Rename site

package powerwall

import (
	"fmt"
	"net/url"
	"time"
)

///////////////////////////////////////////////////////////////////////////////
// Device Management

// GetProducts returns all products (vehicles + energy sites) associated with the account
func (c *Client) GetProducts() ([]EnergyProduct, error) {
	c.logf("Fetching all products...")

	var resp ProductsResponse
	err := c.apiGetJson("/api/1/products", &resp)
	if err != nil {
		return nil, err
	}

	c.logf("Found %d total products", len(resp.Response))
	return resp.Response, nil
}

// GetEnergyProducts returns only energy sites (filtering out vehicles)
func (c *Client) GetEnergyProducts() ([]EnergyProduct, error) {
	products, err := c.GetProducts()
	if err != nil {
		return nil, err
	}

	// Filter to only energy products (not vehicles)
	var energyProducts []EnergyProduct
	for _, product := range products {
		if product.ResourceType == "solar" || product.ResourceType == "battery" {
			energyProducts = append(energyProducts, product)
		}
	}

	c.logf("Found %d energy products (filtered from %d total)", len(energyProducts), len(products))
	return energyProducts, nil
}

// SelectEnergySite sets the active energy site for subsequent API calls
func (c *Client) SelectEnergySite(energySiteID int64) error {
	c.selectedSiteID = energySiteID
	c.logf("Selected energy site ID: %d", energySiteID)
	return nil
}

// GetSelectedEnergySite returns the currently selected energy site ID
func (c *Client) GetSelectedEnergySite() int64 {
	return c.selectedSiteID
}

// ensureSiteSelected checks that an energy site is selected and returns an error if not
func (c *Client) ensureSiteSelected() error {
	if c.selectedSiteID == 0 {
		return fmt.Errorf("no energy site selected - call SelectEnergySite() first")
	}
	return nil
}

///////////////////////////////////////////////////////////////////////////////
// Status API - Enhanced with Fleet API live_status data

// GetStatus returns enhanced system status from Fleet API live_status endpoint.
// This provides much richer real-time data than the local gateway status endpoint.
func (c *Client) GetStatus() (*StatusData, error) {
	if err := c.ensureSiteSelected(); err != nil {
		return nil, err
	}

	c.logf("Fetching live status for energy site %d...", c.selectedSiteID)

	var liveStatus LiveStatusResponse
	endpoint := fmt.Sprintf("/api/1/energy_sites/%d/live_status", c.selectedSiteID)
	err := c.apiGetJson(endpoint, &liveStatus)
	if err != nil {
		return nil, err
	}

	// Map Fleet API live_status to StatusData structure
	// Note: Many fields from local gateway are not available via Fleet API
	status := &StatusData{
		Din:              fmt.Sprintf("fleet-api-%d", c.selectedSiteID), // Fleet API doesn't provide DIN
		StartTime:        NonIsoTime{liveStatus.Response.Timestamp},     // Use data timestamp
		UpTime:           Duration{0},                                   // Not available via Fleet API
		IsNew:            false,                                         // Not available via Fleet API
		Version:          "fleet-api",                                   // Fleet API doesn't provide version
		GitHash:          "",                                            // Not available via Fleet API
		CommissionCount:  0,                                             // Not available via Fleet API
		DeviceType:       "powerwall",                                   // Inferred from energy site
		SyncType:         "",                                            // Not available via Fleet API
		Leader:           "",                                            // Not available via Fleet API
		Followers:        nil,                                           // Not available via Fleet API
		CellularDisabled: false,                                         // Not available via Fleet API
	}

	c.logf("Live status retrieved successfully")
	return status, nil
}

///////////////////////////////////////////////////////////////////////////////
// Site Info API - Enhanced with Fleet API site_info data

// GetSiteInfo returns enhanced site information from Fleet API site_info endpoint.
// This provides installation details and configuration not available from local gateway.
func (c *Client) GetSiteInfo() (*SiteInfoData, error) {
	if err := c.ensureSiteSelected(); err != nil {
		return nil, err
	}

	c.logf("Fetching site info for energy site %d...", c.selectedSiteID)

	var siteInfo SiteInfoResponse
	endpoint := fmt.Sprintf("/api/1/energy_sites/%d/site_info", c.selectedSiteID)
	err := c.apiGetJson(endpoint, &siteInfo)
	if err != nil {
		return nil, err
	}

	// Map Fleet API site_info to SiteInfoData structure
	info := &SiteInfoData{
		SiteName:               siteInfo.Response.SiteName,
		TimeZone:               siteInfo.Response.InstallationTimeZone,
		MaxSiteMeterPowerKW:    int(siteInfo.Response.MaxSiteMeterPowerAC / 1000), // Convert W to kW
		MinSiteMeterPowerKW:    int(siteInfo.Response.MinSiteMeterPowerAC / 1000), // Convert W to kW
		MeasuredFrequency:      0,                                                 // Not available via Fleet API
		MaxSystemEnergyKWH:     0,                                                 // Not available via Fleet API
		MaxSystemPowerKW:       float32(siteInfo.Response.NameplatePower) / 1000,  // Convert W to kW
		NominalSystemEnergyKWH: 0,                                                 // Not available via Fleet API
		NominalSystemPowerKW:   float32(siteInfo.Response.NameplatePower) / 1000,  // Convert W to kW
		// Map available grid/utility info
		GridData: struct {
			GridCode           string `json:"grid_code"`
			GridVoltageSetting int    `json:"grid_voltage_setting"`
			GridFreqSetting    int    `json:"grid_freq_setting"`
			GridPhaseSetting   string `json:"grid_phase_setting"`
			Country            string `json:"country"`
			State              string `json:"state"`
			Distributor        string `json:"distributor"`
			Utility            string `json:"utility"`
			Retailer           string `json:"retailer"`
			Region             string `json:"region"`
		}{
			Utility: siteInfo.Response.Utility,
			// Other fields not available from Fleet API
		},
	}

	c.logf("Site info retrieved successfully: %s", info.SiteName)
	return info, nil
}

///////////////////////////////////////////////////////////////////////////////
// Meters/Aggregates API - Power flow data from live_status

// GetMetersAggregates returns real-time power flow data from Fleet API live_status.
// This provides similar data to local gateway meters/aggregates but from cloud API.
func (c *Client) GetMetersAggregates() (map[string]MeterAggregatesData, error) {
	if err := c.ensureSiteSelected(); err != nil {
		return nil, err
	}

	c.logf("Fetching power flow data for energy site %d...", c.selectedSiteID)

	var liveStatus LiveStatusResponse
	endpoint := fmt.Sprintf("/api/1/energy_sites/%d/live_status", c.selectedSiteID)
	err := c.apiGetJson(endpoint, &liveStatus)
	if err != nil {
		return nil, err
	}

	result := make(map[string]MeterAggregatesData)

	// Map Fleet API power values to MeterAggregatesData format
	// Note: Fleet API provides instant power values, not the full meter data structure

	if liveStatus.Response.SolarPower != nil {
		result["solar"] = MeterAggregatesData{
			LastCommunicationTime: liveStatus.Response.Timestamp,
			InstantPower:          float32(*liveStatus.Response.SolarPower),
			// Other fields not available from Fleet API
		}
	}

	if liveStatus.Response.BatteryPower != nil {
		result["battery"] = MeterAggregatesData{
			LastCommunicationTime: liveStatus.Response.Timestamp,
			InstantPower:          float32(*liveStatus.Response.BatteryPower),
		}
	}

	if liveStatus.Response.GridPower != nil {
		result["site"] = MeterAggregatesData{
			LastCommunicationTime: liveStatus.Response.Timestamp,
			InstantPower:          float32(*liveStatus.Response.GridPower),
		}
	}

	if liveStatus.Response.LoadPower != nil {
		result["load"] = MeterAggregatesData{
			LastCommunicationTime: liveStatus.Response.Timestamp,
			InstantPower:          float32(*liveStatus.Response.LoadPower),
		}
	}

	c.logf("Power flow data retrieved successfully for %d categories", len(result))
	return result, nil
}

///////////////////////////////////////////////////////////////////////////////
// State of Energy (SOE) API

// GetSOE returns battery state of energy from Fleet API live_status.
func (c *Client) GetSOE() (*SOEData, error) {
	if err := c.ensureSiteSelected(); err != nil {
		return nil, err
	}

	c.logf("Fetching battery state of energy for energy site %d...", c.selectedSiteID)

	var liveStatus LiveStatusResponse
	endpoint := fmt.Sprintf("/api/1/energy_sites/%d/live_status", c.selectedSiteID)
	err := c.apiGetJson(endpoint, &liveStatus)
	if err != nil {
		return nil, err
	}

	soe := &SOEData{
		Percentage: 0,
	}

	if liveStatus.Response.PercentageCharged != nil {
		soe.Percentage = *liveStatus.Response.PercentageCharged
	}

	c.logf("Battery SOE retrieved successfully: %.1f%%", soe.Percentage)
	return soe, nil
}

///////////////////////////////////////////////////////////////////////////////
// Grid Status API

// GetGridStatus returns grid connection status from Fleet API live_status.
func (c *Client) GetGridStatus() (*GridStatusData, error) {
	if err := c.ensureSiteSelected(); err != nil {
		return nil, err
	}

	c.logf("Fetching grid status for energy site %d...", c.selectedSiteID)

	var liveStatus LiveStatusResponse
	endpoint := fmt.Sprintf("/api/1/energy_sites/%d/live_status", c.selectedSiteID)
	err := c.apiGetJson(endpoint, &liveStatus)
	if err != nil {
		return nil, err
	}

	gridStatus := &GridStatusData{
		GridStatus:         liveStatus.Response.GridStatus,
		GridServicesActive: liveStatus.Response.GridServicesActive,
	}

	c.logf("Grid status retrieved successfully: grid=%s services=%t",
		gridStatus.GridStatus, gridStatus.GridServicesActive)
	return gridStatus, nil
}

///////////////////////////////////////////////////////////////////////////////
// Historical Data - NEW FUNCTIONALITY

// GetTelemetryHistory retrieves historical telemetry data including charge information.
// startDate and endDate define the date range in YYYY-MM-DD format.
// timeZone specifies the timezone (optional, defaults to site timezone).
// This provides charge telemetry data over the specified time period.
func (c *Client) GetTelemetryHistory(startDate, endDate string, timeZone ...string) (*HistoryData, error) {
	if err := c.ensureSiteSelected(); err != nil {
		return nil, err
	}

	c.logf("Fetching telemetry history for energy site %d, start=%s end=%s...", c.selectedSiteID, startDate, endDate)

	// Build endpoint with query parameters
	endpoint := fmt.Sprintf("/api/1/energy_sites/%d/telemetry_history", c.selectedSiteID)
	params := url.Values{}
	params.Set("kind", "charge")
	params.Set("start_date", startDate)
	params.Set("end_date", endDate)

	// Add timezone if provided
	if len(timeZone) > 0 && timeZone[0] != "" {
		params.Set("time_zone", timeZone[0])
	}

	fullEndpoint := endpoint + "?" + params.Encode()

	var historyResponse struct {
		Response HistoryData `json:"response"`
	}

	err := c.apiGetJson(fullEndpoint, &historyResponse)
	if err != nil {
		return nil, err
	}

	c.logf("Telemetry history retrieved successfully: %d data points from %s to %s",
		len(historyResponse.Response.TimeSeries), startDate, endDate)

	return &historyResponse.Response, nil
}

// GetEnergyHistory retrieves historical energy data using calendar_history endpoint.
// startDate and endDate define the date range in YYYY-MM-DD format.
// period specifies the granularity: "day", "week", "month", "year".
// timeZone specifies the timezone (optional, defaults to site timezone).
// This provides energy import/export totals for solar, battery, grid over time.
func (c *Client) GetEnergyHistory(startDate, endDate, period string, timeZone ...string) (*HistoryData, error) {
	if err := c.ensureSiteSelected(); err != nil {
		return nil, err
	}

	c.logf("Fetching energy history for energy site %d, start=%s end=%s period=%s...", c.selectedSiteID, startDate, endDate, period)

	// Validate period
	validPeriods := map[string]bool{
		"day":   true,
		"week":  true,
		"month": true,
		"year":  true,
	}

	if !validPeriods[period] {
		return nil, fmt.Errorf("invalid period for energy history: %s (supported: day, week, month, year)", period)
	}

	// Build endpoint with query parameters using calendar_history
	endpoint := fmt.Sprintf("/api/1/energy_sites/%d/calendar_history", c.selectedSiteID)
	params := url.Values{}
	params.Set("kind", "energy")
	params.Set("start_date", startDate)
	params.Set("end_date", endDate)
	params.Set("period", period)

	// Add timezone if provided
	if len(timeZone) > 0 && timeZone[0] != "" {
		params.Set("time_zone", timeZone[0])
	}

	fullEndpoint := endpoint + "?" + params.Encode()

	var historyResponse struct {
		Response HistoryData `json:"response"`
	}

	err := c.apiGetJson(fullEndpoint, &historyResponse)
	if err != nil {
		return nil, err
	}

	c.logf("Energy history retrieved successfully: %d data points from %s to %s (%s periods)",
		len(historyResponse.Response.TimeSeries), startDate, endDate, period)

	return &historyResponse.Response, nil
}

// GetBackupHistory retrieves historical backup data using calendar_history endpoint.
// startDate and endDate define the date range in YYYY-MM-DD format.
// period specifies the granularity: "day", "week", "month", "year".
// timeZone specifies the timezone (optional, defaults to site timezone).
// This provides backup usage and outage information over time.
func (c *Client) GetBackupHistory(startDate, endDate, period string, timeZone ...string) (*HistoryData, error) {
	if err := c.ensureSiteSelected(); err != nil {
		return nil, err
	}

	c.logf("Fetching backup history for energy site %d, start=%s end=%s period=%s...", c.selectedSiteID, startDate, endDate, period)

	// Validate period
	validPeriods := map[string]bool{
		"day":   true,
		"week":  true,
		"month": true,
		"year":  true,
	}

	if !validPeriods[period] {
		return nil, fmt.Errorf("invalid period for backup history: %s (supported: day, week, month, year)", period)
	}

	// Build endpoint with query parameters using calendar_history
	endpoint := fmt.Sprintf("/api/1/energy_sites/%d/calendar_history", c.selectedSiteID)
	params := url.Values{}
	params.Set("kind", "backup")
	params.Set("start_date", startDate)
	params.Set("end_date", endDate)
	params.Set("period", period)

	// Add timezone if provided
	if len(timeZone) > 0 && timeZone[0] != "" {
		params.Set("time_zone", timeZone[0])
	}

	fullEndpoint := endpoint + "?" + params.Encode()

	var historyResponse struct {
		Response HistoryData `json:"response"`
	}

	err := c.apiGetJson(fullEndpoint, &historyResponse)
	if err != nil {
		return nil, err
	}

	c.logf("Backup history retrieved successfully: %d data points from %s to %s (%s periods)",
		len(historyResponse.Response.TimeSeries), startDate, endDate, period)

	return &historyResponse.Response, nil
}

// GetCalendarHistory retrieves historical data for a specific date range using calendar_history endpoint.
// kind specifies the data type: "energy" or "backup".
// GetCalendarHistory retrieves historical data for a specific date range using calendar_history endpoint.
// kind specifies the data type: "energy" or "backup".
// startDate and endDate define the date range in YYYY-MM-DD format.
// period specifies the granularity: "day", "week", "month", "year".
// timeZone specifies the timezone (optional, defaults to site timezone).
func (c *Client) GetCalendarHistory(kind, startDate, endDate, period string, timeZone ...string) (*HistoryData, error) {
	if err := c.ensureSiteSelected(); err != nil {
		return nil, err
	}

	c.logf("Fetching calendar history for energy site %d, kind=%s start=%s end=%s period=%s...",
		c.selectedSiteID, kind, startDate, endDate, period)

	// Validate kind
	validKinds := map[string]bool{
		"energy": true,
		"backup": true,
	}

	if !validKinds[kind] {
		return nil, fmt.Errorf("invalid kind for calendar history: %s (supported: energy, backup)", kind)
	}

	// Validate period
	validPeriods := map[string]bool{
		"day":   true,
		"week":  true,
		"month": true,
		"year":  true,
	}

	if !validPeriods[period] {
		return nil, fmt.Errorf("invalid period for calendar history: %s (supported: day, week, month, year)", period)
	}

	// Build endpoint with query parameters
	endpoint := fmt.Sprintf("/api/1/energy_sites/%d/calendar_history", c.selectedSiteID)
	params := url.Values{}
	params.Set("kind", kind)
	params.Set("start_date", startDate)
	params.Set("end_date", endDate)
	params.Set("period", period)

	// Add timezone if provided
	if len(timeZone) > 0 && timeZone[0] != "" {
		params.Set("time_zone", timeZone[0])
	}

	fullEndpoint := endpoint + "?" + params.Encode()

	var historyResponse struct {
		Response HistoryData `json:"response"`
	}

	err := c.apiGetJson(fullEndpoint, &historyResponse)
	if err != nil {
		return nil, err
	}

	c.logf("Calendar history retrieved successfully: %d data points for %s from %s to %s (%s periods)",
		len(historyResponse.Response.TimeSeries), kind, startDate, endDate, period)

	return &historyResponse.Response, nil
}

///////////////////////////////////////////////////////////////////////////////
// Control Commands - NEW FUNCTIONALITY

// SetBackupReserve sets the battery backup reserve percentage (0-100).
// This determines how much battery capacity is reserved for backup power during outages.
func (c *Client) SetBackupReserve(percent int) error {
	if err := c.ensureSiteSelected(); err != nil {
		return err
	}

	if percent < 0 || percent > 100 {
		return fmt.Errorf("backup reserve percent must be between 0 and 100, got %d", percent)
	}

	c.logf("Setting backup reserve to %d%% for energy site %d...", percent, c.selectedSiteID)

	payload := map[string]interface{}{
		"backup_reserve_percent": percent,
	}

	endpoint := fmt.Sprintf("/api/1/energy_sites/%d/backup", c.selectedSiteID)

	var response map[string]interface{}
	err := c.apiPostJson(endpoint, payload, &response)
	if err != nil {
		return err
	}

	c.logf("Backup reserve set successfully to %d%%", percent)
	return nil
}

// SetSiteName changes the display name of the energy site.
func (c *Client) SetSiteName(name string) error {
	if err := c.ensureSiteSelected(); err != nil {
		return err
	}

	if name == "" {
		return fmt.Errorf("site name cannot be empty")
	}

	c.logf("Setting site name to '%s' for energy site %d...", name, c.selectedSiteID)

	payload := map[string]interface{}{
		"site_name": name,
	}

	endpoint := fmt.Sprintf("/api/1/energy_sites/%d/site_name", c.selectedSiteID)

	var response map[string]interface{}
	err := c.apiPostJson(endpoint, payload, &response)
	if err != nil {
		return err
	}

	c.logf("Site name set successfully to '%s'", name)
	return nil
}

// SetStormMode enables or disables Storm Watch mode.
// When enabled, the system will charge to 100% and prepare for potential outages.
func (c *Client) SetStormMode(enabled bool) error {
	if err := c.ensureSiteSelected(); err != nil {
		return err
	}

	c.logf("Setting Storm Watch mode to %t for energy site %d...", enabled, c.selectedSiteID)

	payload := map[string]interface{}{
		"enabled": enabled,
	}

	endpoint := fmt.Sprintf("/api/1/energy_sites/%d/storm_mode", c.selectedSiteID)

	var response map[string]interface{}
	err := c.apiPostJson(endpoint, payload, &response)
	if err != nil {
		return err
	}

	status := "disabled"
	if enabled {
		status = "enabled"
	}
	c.logf("Storm Watch mode %s successfully", status)
	return nil
}

///////////////////////////////////////////////////////////////////////////////
// Unsupported methods - return appropriate errors

// GetOperation returns limited operation data - many local gateway features unavailable
func (c *Client) GetOperation() (*OperationData, error) {
	return nil, UnsupportedError{
		Operation: "GetOperation",
		Reason:    "Fleet API does not provide detailed operation data available from local gateway",
	}
}

// GetSystemStatus returns limited system status - diagnostics not available via Fleet API
func (c *Client) GetSystemStatus() (*SystemStatusData, error) {
	return nil, UnsupportedError{
		Operation: "GetSystemStatus",
		Reason:    "Fleet API does not provide detailed system diagnostics available from local gateway",
	}
}

// GetSitemaster is not available via Fleet API - local gateway clustering only
func (c *Client) GetSitemaster() (*SitemasterData, error) {
	return nil, UnsupportedError{
		Operation: "GetSitemaster",
		Reason:    "Fleet API does not expose local gateway clustering information",
	}
}

// GetNetworks is not available via Fleet API - local network config only
func (c *Client) GetNetworks() ([]NetworkData, error) {
	return nil, UnsupportedError{
		Operation: "GetNetworks",
		Reason:    "Fleet API does not expose local network configuration",
	}
}

// GetGridFaults is not available via Fleet API - detailed diagnostics only on local gateway
func (c *Client) GetGridFaults() ([]GridFaultData, error) {
	return nil, UnsupportedError{
		Operation: "GetGridFaults",
		Reason:    "Fleet API does not provide detailed grid fault information available from local gateway",
	}
}

// GetMeters with specific category is not available - Fleet API consolidates into live_status
func (c *Client) GetMeters(category string) ([]MeterData, error) {
	return nil, UnsupportedError{
		Operation: "GetMeters",
		Reason:    "Fleet API does not provide individual meter details - use GetMetersAggregates() instead",
	}
}

///////////////////////////////////////////////////////////////////////////////
// Convenience methods for common operations

// GetRecentTelemetryData is a convenience method to get the last 7 days of telemetry data
func (c *Client) GetRecentTelemetryData() (*HistoryData, error) {
	endDate := time.Now().Format("2006-01-02")
	startDate := time.Now().AddDate(0, 0, -7).Format("2006-01-02")
	return c.GetTelemetryHistory(startDate, endDate)
}

// GetWeeklyEnergyData is a convenience method to get weekly energy totals for the past month
func (c *Client) GetWeeklyEnergyData() (*HistoryData, error) {
	endDate := time.Now().Format("2006-01-02")
	startDate := time.Now().AddDate(0, -1, 0).Format("2006-01-02")
	return c.GetEnergyHistory(startDate, endDate, "week")
}

// GetMonthlyEnergyData is a convenience method to get monthly energy totals for the past year
func (c *Client) GetMonthlyEnergyData() (*HistoryData, error) {
	endDate := time.Now().Format("2006-01-02")
	startDate := time.Now().AddDate(-1, 0, 0).Format("2006-01-02")
	return c.GetEnergyHistory(startDate, endDate, "month")
}

// GetDailyEnergyData is a convenience method to get daily energy totals for the past week
func (c *Client) GetDailyEnergyData() (*HistoryData, error) {
	endDate := time.Now().Format("2006-01-02")
	startDate := time.Now().AddDate(0, 0, -7).Format("2006-01-02")
	return c.GetEnergyHistory(startDate, endDate, "day")
}

// EnableStormWatch is a convenience method to enable Storm Watch mode
func (c *Client) EnableStormWatch() error {
	return c.SetStormMode(true)
}

// DisableStormWatch is a convenience method to disable Storm Watch mode
func (c *Client) DisableStormWatch() error {
	return c.SetStormMode(false)
}

// SetMinimumBackupReserve sets backup reserve to a minimal level (5%)
func (c *Client) SetMinimumBackupReserve() error {
	return c.SetBackupReserve(5)
}

// SetMaximumBackupReserve sets backup reserve to maximum (100%)
func (c *Client) SetMaximumBackupReserve() error {
	return c.SetBackupReserve(100)
}
