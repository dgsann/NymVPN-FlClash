package com.follow.clash.service.modules

import android.app.Service
import android.net.ConnectivityManager
import android.net.LinkProperties
import android.net.Network
import android.net.NetworkCapabilities
import android.net.NetworkCapabilities.TRANSPORT_SATELLITE
import android.net.NetworkCapabilities.TRANSPORT_USB
import android.net.NetworkRequest
import android.os.Build
import android.os.Handler
import android.os.Looper
import androidx.core.content.getSystemService
import com.follow.clash.core.Core
import java.net.Inet4Address
import java.net.Inet6Address
import java.net.InetAddress
import java.util.concurrent.ConcurrentHashMap

private data class NetworkInfo(
    @Volatile var losingUntilMillis: Long = 0,
    @Volatile var dnsList: List<InetAddress> = emptyList(),
    @Volatile var capabilities: NetworkCapabilities? = null,
) {
    val priorityPenalty: Int
        get() = if (losingUntilMillis > System.currentTimeMillis()) 10 else 0
}

internal class NetworkObserveModule(private val service: Service) : ServiceModule {

    private val networkInfos = ConcurrentHashMap<Network, NetworkInfo>()
    private val connectivity by lazy {
        service.getSystemService<ConnectivityManager>()
    }
    private val mainHandler = Handler(Looper.getMainLooper())
    private val dnsSelector = DnsNetworkSelector<Network>()
    @Volatile private var running = false

    private val request = NetworkRequest.Builder().apply {
        addCapability(NetworkCapabilities.NET_CAPABILITY_NOT_VPN)
        addCapability(NetworkCapabilities.NET_CAPABILITY_INTERNET)
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.P) {
            addCapability(NetworkCapabilities.NET_CAPABILITY_FOREGROUND)
        }
        addCapability(NetworkCapabilities.NET_CAPABILITY_NOT_RESTRICTED)
    }.build()

    private val callback = object : ConnectivityManager.NetworkCallback() {
        override fun onAvailable(network: Network) = updateNetwork {
            networkInfos[network] = NetworkInfo()
        }

        override fun onLosing(network: Network, maxMsToLive: Int) = updateNetwork {
            val info = networkInfos[network] ?: return@updateNetwork
            info.losingUntilMillis = System.currentTimeMillis() + maxMsToLive
            if (maxMsToLive > 0) {
                mainHandler.postDelayed({ updateNetwork {} }, maxMsToLive.toLong() + 50)
            }
        }

        override fun onLost(network: Network) = updateNetwork {
            networkInfos.remove(network)
        }

        override fun onCapabilitiesChanged(network: Network, capabilities: NetworkCapabilities) =
            updateNetwork {
                networkInfos[network]?.capabilities = capabilities
            }

        override fun onLinkPropertiesChanged(network: Network, linkProperties: LinkProperties) =
            updateNetwork {
                networkInfos[network]?.dnsList = linkProperties.dnsServers
            }
    }

    @Synchronized
    private fun updateNetwork(change: () -> Unit) {
        if (!running) return
        change()
        updateDns()
    }

    @Synchronized
    override fun start() {
        if (running) return
        running = true
        try {
            connectivity?.registerNetworkCallback(request, callback)
        } catch (error: Exception) {
            running = false
            throw error
        }
    }

    private fun dnsCandidate(entry: Map.Entry<Network, NetworkInfo>): DnsNetworkCandidate<Network> {
        val capabilities = entry.value.capabilities
        val access = when {
            capabilities == null -> NetworkAccess.UNKNOWN
            capabilities.hasCapability(NetworkCapabilities.NET_CAPABILITY_CAPTIVE_PORTAL) ->
                NetworkAccess.CAPTIVE_PORTAL
            capabilities.hasCapability(NetworkCapabilities.NET_CAPABILITY_VALIDATED) ->
                NetworkAccess.VALIDATED
            else -> NetworkAccess.UNVALIDATED
        }
        val transportPriority = when {
            capabilities == null -> 100
            capabilities.hasTransport(NetworkCapabilities.TRANSPORT_VPN) -> 90
            capabilities.hasTransport(NetworkCapabilities.TRANSPORT_WIFI) -> 0
            capabilities.hasTransport(NetworkCapabilities.TRANSPORT_ETHERNET) -> 1
            Build.VERSION.SDK_INT >= Build.VERSION_CODES.S &&
                capabilities.hasTransport(TRANSPORT_USB) -> 2
            capabilities.hasTransport(NetworkCapabilities.TRANSPORT_BLUETOOTH) -> 3
            capabilities.hasTransport(NetworkCapabilities.TRANSPORT_CELLULAR) -> 4
            Build.VERSION.SDK_INT >= Build.VERSION_CODES.VANILLA_ICE_CREAM &&
                capabilities.hasTransport(TRANSPORT_SATELLITE) -> 5
            else -> 20
        }
        return DnsNetworkCandidate(
            network = entry.key,
            dns = entry.value.dnsList.map { it.asSocketAddressText(DNS_PORT) }.distinct(),
            access = access,
            transportPriority = transportPriority,
            losing = entry.value.priorityPenalty != 0,
        )
    }

    @Synchronized
    private fun updateDns() {
        val selected = dnsSelector.update(networkInfos.entries.map(::dnsCandidate)) ?: return
        Core.updateDNS(selected.dns.joinToString(","))
    }

    @Synchronized
    override fun stop() {
        running = false
        mainHandler.removeCallbacksAndMessages(null)
        try {
            connectivity?.unregisterNetworkCallback(callback)
        } finally {
            networkInfos.clear()
            updateDns()
        }
    }
}

private const val DNS_PORT = 53

private fun InetAddress.asSocketAddressText(port: Int): String = when (this) {
    is Inet6Address -> "[$hostAddress]:$port"
    is Inet4Address -> "$hostAddress:$port"
    else -> error("Unsupported address type: ${javaClass.name}")
}
