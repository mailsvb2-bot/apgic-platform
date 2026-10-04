package com.apgic.ci

import android.security.keystore.KeyGenParameterSpec
import android.security.keystore.KeyProperties
import android.util.Base64
import com.facebook.react.bridge.Promise
import com.facebook.react.bridge.ReactApplicationContext
import com.facebook.react.bridge.ReactContextBaseJavaModule
import com.facebook.react.bridge.ReactMethod
import java.security.KeyStore
import javax.crypto.Cipher
import javax.crypto.KeyGenerator
import javax.crypto.SecretKey
import javax.crypto.spec.GCMParameterSpec

// APGIC_AUDITED_STORAGE_ADAPTER: INTERNAL_OFFLINE_MUTATION_QUEUE
// APGIC_SECURE_CREDENTIAL_ADAPTER: SYSTEM_KEYSTORE_V1
class OfflineMutationStorageModule(
  reactContext: ReactApplicationContext,
) : ReactContextBaseJavaModule(reactContext) {
  companion object {
    const val NAME = "APGICOfflineMutationStorage"
    private const val PREFS_NAME = "apgic_offline_mutation_queue"
    private const val CREDENTIAL_PREFS_NAME = "apgic_secure_credentials"
    private const val QUEUE_KEY = "queue_v1"
    private const val REMOTE_CONFIG_KEY = "remote_config_v1"
    private const val CREDENTIAL_KEY = "session_credential_v1"
    private const val CREDENTIAL_KEY_ALIAS = "apgic.session.credential.v1"
    private const val MAX_BYTES = 8192
    private const val REMOTE_CONFIG_MAX_BYTES = 16384
    private const val CREDENTIAL_MAX_BYTES = 4096
  }

  override fun getName(): String = NAME

  private fun preferences() =
    reactApplicationContext.getSharedPreferences(PREFS_NAME, 0)

  private fun credentialPreferences() =
    reactApplicationContext.getSharedPreferences(CREDENTIAL_PREFS_NAME, 0)

  private fun credentialKey(): SecretKey {
    val keyStore = KeyStore.getInstance("AndroidKeyStore")
    keyStore.load(null)
    val existing = keyStore.getKey(CREDENTIAL_KEY_ALIAS, null) as? SecretKey
    if (existing != null) {
      return existing
    }

    val generator = KeyGenerator.getInstance(
      KeyProperties.KEY_ALGORITHM_AES,
      "AndroidKeyStore",
    )
    generator.init(
      KeyGenParameterSpec.Builder(
        CREDENTIAL_KEY_ALIAS,
        KeyProperties.PURPOSE_ENCRYPT or KeyProperties.PURPOSE_DECRYPT,
      )
        .setBlockModes(KeyProperties.BLOCK_MODE_GCM)
        .setEncryptionPaddings(KeyProperties.ENCRYPTION_PADDING_NONE)
        .build(),
    )
    return generator.generateKey()
  }

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

  @ReactMethod
  fun loadRemoteConfig(promise: Promise) {
    promise.resolve(preferences().getString(REMOTE_CONFIG_KEY, null))
  }

  @ReactMethod
  fun saveRemoteConfig(value: String, promise: Promise) {
    if (value.toByteArray(Charsets.UTF_8).size > REMOTE_CONFIG_MAX_BYTES) {
      promise.reject("REMOTE_CONFIG_STORAGE_TOO_LARGE", "Remote config exceeds audited size bound")
      return
    }
    if (!preferences().edit().putString(REMOTE_CONFIG_KEY, value).commit()) {
      promise.reject("REMOTE_CONFIG_STORAGE_WRITE_FAILED", "Remote config could not be persisted")
      return
    }
    promise.resolve(null)
  }

  @ReactMethod
  fun clearRemoteConfig(promise: Promise) {
    if (!preferences().edit().remove(REMOTE_CONFIG_KEY).commit()) {
      promise.reject("REMOTE_CONFIG_STORAGE_CLEAR_FAILED", "Remote config could not be cleared")
      return
    }
    promise.resolve(null)
  }

  @ReactMethod
  fun saveCredential(value: String, promise: Promise) {
    try {
      if (value.toByteArray(Charsets.UTF_8).size > CREDENTIAL_MAX_BYTES) {
        promise.reject("SECURE_CREDENTIAL_TOO_LARGE", "Credential exceeds audited size bound")
        return
      }
      val cipher = Cipher.getInstance("AES/GCM/NoPadding")
      cipher.init(Cipher.ENCRYPT_MODE, credentialKey())
      val iv = Base64.encodeToString(cipher.iv, Base64.NO_WRAP)
      val ciphertext = Base64.encodeToString(
        cipher.doFinal(value.toByteArray(Charsets.UTF_8)),
        Base64.NO_WRAP,
      )
      if (!credentialPreferences().edit().putString(CREDENTIAL_KEY, "$iv.$ciphertext").commit()) {
        promise.reject("SECURE_CREDENTIAL_WRITE_FAILED", "Credential ciphertext could not be persisted")
        return
      }
      promise.resolve(null)
    } catch (error: Exception) {
      promise.reject("SECURE_CREDENTIAL_WRITE_FAILED", error)
    }
  }

  @ReactMethod
  fun loadCredential(promise: Promise) {
    try {
      val encoded = credentialPreferences().getString(CREDENTIAL_KEY, null)
      if (encoded == null) {
        promise.resolve(null)
        return
      }
      val parts = encoded.split(".", limit = 2)
      if (parts.size != 2) {
        promise.reject("SECURE_CREDENTIAL_CORRUPT", "Credential ciphertext is malformed")
        return
      }
      val cipher = Cipher.getInstance("AES/GCM/NoPadding")
      cipher.init(
        Cipher.DECRYPT_MODE,
        credentialKey(),
        GCMParameterSpec(128, Base64.decode(parts[0], Base64.NO_WRAP)),
      )
      val plaintext = cipher.doFinal(Base64.decode(parts[1], Base64.NO_WRAP))
      promise.resolve(String(plaintext, Charsets.UTF_8))
    } catch (error: Exception) {
      promise.reject("SECURE_CREDENTIAL_READ_FAILED", error)
    }
  }

  private fun deleteCredentialMaterial(): Boolean {
    val removed = credentialPreferences().edit().remove(CREDENTIAL_KEY).commit()
    val keyStore = KeyStore.getInstance("AndroidKeyStore")
    keyStore.load(null)
    if (keyStore.containsAlias(CREDENTIAL_KEY_ALIAS)) {
      keyStore.deleteEntry(CREDENTIAL_KEY_ALIAS)
    }
    return removed
  }

  @ReactMethod
  fun clearCredential(promise: Promise) {
    try {
      if (!deleteCredentialMaterial()) {
        promise.reject("SECURE_CREDENTIAL_CLEAR_FAILED", "Credential ciphertext could not be cleared")
        return
      }
      promise.resolve(null)
    } catch (error: Exception) {
      promise.reject("SECURE_CREDENTIAL_CLEAR_FAILED", error)
    }
  }

  @ReactMethod
  fun clearUserScopedState(promise: Promise) {
    var queueCleared = false
    var credentialCleared = false
    try {
      queueCleared = preferences().edit().remove(QUEUE_KEY).commit()
      credentialCleared = deleteCredentialMaterial()
    } catch (error: Exception) {
      promise.reject("LOCAL_USER_STATE_CLEAR_FAILED", error)
      return
    }
    if (!queueCleared || !credentialCleared) {
      promise.reject("LOCAL_USER_STATE_CLEAR_FAILED", "User-scoped local state could not be fully cleared")
      return
    }
    promise.resolve(null)
  }
}
