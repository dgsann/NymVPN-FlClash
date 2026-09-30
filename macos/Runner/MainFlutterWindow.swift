import Cocoa
import IOKit.ps
import FlutterMacOS
import window_manager
import LaunchAtLogin

class MainFlutterWindow: NSWindow {
    override func awakeFromNib() {
        let flutterViewController = FlutterViewController()
        let windowFrame = self.frame
        self.contentViewController = flutterViewController
        self.setFrame(windowFrame, display: true)
        
        FlutterMethodChannel(
            name: "launch_at_startup", binaryMessenger: flutterViewController.engine.binaryMessenger
        )
        .setMethodCallHandler { (_ call: FlutterMethodCall, result: @escaping FlutterResult) in
            switch call.method {
            case "launchAtStartupIsEnabled":
                result(LaunchAtLogin.isEnabled)
            case "launchAtStartupSetEnabled":
                if let arguments = call.arguments as? [String: Any] {
                    LaunchAtLogin.isEnabled = arguments["setEnabledValue"] as! Bool
                }
                result(nil)
            default:
                result(FlutterMethodNotImplemented)
            }
        }
        
        FlutterMethodChannel(name: "com.follow.clash/power", binaryMessenger: flutterViewController.engine.binaryMessenger)
            .setMethodCallHandler { call, result in
                guard call.method == "readBattery" else {
                    result(FlutterMethodNotImplemented)
                    return
                }
                result(nymBatteryReading())
            }
        RegisterGeneratedPlugins(registry: flutterViewController)
        super.awakeFromNib()
    }
    override public func order(_ place: NSWindow.OrderingMode, relativeTo otherWin: Int) {
        super.order(place, relativeTo: otherWin)
        hiddenWindowAtLaunch()
    }
}

private func nymBatteryReading() -> [String: Any]? {
    guard let info = IOPSCopyPowerSourcesInfo()?.takeRetainedValue(),
          let sources = IOPSCopyPowerSourcesList(info)?.takeRetainedValue() as? [CFTypeRef] else { return nil }
    for source in sources {
        guard let data = IOPSGetPowerSourceDescription(info, source)?.takeUnretainedValue() as? [String: Any],
              data[kIOPSTypeKey] as? String == kIOPSInternalBatteryType,
              data[kIOPSIsPresentKey] as? Bool == true,
              let current = data[kIOPSCurrentCapacityKey] as? Int,
              let maximum = data[kIOPSMaxCapacityKey] as? Int,
              maximum > 0, current >= 0, current <= maximum,
              let sourceState = data[kIOPSPowerSourceStateKey] as? String else { continue }
        return ["percent": current * 100 / maximum,
                "onBattery": sourceState == kIOPSBatteryPowerValue]
    }
    return nil
}
