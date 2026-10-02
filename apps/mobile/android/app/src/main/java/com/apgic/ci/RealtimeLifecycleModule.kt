package com.apgic.ci

import android.Manifest
import android.content.Context
import android.content.pm.PackageManager
import android.media.AudioDeviceCallback
import android.media.AudioDeviceInfo
import android.media.AudioManager
import android.net.ConnectivityManager
import android.net.Network
import android.net.NetworkCapabilities
import com.facebook.react.bridge.Arguments
import com.facebook.react.bridge.LifecycleEventListener
import com.facebook.react.bridge.Promise
import com.facebook.react.bridge.ReactApplicationContext
import com.facebook.react.bridge.ReactContextBaseJavaModule
import com.facebook.react.bridge.ReactMethod
import com.facebook.react.modules.core.DeviceEventManagerModule

class RealtimeLifecycleModule(
  reactContext: ReactApplicationContext,
) : ReactContextBaseJavaModule(reactContext), LifecycleEventListener {
  companion object {
    const val NAME = "APGICRealtimeLifecycle"
    private const val EVENT = "APGICRealtimeLifecycleEvent"
  }

  private val connectivity = reactContext.getSystemService(Context.CONNECTIVITY_SERVICE) as ConnectivityManager
  private val audio = reactContext.getSystemService(Context.AUDIO_SERVICE) as AudioManager
  private var started = false
  private var interrupted = false

  private val networkCallback = object : ConnectivityManager.NetworkCallback() {
    override fun onAvailable(network: Network) = emitNetwork(network)
    override fun onCapabilitiesChanged(network: Network, capabilities: NetworkCapabilities) = emitNetwork(capabilities)
    override fun onLost(network: Network) = emit("NETWORK_OFFLINE")
  }

  private val audioDeviceCallback = object : AudioDeviceCallback() {
    override fun onAudioDevicesAdded(addedDevices: Array<out AudioDeviceInfo>?) = emitAudioRoute()
    override fun onAudioDevicesRemoved(removedDevices: Array<out AudioDeviceInfo>?) = emitAudioRoute()
  }

  private val focusListener = AudioManager.OnAudioFocusChangeListener { change ->
    when (change) {
      AudioManager.AUDIOFOCUS_LOSS,
      AudioManager.AUDIOFOCUS_LOSS_TRANSIENT,
      AudioManager.AUDIOFOCUS_LOSS_TRANSIENT_CAN_DUCK -> {
        if (!interrupted) {
          interrupted = true
          emit("INTERRUPTION_BEGAN")
        }
      }
      AudioManager.AUDIOFOCUS_GAIN -> {
        if (interrupted) {
          interrupted = false
          emit("INTERRUPTION_ENDED")
        }
      }
    }
  }

  override fun getName(): String = NAME

  @ReactMethod
  fun addListener(eventName: String) = Unit

  @ReactMethod
  fun removeListeners(count: Double) = Unit

  @ReactMethod
  fun start(promise: Promise) {
    if (started) {
      promise.resolve(null)
      return
    }
    try {
      started = true
      reactApplicationContext.addLifecycleEventListener(this)
      connectivity.registerDefaultNetworkCallback(networkCallback)
      audio.registerAudioDeviceCallback(audioDeviceCallback, null)
      @Suppress("DEPRECATION")
      audio.requestAudioFocus(focusListener, AudioManager.STREAM_VOICE_CALL, AudioManager.AUDIOFOCUS_GAIN_TRANSIENT)
      emitMicrophonePermission()
      connectivity.activeNetwork?.let(::emitNetwork) ?: emit("NETWORK_OFFLINE")
      emitAudioRoute()
      promise.resolve(null)
    } catch (error: Exception) {
      started = false
      promise.reject("REALTIME_LIFECYCLE_START_FAILED", error)
    }
  }

  @ReactMethod
  fun stop(promise: Promise) {
    stopNative()
    promise.resolve(null)
  }

  @ReactMethod
  fun debugEmit(type: String, route: String?, promise: Promise) {
    if (!BuildConfig.DEBUG) {
      promise.reject("REALTIME_DEBUG_DISABLED", "Debug lifecycle injection is disabled")
      return
    }
    if (type == "AUDIO_ROUTE_CHANGED") {
      emit(type, route ?: "UNKNOWN")
    } else {
      emit(type)
    }
    promise.resolve(null)
  }

  override fun onHostResume() {
    emit("APP_FOREGROUND")
    emitMicrophonePermission()
  }

  override fun onHostPause() = emit("APP_BACKGROUND")
  override fun onHostDestroy() = stopNative()

  private fun emitMicrophonePermission() {
    if (reactApplicationContext.checkSelfPermission(Manifest.permission.RECORD_AUDIO) == PackageManager.PERMISSION_GRANTED) {
      emit("MICROPHONE_PERMISSION_GRANTED")
    } else {
      emit("MICROPHONE_PERMISSION_REVOKED")
    }
  }

  private fun stopNative() {
    if (!started) return
    started = false
    reactApplicationContext.removeLifecycleEventListener(this)
    runCatching { connectivity.unregisterNetworkCallback(networkCallback) }
    audio.unregisterAudioDeviceCallback(audioDeviceCallback)
    @Suppress("DEPRECATION")
    audio.abandonAudioFocus(focusListener)
  }

  private fun emitNetwork(network: Network) {
    val capabilities = connectivity.getNetworkCapabilities(network)
    if (capabilities == null) emit("NETWORK_OFFLINE") else emitNetwork(capabilities)
  }

  private fun emitNetwork(capabilities: NetworkCapabilities) {
    when {
      !capabilities.hasCapability(NetworkCapabilities.NET_CAPABILITY_INTERNET) -> emit("NETWORK_OFFLINE")
      capabilities.hasCapability(NetworkCapabilities.NET_CAPABILITY_VALIDATED) -> emit("NETWORK_ONLINE")
      else -> emit("NETWORK_DEGRADED")
    }
  }

  private fun emitAudioRoute() {
    val route = audio.getDevices(AudioManager.GET_DEVICES_OUTPUTS).let { devices ->
      when {
        devices.any { it.type == AudioDeviceInfo.TYPE_BLUETOOTH_A2DP || it.type == AudioDeviceInfo.TYPE_BLUETOOTH_SCO } -> "BLUETOOTH"
        devices.any { it.type == AudioDeviceInfo.TYPE_WIRED_HEADPHONES || it.type == AudioDeviceInfo.TYPE_WIRED_HEADSET || it.type == AudioDeviceInfo.TYPE_USB_HEADSET } -> "WIRED"
        devices.any { it.type == AudioDeviceInfo.TYPE_BUILTIN_EARPIECE } -> "EARPIECE"
        devices.any { it.type == AudioDeviceInfo.TYPE_BUILTIN_SPEAKER } -> "SPEAKER"
        else -> "UNKNOWN"
      }
    }
    emit("AUDIO_ROUTE_CHANGED", route)
  }

  private fun emit(type: String, route: String? = null) {
    if (!started && !BuildConfig.DEBUG) return
    val payload = Arguments.createMap().apply {
      putString("type", type)
      route?.let { putString("route", it) }
    }
    reactApplicationContext
      .getJSModule(DeviceEventManagerModule.RCTDeviceEventEmitter::class.java)
      .emit(EVENT, payload)
  }
}