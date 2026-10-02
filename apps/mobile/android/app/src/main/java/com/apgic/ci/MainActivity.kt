package com.apgic.ci

import android.Manifest
import android.content.pm.PackageManager
import android.os.Bundle
import com.facebook.react.ReactActivity
import com.facebook.react.ReactActivityDelegate
import com.facebook.react.defaults.DefaultNewArchitectureEntryPoint.fabricEnabled
import com.facebook.react.defaults.DefaultReactActivityDelegate

class MainActivity : ReactActivity() {
  companion object {
    private const val E2E_CAPABILITY_STATE = "APGIC_E2E_CAPABILITY_STATE"
    private const val E2E_INSTALLATION_BASE_URL = "APGIC_E2E_INSTALLATION_BASE_URL"
    private const val E2E_SESSION_COOKIE = "APGIC_E2E_SESSION_COOKIE"
    private const val E2E_INSTALLATION_ID = "APGIC_E2E_INSTALLATION_ID"
    private const val E2E_INSTALLATION_PLATFORM = "APGIC_E2E_INSTALLATION_PLATFORM"
    private const val E2E_DEEP_LINK_BASE_URL = "APGIC_E2E_DEEP_LINK_BASE_URL"
    private const val E2E_DEEP_LINK_SESSION_COOKIE = "APGIC_E2E_DEEP_LINK_SESSION_COOKIE"
    private const val E2E_DEEP_LINK_URL = "APGIC_E2E_DEEP_LINK_URL"
    private const val E2E_NOTIFICATION_BASE_URL = "APGIC_E2E_NOTIFICATION_BASE_URL"
    private const val E2E_NOTIFICATION_SESSION_COOKIE = "APGIC_E2E_NOTIFICATION_SESSION_COOKIE"
    private const val E2E_NOTIFICATION_DELIVERY_ID = "APGIC_E2E_NOTIFICATION_DELIVERY_ID"
    private const val E2E_NOTIFICATION_INTENT_ID = "APGIC_E2E_NOTIFICATION_INTENT_ID"
    private const val E2E_OFFLINE_MUTATION_BASE_URL = "APGIC_E2E_OFFLINE_MUTATION_BASE_URL"
    private const val E2E_OFFLINE_MUTATION_SESSION_COOKIE = "APGIC_E2E_OFFLINE_MUTATION_SESSION_COOKIE"
    private const val E2E_OFFLINE_MUTATION_HOLD_ID = "APGIC_E2E_OFFLINE_MUTATION_HOLD_ID"
    private const val E2E_OFFLINE_MUTATION_IDEMPOTENCY_KEY = "APGIC_E2E_OFFLINE_MUTATION_IDEMPOTENCY_KEY"
    private const val E2E_OFFLINE_MUTATION_METHOD_CODE = "APGIC_E2E_OFFLINE_MUTATION_METHOD_CODE"
    private const val E2E_REALTIME_EVENTS = "APGIC_E2E_REALTIME_EVENTS"
    private const val E2E_REALTIME_RECONNECT_FAILURES = "APGIC_E2E_REALTIME_RECONNECT_FAILURES"
    private const val E2E_REALTIME_CONSULTATION_ID = "APGIC_E2E_REALTIME_CONSULTATION_ID"
    private const val E2E_COMPATIBILITY_BASE_URL = "APGIC_E2E_COMPATIBILITY_BASE_URL"
    private const val E2E_APP_VERSION = "APGIC_E2E_APP_VERSION"
    private const val E2E_BUILD_NUMBER = "APGIC_E2E_BUILD_NUMBER"
    private const val E2E_CONTRACT_VERSION = "APGIC_E2E_CONTRACT_VERSION"
    private val CAPABILITY_STATES = setOf(
      "UNKNOWN",
      "NOT_REQUESTED",
      "GRANTED",
      "DENIED",
      "RESTRICTED",
      "UNAVAILABLE",
    )
  }

  override fun getMainComponentName(): String = "APGIC"

  private fun microphoneState(): String {
    if (BuildConfig.DEBUG) {
      intent?.getStringExtra(E2E_CAPABILITY_STATE)
        ?.takeIf(CAPABILITY_STATES::contains)
        ?.let { return it }
    }

    if (!packageManager.hasSystemFeature(PackageManager.FEATURE_MICROPHONE)) {
      return "UNAVAILABLE"
    }

    return when {
      checkSelfPermission(Manifest.permission.RECORD_AUDIO) == PackageManager.PERMISSION_GRANTED ->
        "GRANTED"
      shouldShowRequestPermissionRationale(Manifest.permission.RECORD_AUDIO) ->
        "DENIED"
      else ->
        "NOT_REQUESTED"
    }
  }

  override fun createReactActivityDelegate(): ReactActivityDelegate =
    object : DefaultReactActivityDelegate(this, mainComponentName, fabricEnabled) {
      override fun getLaunchOptions(): Bundle =
        Bundle().apply {
          putString("deviceCapability", "MICROPHONE")
          putString("deviceCapabilityState", microphoneState())
          putString("compatibilityPlatform", "ANDROID")
          putString("appVersion", BuildConfig.VERSION_NAME)
          putString("buildNumber", BuildConfig.VERSION_CODE.toString())
          if (BuildConfig.DEBUG) {
            intent?.getStringExtra(E2E_INSTALLATION_BASE_URL)
              ?.takeIf { it.isNotBlank() }
              ?.let { putString("installationE2EBaseURL", it) }
            intent?.getStringExtra(E2E_SESSION_COOKIE)
              ?.takeIf { it.isNotBlank() }
              ?.let { putString("installationE2ESessionCookie", it) }
            intent?.getStringExtra(E2E_INSTALLATION_ID)
              ?.takeIf { it.isNotBlank() }
              ?.let { putString("installationE2EInstallationID", it) }
            intent?.getStringExtra(E2E_INSTALLATION_PLATFORM)
              ?.takeIf { it == "IOS" || it == "ANDROID" }
              ?.let { putString("installationE2EPlatform", it) }
            intent?.getStringExtra(E2E_DEEP_LINK_BASE_URL)
              ?.takeIf { it.isNotBlank() }
              ?.let { putString("deepLinkAPIBaseURL", it) }
            intent?.getStringExtra(E2E_DEEP_LINK_SESSION_COOKIE)
              ?.takeIf { it.isNotBlank() }
              ?.let { putString("deepLinkE2ESessionCookie", it) }
            intent?.getStringExtra(E2E_DEEP_LINK_URL)
              ?.takeIf { it.isNotBlank() }
              ?.let { putString("deepLinkE2EURL", it) }
            intent?.getStringExtra(E2E_NOTIFICATION_BASE_URL)
              ?.takeIf { it.isNotBlank() }
              ?.let { putString("notificationE2EBaseURL", it) }
            intent?.getStringExtra(E2E_NOTIFICATION_SESSION_COOKIE)
              ?.takeIf { it.isNotBlank() }
              ?.let { putString("notificationE2ESessionCookie", it) }
            intent?.getStringExtra(E2E_NOTIFICATION_DELIVERY_ID)
              ?.takeIf { it.isNotBlank() }
              ?.let { putString("notificationE2EDeliveryID", it) }
            intent?.getStringExtra(E2E_NOTIFICATION_INTENT_ID)
              ?.takeIf { it.isNotBlank() }
              ?.let { putString("notificationE2EIntentID", it) }
            intent?.getStringExtra(E2E_OFFLINE_MUTATION_BASE_URL)
              ?.takeIf { it.isNotBlank() }
              ?.let { putString("offlineMutationE2EBaseURL", it) }
            intent?.getStringExtra(E2E_OFFLINE_MUTATION_SESSION_COOKIE)
              ?.takeIf { it.isNotBlank() }
              ?.let { putString("offlineMutationE2ESessionCookie", it) }
            intent?.getStringExtra(E2E_OFFLINE_MUTATION_HOLD_ID)
              ?.takeIf { it.isNotBlank() }
              ?.let { putString("offlineMutationE2EHoldID", it) }
            intent?.getStringExtra(E2E_OFFLINE_MUTATION_IDEMPOTENCY_KEY)
              ?.takeIf { it.isNotBlank() }
              ?.let { putString("offlineMutationE2EIdempotencyKey", it) }
            intent?.getStringExtra(E2E_OFFLINE_MUTATION_METHOD_CODE)
              ?.takeIf { it.isNotBlank() }
              ?.let { putString("offlineMutationE2EMethodCode", it) }
            intent?.getStringExtra(E2E_REALTIME_EVENTS)
              ?.takeIf { it.isNotBlank() }
              ?.let { putString("realtimeE2EEvents", it) }
            intent?.getStringExtra(E2E_REALTIME_RECONNECT_FAILURES)
              ?.takeIf { it.isNotBlank() }
              ?.let { putString("realtimeE2EReconnectFailures", it) }
            intent?.getStringExtra(E2E_REALTIME_CONSULTATION_ID)
              ?.takeIf { it.isNotBlank() }
              ?.let { putString("realtimeE2EConsultationID", it) }
            intent?.getStringExtra(E2E_COMPATIBILITY_BASE_URL)
              ?.takeIf { it.isNotBlank() }
              ?.let { putString("compatibilityBaseURL", it) }
            intent?.getStringExtra(E2E_APP_VERSION)
              ?.takeIf { it.isNotBlank() }
              ?.let { putString("appVersion", it) }
            intent?.getStringExtra(E2E_BUILD_NUMBER)
              ?.takeIf { it.isNotBlank() }
              ?.let { putString("buildNumber", it) }
            intent?.getStringExtra(E2E_CONTRACT_VERSION)
              ?.takeIf { it.isNotBlank() }
              ?.let { putString("compatibilityContractVersion", it) }
          }
        }
    }
}