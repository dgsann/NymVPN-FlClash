package com.follow.clash

import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

class PowerPolicyTest {
    @Test fun `all policies are opt in and invalid values are ignored`() {
        for (policy in listOf(PowerPolicy(), PowerPolicy(-1, -1), PowerPolicy(1441, 51))) {
            assertFalse(policy.enabled)
            assertFalse(policy.shouldStop(Long.MAX_VALUE, BatteryReading(0)))
        }
    }
    @Test fun `timer fires at its deadline even while charging`() {
        val policy = PowerPolicy(minutes = 1)
        assertFalse(policy.shouldStop(59_999, BatteryReading(100)))
        assertTrue(policy.shouldStop(60_000, BatteryReading(100, true)))
    }
    @Test fun `battery condition requires a known unplugged reading at threshold`() {
        val policy = PowerPolicy(batteryPercent = 15)
        assertFalse(policy.shouldStop(0, BatteryReading()))
        assertFalse(policy.shouldStop(0, BatteryReading(-1)))
        assertFalse(policy.shouldStop(0, BatteryReading(16)))
        assertFalse(policy.shouldStop(0, BatteryReading(10, true)))
        assertTrue(policy.shouldStop(0, BatteryReading(15)))
        assertTrue(policy.shouldStop(0, BatteryReading(0)))
    }
}
