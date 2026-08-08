# Model assumptions

## Scope

The model creates synthetic data for one household. It is useful for testing
state changes and battery decisions. It does not forecast a real household,
weather, or electricity market.

Photovoltaic (PV) means electricity produced by solar panels. Battery state of
charge (SOC) means stored energy as a percentage of battery capacity.

## Inputs

| Input | Unit or format | Effect |
| --- | --- | --- |
| Start | RFC 3339 timestamp | Sets the first event time. The simulator converts it to UTC. |
| Interval | Time duration | Sets the time between events. |
| Seed | Integer | Selects repeatable daily PV, load, and price scale factors. |
| Device ID | Text | Identifies the simulated household. |
| Battery capacity | Kilowatt-hours | Converts stored energy to battery SOC. |
| Starting battery SOC | Percent | Sets stored energy at the first event. |
| PV peak power | Kilowatts | Sets the upper bound of the solar profile. |
| Base load power | Kilowatts | Sets the scale of household electricity use. |
| Base price | Euros per kilowatt-hour | Sets the scale of the electricity price. |

## Outputs

Each telemetry event contains a producer-assigned event ID, timestamp, device
ID, PV power, load power, battery SOC, and electricity price. The policy adds a
decision, command power, and reason. The simulator derives a stable event ID
from the device ID and UTC timestamp so replaying the same interval retains its
identity.

Power uses kilowatts. Energy uses kilowatt-hours. The model treats sampled power
as constant until the next event:

```text
interval_energy_kwh = power_kw × interval_hours
```

## Time model

Each run covers the half-open range from `start` through `start + 24h`. The end
time is not emitted. The interval must divide 24 hours exactly.

Assumption: All profile clock times use UTC.

Consequence: Local time zones, daylight saving time, and solar time do not
shift the profiles.

Assumption: One sampled value represents its complete interval.

Consequence: Changes within an interval are not represented.

## Device or asset behavior

PV power is zero from 18:00 through 06:00 UTC. It follows a smooth curve during
daylight and reaches its highest value at noon.

Household load stays above zero. It has a morning peak and a larger evening
peak. Electricity price has a midday dip and morning and evening peaks.

The seed selects one scale factor for each profile for the full day. The same
configuration and seed produce the same telemetry.

The battery changes from the signed command power:

```text
battery_energy_kwh += signed_command_power_kw × interval_hours
battery_soc_percent = 100 × battery_energy_kwh / battery_capacity_kwh
```

Charge power is positive. Discharge power is negative. Stored energy stays
between empty and full.

The policy follows these rules:

- Charge when PV production exceeds household load and the battery is not full.
- Discharge a load deficit when price is at least 0.30 euros per kilowatt-hour
  and battery SOC is above 20%.
- Limit discharge power when needed to preserve the 20% reserve.
- Use idle for all other conditions.

## Simplifications

| Assumption | Consequence |
| --- | --- |
| Battery efficiency is 100%. | Charging and discharging lose no energy. |
| The battery has no power limit. | A command can request any power needed for an interval. |
| PV weather variation is one daily factor. | Passing clouds and short weather changes are absent. |
| Load has no appliance events. | The profile does not show individual device use. |
| Price is always positive. | Negative and volatile wholesale prices are absent. |
| The grid has no limit. | It supplies unmet load and receives unused PV production. |
| Profiles do not affect each other. | Household load does not change the simulated price. |

## Known limitations

- The model has one household and one battery.
- Battery temperature, age, standby loss, and charge-rate effects are absent.
- PV orientation, shading, location, and weather history are absent.
- Load and price shapes are fixed formulas, not measured data.
- The last command changes internal battery state, but no later event reports
  that final state.
