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
        }
    }
}
