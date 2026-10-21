package com.follow.clash.service.modules

internal enum class NetworkAccess(val penalty: Int) {
    VALIDATED(0),
    UNVALIDATED(100),
    CAPTIVE_PORTAL(200),
    UNKNOWN(1000),
}

internal data class DnsNetworkCandidate<N>(
    val network: N,
    val dns: List<String>,
    val access: NetworkAccess,
    val transportPriority: Int,
    val losing: Boolean = false,
) {
    val priority: Int
        get() = access.penalty + transportPriority + if (losing) 10 else 0
}

internal data class DnsNetworkSelection<N>(
    val network: N?,
    val dns: List<String>,
    val access: NetworkAccess,
)

internal class DnsNetworkSelector<N> {
    private var current = DnsNetworkSelection<N>(null, emptyList(), NetworkAccess.UNKNOWN)

    fun update(candidates: Iterable<DnsNetworkCandidate<N>>): DnsNetworkSelection<N>? {
        val selected = candidates.minWithOrNull(
            compareBy<DnsNetworkCandidate<N>> { it.priority }
                .thenBy { if (it.network == current.network) 0 else 1 },
        )
        val next = DnsNetworkSelection<N>(
            selected?.network,
            selected?.dns.orEmpty(),
            selected?.access ?: NetworkAccess.UNKNOWN,
        )
        if (next == current) return null
        current = next
        return next
    }
}
