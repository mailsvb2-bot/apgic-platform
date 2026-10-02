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

    var initialProperties: [String: Any] = [
      "deviceCapability": "MICROPHONE",
      "deviceCapabilityState": microphoneState(),
      "compatibilityPlatform": "IOS",
      "appVersion": Bundle.main.object(forInfoDictionaryKey: "CFBundleShortVersionString") as? String ?? "0.0.0",
    ]

#if DEBUG
    let environment = ProcessInfo.processInfo.environment
    let installationE2EKeys = [
      "APGIC_E2E_INSTALLATION_BASE_URL": "installationE2EBaseURL",
      "APGIC_E2E_SESSION_COOKIE": "installationE2ESessionCookie",
      "APGIC_E2E_INSTALLATION_ID": "installationE2EInstallationID",
      "APGIC_E2E_INSTALLATION_PLATFORM": "installationE2EPlatform",
      "APGIC_E2E_DEEP_LINK_BASE_URL": "deepLinkAPIBaseURL",
      "APGIC_E2E_DEEP_LINK_SESSION_COOKIE": "deepLinkE2ESessionCookie",
      "APGIC_E2E_DEEP_LINK_URL": "deepLinkE2EURL",
      "APGIC_E2E_NOTIFICATION_BASE_URL": "notificationE2EBaseURL",
      "APGIC_E2E_NOTIFICATION_SESSION_COOKIE": "notificationE2ESessionCookie",
      "APGIC_E2E_NOTIFICATION_DELIVERY_ID": "notificationE2EDeliveryID",
      "APGIC_E2E_NOTIFICATION_INTENT_ID": "notificationE2EIntentID",
      "APGIC_E2E_OFFLINE_MUTATION_BASE_URL": "offlineMutationE2EBaseURL",
      "APGIC_E2E_OFFLINE_MUTATION_SESSION_COOKIE": "offlineMutationE2ESessionCookie",
      "APGIC_E2E_OFFLINE_MUTATION_HOLD_ID": "offlineMutationE2EHoldID",
      "APGIC_E2E_OFFLINE_MUTATION_IDEMPOTENCY_KEY": "offlineMutationE2EIdempotencyKey",
      "APGIC_E2E_OFFLINE_MUTATION_METHOD_CODE": "offlineMutationE2EMethodCode",
      "APGIC_E2E_REALTIME_EVENTS": "realtimeE2EEvents",
      "APGIC_E2E_REALTIME_RECONNECT_FAILURES": "realtimeE2EReconnectFailures",
      "APGIC_E2E_REALTIME_CONSULTATION_ID": "realtimeE2EConsultationID",
      "APGIC_E2E_COMPATIBILITY_BASE_URL": "compatibilityBaseURL",
      "APGIC_E2E_APP_VERSION": "appVersion",
      "APGIC_E2E_CONTRACT_VERSION": "compatibilityContractVersion",
    ]
    for (environmentKey, propertyKey) in installationE2EKeys {
      if let value = environment[environmentKey], !value.isEmpty {
        initialProperties[propertyKey] = value
      }
    }
#endif

    factory.startReactNative(
      withModuleName: "APGIC",
      in: window,
      initialProperties: initialProperties,
      launchOptions: launchOptions
    )

    return true
  }

  func application(
    _ app: UIApplication,
    open url: URL,
    options: [UIApplication.OpenURLOptionsKey: Any] = [:]
  ) -> Bool {
    RCTLinkingManager.application(app, open: url, options: options)
  }

  func application(
    _ application: UIApplication,
    continue userActivity: NSUserActivity,
    restorationHandler: @escaping ([UIUserActivityRestoring]?) -> Void
  ) -> Bool {
    RCTLinkingManager.application(
      application,
      continue: userActivity,
      restorationHandler: restorationHandler
    )
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