import UIKit
import AVFoundation
import React
import React_RCTAppDelegate
import ReactAppDependencyProvider

@main
class AppDelegate: UIResponder, UIApplicationDelegate {
  var window: UIWindow?

  var reactNativeDelegate: ReactNativeDelegate?
  var reactNativeFactory: RCTReactNativeFactory?

  private let capabilityStates = Set([
    "UNKNOWN",
    "NOT_REQUESTED",
    "GRANTED",
    "DENIED",
    "RESTRICTED",
    "UNAVAILABLE",
  ])

  func application(
    _ application: UIApplication,
    didFinishLaunchingWithOptions launchOptions: [UIApplication.LaunchOptionsKey: Any]? = nil
  ) -> Bool {
    let delegate = ReactNativeDelegate()
    let factory = RCTReactNativeFactory(delegate: delegate)
    delegate.dependencyProvider = RCTAppDependencyProvider()

    reactNativeDelegate = delegate
    reactNativeFactory = factory

    window = UIWindow(frame: UIScreen.main.bounds)

    let initialProperties: [String: Any] = [
      "deviceCapability": "MICROPHONE",
      "deviceCapabilityState": microphoneState(),
    ]

    factory.startReactNative(
      withModuleName: "APGIC",
      in: window,
      initialProperties: initialProperties,
      launchOptions: launchOptions
    )

    return true
  }

  private func microphoneState() -> String {
#if DEBUG
    if let override = ProcessInfo.processInfo.environment["APGIC_E2E_CAPABILITY_STATE"],
       capabilityStates.contains(override) {
      return override
    }
#endif

    switch AVAudioSession.sharedInstance().recordPermission {
    case .undetermined:
      return "NOT_REQUESTED"
    case .denied:
      return "DENIED"
    case .granted:
      return "GRANTED"
    @unknown default:
      return "UNKNOWN"
    }
  }
}

class ReactNativeDelegate: RCTDefaultReactNativeFactoryDelegate {
  override func sourceURL(for bridge: RCTBridge) -> URL? {
    self.bundleURL()
  }

  override func bundleURL() -> URL? {
#if DEBUG
    RCTBundleURLProvider.sharedSettings().jsBundleURL(forBundleRoot: "index")
#else
    Bundle.main.url(forResource: "main", withExtension: "jsbundle")
#endif
  }
}
