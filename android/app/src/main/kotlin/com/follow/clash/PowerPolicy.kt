package com.follow.clash

internal data class BatteryReading(val percent: Int? = null, val charging: Boolean = false)

internal data class PowerPolicy(val minutes: Int = 0, val batteryPercent: Int = 0) {
    val enabled: Boolean get() = minutes in 1..1440 || batteryPercent in 1..50

    fun shouldStop(elapsedMillis: Long, battery: BatteryReading): Boolean =
        (minutes in 1..1440 && elapsedMillis >= minutes * 60_000L) ||
            (batteryPercent in 1..50 && !battery.charging &&
                battery.percent?.let { it in 0..batteryPercent } == true)
}
