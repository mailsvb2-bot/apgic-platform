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
          }
        }
    }
}
