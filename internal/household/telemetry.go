package household

import "time"

type Telemetry struct {
	Timestamp         time.Time
	DeviceID          string
	PVPowerKW         float64
	LoadPowerKW       float64
	BatterySOCPercent float64
	PriceEURPerKWh    float64
}
