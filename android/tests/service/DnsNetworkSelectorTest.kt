package com.follow.clash.service.modules

import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Test

private fun wifi(access: NetworkAccess, dns: String = "192.0.2.53:53") =
    DnsNetworkCandidate("wifi", listOf(dns), access, transportPriority = 0)

private fun mobile() = DnsNetworkCandidate(
    "mobile", listOf("198.51.100.53:53"), NetworkAccess.VALIDATED, transportPriority = 4,
)

class DnsNetworkSelectorTest {
    @Test
    fun `joining a captive wifi keeps validated mobile DNS`() {
        val selector = DnsNetworkSelector<String>()
        assertEquals("mobile", selector.update(listOf(mobile()))?.network)
        assertNull(selector.update(listOf(mobile(), wifi(NetworkAccess.UNKNOWN))))
        assertNull(selector.update(listOf(mobile(), wifi(NetworkAccess.UNVALIDATED))))
        assertNull(selector.update(listOf(mobile(), wifi(NetworkAccess.CAPTIVE_PORTAL))))
        assertEquals("wifi", selector.update(listOf(mobile(), wifi(NetworkAccess.VALIDATED)))?.network)
    }

    @Test
    fun `portal login restarts checks even when network and DNS stay the same`() {
        val selector = DnsNetworkSelector<String>()
        val before = selector.update(listOf(wifi(NetworkAccess.CAPTIVE_PORTAL)))!!
        val after = selector.update(listOf(wifi(NetworkAccess.VALIDATED)))!!
        assertEquals(before.network, after.network)
        assertEquals(before.dns, after.dns)
        assertEquals(NetworkAccess.VALIDATED, after.access)
        assertNull(selector.update(listOf(wifi(NetworkAccess.VALIDATED))))
    }

    @Test
    fun `losing wifi validation returns to mobile without a reboot`() {
        val selector = DnsNetworkSelector<String>()
        assertEquals("wifi", selector.update(listOf(mobile(), wifi(NetworkAccess.VALIDATED)))?.network)
        assertEquals("mobile", selector.update(listOf(mobile(), wifi(NetworkAccess.CAPTIVE_PORTAL)))?.network)
        assertNull(selector.update(listOf(mobile())))
        assertEquals(emptyList<String>(), selector.update(emptyList())?.dns)
        assertEquals("mobile", selector.update(listOf(mobile()))?.network)
    }

    @Test
    fun `an unvalidated network remains usable if Android validation is blocked`() {
        val selector = DnsNetworkSelector<String>()
        assertEquals("wifi", selector.update(listOf(wifi(NetworkAccess.UNVALIDATED)))?.network)
        assertEquals(
            listOf("203.0.113.53:53"),
            selector.update(listOf(wifi(NetworkAccess.UNVALIDATED, "203.0.113.53:53")))?.dns,
        )
    }

    @Test
    fun `equal priority and unchanged capabilities do not reset a working route`() {
        val selector = DnsNetworkSelector<String>()
        val first = wifi(NetworkAccess.VALIDATED)
        val second = first.copy(network = "other wifi")
        assertEquals("wifi", selector.update(listOf(first))?.network)
        assertNull(selector.update(listOf(second, first)))
    }

    @Test
    fun `losing network penalty still favors a healthy backup`() {
        val selector = DnsNetworkSelector<String>()
        val first = wifi(NetworkAccess.VALIDATED)
        assertEquals("wifi", selector.update(listOf(first, mobile()))?.network)
        assertEquals("mobile", selector.update(listOf(first.copy(losing = true), mobile()))?.network)
        assertEquals("wifi", selector.update(listOf(first, mobile()))?.network)
    }
}
