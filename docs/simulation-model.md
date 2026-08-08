# Simulation Model

This guide explains what the v0.1 simulator generates and how to interpret it.
The profiles are deterministic test data, not forecasts or a model of a
specific household.

## One telemetry event

Each event is a snapshot at one UTC timestamp:

| Field | Unit | Meaning |
| --- | --- | --- |
| `PVPowerKW` | kW | Power produced by the solar panels. |
| `LoadPowerKW` | kW | Power consumed by the household. |
| `BatterySOCPercent` | % | Stored energy as a percentage of battery capacity. |
| `PriceEURPerKWh` | EUR/kWh | Cost of importing one kilowatt-hour at that time. |

Power and energy are related but different. Power (`kW`) is the rate of energy
flow; energy (`kWh`) is the amount transferred over time:

```text
energy_kwh = power_kw × interval_hours
```

For example, a steady 4 kW flow over a 15-minute (0.25-hour) interval transfers
1 kWh. When interval energy is calculated, the sampled power is treated as
constant until the next event. An event reports the battery state at its
timestamp; the command applied for that interval changes the state reported by
the following event.

PV and load are both non-negative measurements. Their difference describes the
household balance before battery action:

```text
surplus_power_kw = pv_power_kw - load_power_kw
```

A positive result is surplus generation; a negative result is demand that must
come from the battery or grid. The simulator treats this result as
battery-relative power: positive power charges the battery and negative power
discharges it. This sign convention avoids encoding import or export as
negative PV or load values.

Battery energy evolves once per interval from the applied command:

```text
battery_energy_kwh += signed_command_power_kw × interval_hours
battery_soc_percent = 100 × battery_energy_kwh / battery_capacity_kwh
```

Charge power is positive, discharge power is negative, and idle power is zero.
The runnable application obtains that command from the control policy. The
standalone `SimulateDay` helper uses the household's PV-minus-load balance as an
uncontrolled command so it retains the original passive simulation behavior.
When the policy discharges, it limits command power to the energy available
above the 20% reserve over the configured interval.

Stored energy is clamped to zero and the configured capacity. The grid
implicitly handles any household balance that the command does not assign to
the battery. This includes surplus or demand left by an idle command and energy
that would overfill or empty the battery. The model assumes perfect charge and
discharge efficiency and no battery power limit.

## Daily profiles

`SimulateDay` emits the half-open window `[start, start + 24h)`, sampled at the
configured interval. The interval must divide 24 hours exactly, so every day
has a complete and predictable event count.

| Profile | Daily shape | Seeded day scale |
| --- | --- | --- |
| PV | Zero outside 06:00–18:00 UTC; smooth rise to noon and fall afterward. | 80–100% of configured peak power. |
| Load | Always above zero; broad morning and larger evening demand peaks. | Multiply the whole curve by 0.85–1.15. |
| Price | Positive retail-style price; midday dip, small morning peak, largest evening peak. | Multiply the whole curve by 0.90–1.10. |

The load and price peaks use smooth bell-shaped curves. This prevents abrupt
jumps at exact clock times while keeping the intended daily pattern easy to
test.

## Determinism and scope

Each simulator owns its clock and pseudorandom number generator. The same
configuration and seed produce the same events. A different seed changes each
profile's daily scale while preserving its shape and bounds. Calling
`SimulateDay` again advances that simulator by exactly 24 hours and consumes the
next values from its random sequence.

Deliberate simplifications:

- Clock times are UTC, not household-local time or solar time.
- Weather changes the whole PV day by one factor; there are no passing clouds.
- Load has no per-appliance events or random interval-to-interval noise.
- Price models a positive time-of-use retail tariff, not volatile wholesale
  prices, taxes, or negative prices.
- Profiles are independent; the price does not react to this household's load.
- Battery losses and battery power limits are not modeled. Commands apply their
  full requested interval power until stored energy reaches a physical bound.

These constraints keep v0.1 reproducible and make later reliability and control
behavior testable without pretending the synthetic data is physically complete.
