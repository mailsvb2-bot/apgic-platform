package com.apgic.ci

import com.facebook.react.bridge.Promise
import com.facebook.react.bridge.ReactApplicationContext
import com.facebook.react.bridge.ReactContextBaseJavaModule
import com.facebook.react.bridge.ReactMethod

// APGIC_AUDITED_STORAGE_ADAPTER: INTERNAL_OFFLINE_MUTATION_QUEUE
class OfflineMutationStorageModule(
  reactContext: ReactApplicationContext,
) : ReactContextBaseJavaModule(reactContext) {
  companion object {
    const val NAME = "APGICOfflineMutationStorage"
    private const val PREFS_NAME = "apgic_offline_mutation_queue"
    private const val QUEUE_KEY = "queue_v1"
    private const val MAX_BYTES = 8192
  }

  override fun getName(): String = NAME

  private fun preferences() =
    reactApplicationContext.getSharedPreferences(PREFS_NAME, 0)

  @ReactMethod
  fun load(promise: Promise) {
    promise.resolve(preferences().getString(QUEUE_KEY, null))
  }

  @ReactMethod
  fun save(value: String, promise: Promise) {
    if (value.toByteArray(Charsets.UTF_8).size > MAX_BYTES) {
      promise.reject("OFFLINE_MUTATION_STORAGE_TOO_LARGE", "Offline mutation queue exceeds audited size bound")
      return
    }
    if (!preferences().edit().putString(QUEUE_KEY, value).commit()) {
      promise.reject("OFFLINE_MUTATION_STORAGE_WRITE_FAILED", "Offline mutation queue could not be persisted")
      return
    }
    promise.resolve(null)
  }

  @ReactMethod
  fun clear(promise: Promise) {
    if (!preferences().edit().remove(QUEUE_KEY).commit()) {
      promise.reject("OFFLINE_MUTATION_STORAGE_CLEAR_FAILED", "Offline mutation queue could not be cleared")
      return
    }
    promise.resolve(null)
  }
}
