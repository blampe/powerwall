package powerwall

import (
	"encoding/json"
	"strings"
	"time"
)

// Most time values in the API are produced in standard ISO-8601 format, which
// works just fine for unmarshalling to time.Time as well.  However, the
// "start_time" field of the status API call is not returned in this format for
// some reason and thus will not unmarshal directly to a time.Time value.  We
// provide a custom type to handle this case.
type NonIsoTime struct {
	time.Time
}

const nonIsoTimeFormat = "2006-01-02 15:04:05 -0700"

func (v *NonIsoTime) UnmarshalJSON(p []byte) error {
	t, err := time.Parse(nonIsoTimeFormat, strings.Replace(string(p), `"`, ``, -1))
	if err == nil {
		*v = NonIsoTime{t}
	}
	return err
}

// Durations in the API are typically represented as strings in duration-string
// format ("1h23m45.67s", etc).  Go's time.Duration type actually produces this
// format natively, yet will not parse it as an input when unmarshalling JSON
// (grr), so we need a custom type (with a custom UnmarshalJSON function) to
// handle this.
type Duration struct {
	time.Duration
}

func (v *Duration) UnmarshalJSON(p []byte) error {
	d, err := time.ParseDuration(strings.Replace(string(p), `"`, ``, -1))
	if err == nil {
		*v = Duration{d}
	}
	return err
}

func (v *Duration) MarshalJSON() ([]byte, error) {
	return json.Marshal(v.String())
}

// The DecodedAlert type is used for unpacking values in the "decoded_alert"
// field of GridFault structures.  These are actually encoded as a string,
// which itself contains a JSON representation of a list of maps, each one
// containing a "name" and "value".  For example:
//
// "[{\"name\":\"PINV_alertID\",\"value\":\"PINV_a008_vfCheckRocof\"},{\"name\":\"PINV_alertType\",\"value\":\"Warning\"}]"
//
// Needless to say, this encoding is rather cumbersome and redundant, so we
// instead provide a custom JSON decoder to decode these into a string/string
// map in the form 'name: value'.
type DecodedAlert map[string]interface{}

func (v *DecodedAlert) UnmarshalJSON(data []byte) error {
	type entry struct {
		Name  string      `json:"name"`
		Value interface{} `json:"value"`
	}

	var strvalue string
	err := json.Unmarshal(data, &strvalue)
	if err != nil {
		return err
	}
	if strvalue == "" {
		// For an empty string, just return a nil map
		return nil
	}

	entries := []entry{}
	err = json.Unmarshal([]byte(strvalue), &entries)
	if err != nil {
		return err
	}

	*v = make(map[string]interface{}, len(entries))
	for _, e := range entries {
		(*v)[e.Name] = e.Value
	}
	return nil
}

///////////////////////////////////////////////////////////////////////////////
// Core API Data Structures

// StatusData contains general system information returned by the "status" API
// call, such as the device identification number, type, software version, etc.
//
// This structure is returned by the GetStatus function.
type StatusData struct {
	Din              string      `json:"din"`
	StartTime        NonIsoTime  `json:"start_time"`
	UpTime           Duration    `json:"up_time_seconds"`
	IsNew            bool        `json:"is_new"`
	Version          string      `json:"version"`
	GitHash          string      `json:"git_hash"`
	CommissionCount  int         `json:"commission_count"`
	DeviceType       string      `json:"device_type"`
	SyncType         string      `json:"sync_type"`
	Leader           string      `json:"leader"`
	Followers        interface{} `json:"followers"` // TODO: Unsure what type this returns when present
	CellularDisabled bool        `json:"cellular_disabled"`
}

// SiteInfoData contains information returned by the "site_info" API call.
//
// This structure is returned by the GetSiteInfo function.
type SiteInfoData struct {
	SiteName               string  `json:"site_name"`
	TimeZone               string  `json:"timezone"`
	MaxSiteMeterPowerKW    int     `json:"max_site_meter_power_kW"`
	MinSiteMeterPowerKW    int     `json:"min_site_meter_power_kW"`
	MeasuredFrequency      float32 `json:"measured_frequency"`
	MaxSystemEnergyKWH     float32 `json:"max_system_energy_kWh"`
	MaxSystemPowerKW       float32 `json:"max_system_power_kW"`
	NominalSystemEnergyKWH float32 `json:"nominal_system_energy_kWh"`
	NominalSystemPowerKW   float32 `json:"nominal_system_power_kW"`
	GridData               struct {
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
	} `json:"grid_code"`
}

// MeterAggregatesData contains fields returned by the "meters/aggregates" API
// call.  This reflects statistics collected across all of the meters in a
// given category (e.g. "site", "solar", "battery", "load", etc).
//
// This structure is returned by the GetMetersAggregates function.
type MeterAggregatesData struct {
	LastCommunicationTime             time.Time `json:"last_communication_time"`
	InstantPower                      float32   `json:"instant_power"`
	InstantReactivePower              float32   `json:"instant_reactive_power"`
	InstantApparentPower              float32   `json:"instant_apparent_power"`
	Frequency                         float32   `json:"frequency"`
	EnergyExported                    float32   `json:"energy_exported"`
	EnergyImported                    float32   `json:"energy_imported"`
	InstantAverageVoltage             float32   `json:"instant_average_voltage"`
	InstantAverageCurrent             float32   `json:"instant_average_current"`
	IACurrent                         float32   `json:"i_a_current"`
	IBCurrent                         float32   `json:"i_b_current"`
	ICCurrent                         float32   `json:"i_c_current"`
	LastPhaseVoltageCommunicationTime time.Time `json:"last_phase_voltage_communication_time"`
	LastPhasePowerCommunicationTime   time.Time `json:"last_phase_power_communication_time"`
	Timeout                           int64     `json:"timeout"`
	NumMetersAggregated               int       `json:"num_meters_aggregated"`
	InstantTotalCurrent               float32   `json:"instant_total_current"`
}

// GridStatusData contains fields returned by the "system_status/grid_status" API call.
//
// This structure is returned by the GetGridStatus function.
type GridStatusData struct {
	GridStatus         string `json:"grid_status"`
	GridServicesActive bool   `json:"grid_services_active"`
}

// SOEData contains fields returned by the "system_status/soe" API call.
// This currently just returns a single "Percentage" field, indicating the
// total amount of charge across all batteries.
//
// This structure is returned by the GetSOE function.
type SOEData struct {
	Percentage float64 `json:"percentage"`
}

// OperationData contains fields returned by the "operation" API call.
//
// This structure is returned by the GetOperation function.
type OperationData struct {
	RealMode                string  `json:"real_mode"`
	BackupReservePercent    float64 `json:"backup_reserve_percent"`
	FreqShiftLoadShedSoe    float64 `json:"freq_shift_load_shed_soe"`
	FreqShiftLoadShedDeltaF float64 `json:"freq_shift_load_shed_delta_f"`
}

// SystemStatusData contains fields returned by the "system_status" API call.
// This contains a lot of information about the general state of the system and
// how it is operating, such as battery charge, utility power status, etc.
//
// This structure is returned by the GetSystemStatus function.
type SystemStatusData struct {
	CommandSource                  string  `json:"command_source"`
	BatteryTargetPower             float64 `json:"battery_target_power"`
	BatteryTargetReactivePower     float64 `json:"battery_target_reactive_power"`
	NominalFullPackEnergy          float64 `json:"nominal_full_pack_energy"`
	NominalEnergyRemaining         float64 `json:"nominal_energy_remaining"`
	MaxPowerEnergyRemaining        float64 `json:"max_power_energy_remaining"`
	MaxPowerEnergyToBeCharged      float64 `json:"max_power_energy_to_be_charged"`
	MaxChargePower                 float64 `json:"max_charge_power"`
	MaxDischargePower              float64 `json:"max_discharge_power"`
	MaxApparentPower               float64 `json:"max_apparent_power"`
	InstantaneousMaxDischargePower float64 `json:"instantaneous_max_discharge_power"`
	InstantaneousMaxChargePower    float64 `json:"instantaneous_max_charge_power"`
	GridServicesPower              float64 `json:"grid_services_power"`
	SystemIslandState              string  `json:"system_island_state"`
	AvailableBlocks                int     `json:"available_blocks"`
	// Simplified battery_blocks for now
	BatteryBlocks              []interface{}   `json:"battery_blocks"`
	FfrPowerAvailabilityHigh   float64         `json:"ffr_power_availability_high"`
	FfrPowerAvailabilityLow    float64         `json:"ffr_power_availability_low"`
	LoadChargeConstraint       float64         `json:"load_charge_constraint"`
	MaxSustainedRampRate       float64         `json:"max_sustained_ramp_rate"`
	GridFaults                 []GridFaultData `json:"grid_faults"`
	CanReboot                  string          `json:"can_reboot"`
	SmartInvDeltaP             float64         `json:"smart_inv_delta_p"`
	SmartInvDeltaQ             float64         `json:"smart_inv_delta_q"`
	LastToggleTimestamp        time.Time       `json:"last_toggle_timestamp"`
	SolarRealPowerLimit        float64         `json:"solar_real_power_limit"`
	Score                      float64         `json:"score"`
	BlocksControlled           int             `json:"blocks_controlled"`
	Primary                    bool            `json:"primary"`
	AuxiliaryLoad              float64         `json:"auxiliary_load"`
	AllEnableLinesHigh         bool            `json:"all_enable_lines_high"`
	InverterNominalUsablePower float64         `json:"inverter_nominal_usable_power"`
	ExpectedEnergyRemaining    float64         `json:"expected_energy_remaining"`
}

// GridFaultData contains fields returned by the "system_status/grid_faults" API call.
//
// This structure is returned by the GetSystemStatus and GetGridFaults functions.
type GridFaultData struct {
	Timestamp              int64        `json:"timestamp"`
	AlertName              string       `json:"alert_name"`
	AlertIsFault           bool         `json:"alert_is_fault"`
	DecodedAlert           DecodedAlert `json:"decoded_alert"`
	AlertRaw               int64        `json:"alert_raw"`
	GitHash                string       `json:"git_hash"`
	SiteUID                string       `json:"site_uid"`
	EcuType                string       `json:"ecu_type"`
	EcuPackagePartNumber   string       `json:"ecu_package_part_number"`
	EcuPackageSerialNumber string       `json:"ecu_package_serial_number"`
}

// NetworkData contains information returned by the "networks" API call for a
// particular network interface.
//
// A list of this structure is returned by the GetNetworks function.
type NetworkData struct {
	NetworkName string `json:"network_name"`
	Interface   string `json:"interface"`
	Dhcp        bool   `json:"dhcp"`
	Enabled     bool   `json:"enabled"`
	ExtraIps    []struct {
		IP      string `json:"ip"`
		Netmask int    `json:"netmask"`
	} `json:"extra_ips,omitempty"`
	Active                bool `json:"active"`
	Primary               bool `json:"primary"`
	LastTeslaConnected    bool `json:"lastTeslaConnected"`
	LastInternetConnected bool `json:"lastInternetConnected"`
	IfaceNetworkInfo      struct {
		NetworkName string `json:"network_name"`
		IPNetworks  []struct {
			IP   string `json:"ip"`
			Mask string `json:"mask"`
		} `json:"ip_networks"`
		Gateway        string `json:"gateway"`
		Interface      string `json:"interface"`
		State          string `json:"state"`
		StateReason    string `json:"state_reason"`
		SignalStrength int    `json:"signal_strength"`
		HwAddress      string `json:"hw_address"`
	} `json:"iface_network_info"`
	SecurityType string `json:"security_type"`
	Username     string `json:"username"`
}

// MeterData contains fields returned by the "meters/<category>" API call, which
// returns information for each individual meter within that category.
//
// A list of this structure is returned by the GetMeters function.
type MeterData struct {
	ID         int    `json:"id"`
	Location   string `json:"location"`
	Type       string `json:"type"`
	Cts        []bool `json:"cts"`
	Inverted   []bool `json:"inverted"`
	Connection struct {
		ShortID      string `json:"short_id"`
		DeviceSerial string `json:"device_serial"`
		HTTPSConf    struct {
			ClientCert          string `json:"client_cert"`
			ClientKey           string `json:"client_key"`
			ServerCaCert        string `json:"server_ca_cert"`
			MaxIdleConnsPerHost int    `json:"max_idle_conns_per_host"`
		} `json:"https_conf"`
	} `json:"connection"`
	RealPowerScaleFactor float32 `json:"real_power_scale_factor"`
	CachedReadings       struct {
		LastCommunicationTime             time.Time `json:"last_communication_time"`
		InstantPower                      float32   `json:"instant_power"`
		InstantReactivePower              float32   `json:"instant_reactive_power"`
		InstantApparentPower              float32   `json:"instant_apparent_power"`
		Frequency                         float32   `json:"frequency"`
		EnergyExported                    float32   `json:"energy_exported"`
		EnergyImported                    float32   `json:"energy_imported"`
		InstantAverageVoltage             float32   `json:"instant_average_voltage"`
		InstantAverageCurrent             float32   `json:"instant_average_current"`
		IACurrent                         float32   `json:"i_a_current"`
		IBCurrent                         float32   `json:"i_b_current"`
		ICCurrent                         float32   `json:"i_c_current"`
		VL1N                              float32   `json:"v_l1n"`
		VL2N                              float32   `json:"v_l2n"`
		LastPhaseVoltageCommunicationTime time.Time `json:"last_phase_voltage_communication_time"`
		RealPowerA                        float32   `json:"real_power_a"`
		RealPowerB                        float32   `json:"real_power_b"`
		ReactivePowerA                    float32   `json:"reactive_power_a"`
		ReactivePowerB                    float32   `json:"reactive_power_b"`
		LastPhasePowerCommunicationTime   time.Time `json:"last_phase_power_communication_time"`
		SerialNumber                      string    `json:"serial_number"`
		Timeout                           int64     `json:"timeout"`
		InstantTotalCurrent               float32   `json:"instant_total_current"`
	} `json:"Cached_readings"`
	CtVoltageReferences struct {
		Ct1 string `json:"ct1"`
		Ct2 string `json:"ct2"`
		Ct3 string `json:"ct3"`
	} `json:"ct_voltage_references"`
}

// SitemasterData contains information returned by the "sitemaster" API call.
//
// The CanReboot field indicates whether the sitemaster can be
// stopped/restarted without disrupting anything or not.  It will be either
// "Yes", or it will be a string indicating the reason why it can't be stopped
// right now (such as "Power flow is too high").  (see the SitemasterReboot*
// constants for known possible values)
//
// Note that it is still possible to stop the sitemaster even under these
// conditions, but it is necessary to set the "force" option to true when
// calling the "sitemaster/stop" API in that case.
//
// This structure is returned by the GetSitemaster function.
type SitemasterData struct {
	Status           string `json:"status"`
	Running          bool   `json:"running"`
	ConnectedToTesla bool   `json:"connected_to_tesla"`
	PowerSupplyMode  bool   `json:"power_supply_mode"`
	CanReboot        string `json:"can_reboot"`
}

///////////////////////////////////////////////////////////////////////////////
// Fleet API Types

// EnergyProduct represents an energy site (solar, battery, etc.) from the Fleet API
type EnergyProduct struct {
	EnergyProductID   int64                  `json:"energy_site_id"`
	DeviceType        string                 `json:"device_type"`
	ResourceType      string                 `json:"resource_type"` // "solar", "battery"
	SiteName          string                 `json:"site_name"`
	ID                string                 `json:"id"`
	GatewayID         string                 `json:"gateway_id"`
	AssetSiteID       string                 `json:"asset_site_id"`
	WarpSiteNumber    string                 `json:"warp_site_number"`
	PercentageCharged *float64               `json:"percentage_charged"`
	BatteryType       string                 `json:"battery_type,omitempty"`
	BatteryPower      *float64               `json:"battery_power"`
	StormModeEnabled  *bool                  `json:"storm_mode_enabled"`
	Components        map[string]interface{} `json:"components,omitempty"`
	Features          map[string]interface{} `json:"features,omitempty"`
}

// ProductsResponse represents the response from the Fleet API products endpoint
type ProductsResponse struct {
	Response []EnergyProduct `json:"response"` // Flat array of products
	Count    int             `json:"count"`
}

// LiveStatusResponse represents the response from the Fleet API live_status endpoint
type LiveStatusResponse struct {
	Response struct {
		SolarPower         *float64  `json:"solar_power"`
		BatteryPower       *float64  `json:"battery_power"`
		LoadPower          *float64  `json:"load_power"`
		GridPower          *float64  `json:"grid_power"`
		EnergyLeft         *float64  `json:"energy_left"`
		TotalPackEnergy    *float64  `json:"total_pack_energy"`
		PercentageCharged  *float64  `json:"percentage_charged"`
		GridStatus         string    `json:"grid_status"`
		IslandStatus       string    `json:"island_status"`
		StormModeActive    bool      `json:"storm_mode_active"`
		GridServicesActive bool      `json:"grid_services_active"`
		Timestamp          time.Time `json:"timestamp"`
		// Power flow data (matches existing meters/aggregates structure)
		Site    *MeterAggregatesData `json:"site,omitempty"`
		Solar   *MeterAggregatesData `json:"solar,omitempty"`
		Battery *MeterAggregatesData `json:"battery,omitempty"`
		Load    *MeterAggregatesData `json:"load,omitempty"`
	} `json:"response"`
}

// SiteInfoResponse represents the response from the Fleet API site_info endpoint
type SiteInfoResponse struct {
	Response struct {
		ID                   string                 `json:"id"`
		SiteName             string                 `json:"site_name"`
		BackupReservePercent *int                   `json:"backup_reserve_percent"`
		DefaultRealMode      string                 `json:"default_real_mode"`
		InstallationDate     time.Time              `json:"installation_date"`
		UserSettings         map[string]interface{} `json:"user_settings"`
		Components           struct {
			Solar               bool   `json:"solar"`
			SolarType           string `json:"solar_type"`
			Battery             bool   `json:"battery"`
			Grid                bool   `json:"grid"`
			Backup              bool   `json:"backup"`
			Gateway             string `json:"gateway"`
			LoadMeter           bool   `json:"load_meter"`
			TOUCapable          bool   `json:"tou_capable"`
			StormModeCapable    bool   `json:"storm_mode_capable"`
			BatteryType         string `json:"battery_type"`
			Configurable        bool   `json:"configurable"`
			GridServicesEnabled bool   `json:"grid_services_enabled"`
			Gateways            []struct {
				DeviceID        string    `json:"device_id"`
				DIN             string    `json:"din"`
				SerialNumber    string    `json:"serial_number"`
				PartNumber      string    `json:"part_number"`
				PartType        int       `json:"part_type"`
				PartName        string    `json:"part_name"`
				IsActive        bool      `json:"is_active"`
				SiteID          string    `json:"site_id"`
				FirmwareVersion string    `json:"firmware_version"`
				UpdatedDatetime time.Time `json:"updated_datetime"`
			} `json:"gateways"`
		} `json:"components"`
		Version                 string                 `json:"version"`
		BatteryCount            int                    `json:"battery_count"`
		TariffContent           map[string]interface{} `json:"tariff_content"`
		TariffID                string                 `json:"tariff_id"`
		NameplatePower          int                    `json:"nameplate_power"`
		InstallationTimeZone    string                 `json:"installation_time_zone"`
		MaxSiteMeterPowerAC     int64                  `json:"max_site_meter_power_ac"`
		MinSiteMeterPowerAC     int64                  `json:"min_site_meter_power_ac"`
		VPPBackupReservePercent int                    `json:"vpp_backup_reserve_percent"`
		Utility                 string                 `json:"utility"`
	} `json:"response"`
}

// HistoryData represents historical power/energy data from Fleet API
type HistoryData struct {
	SerialNumber string      `json:"serial_number"`
	Period       string      `json:"period"` // "day", "week", "month", "year", "lifetime"
	TimeSeries   []TimePoint `json:"time_series"`
}

// TimePoint represents a single data point in historical data
type TimePoint struct {
	Timestamp time.Time `json:"timestamp"`

	// Power data (15-min intervals)
	SolarPower        *float64 `json:"solar_power,omitempty"`
	BatteryPower      *float64 `json:"battery_power,omitempty"`
	GridPower         *float64 `json:"grid_power,omitempty"`
	GridServicesPower *float64 `json:"grid_services_power,omitempty"`
	GeneratorPower    *float64 `json:"generator_power,omitempty"`

	// Energy data (daily/weekly/monthly/yearly)
	SolarEnergyExported    *float64 `json:"solar_energy_exported,omitempty"`
	GridEnergyImported     *float64 `json:"grid_energy_imported,omitempty"`
	GridEnergyExported     *float64 `json:"grid_energy_exported,omitempty"`
	BatteryEnergyExported  *float64 `json:"battery_energy_exported,omitempty"`
	BatteryEnergyImported  *float64 `json:"battery_energy_imported,omitempty"`
	ConsumerEnergyImported *float64 `json:"consumer_energy_imported,omitempty"`
}

// RateLimitConfig defines rate limiting configuration for Fleet API
type RateLimitConfig struct {
	RealtimeDataRPM int `json:"realtime_data_rpm"` // 60 requests per minute (Tesla limit)
	CommandsRPM     int `json:"commands_rpm"`      // 30 requests per minute (Tesla limit)
	MaxMonthlyCost  int `json:"max_monthly_cost"`  // $10 free tier limit
}
