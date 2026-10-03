#!/usr/bin/env python3
from __future__ import annotations

import json
import re
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]

JS_STORAGE = re.compile(
    r"\b(?:AsyncStorage|MMKV|localStorage|sessionStorage|"
    r"react-native-mmkv|@react-native-async-storage/async-storage)\b",
    re.I,
)
ANDROID_STORAGE = re.compile(
    r"\b(?:SharedPreferences|getSharedPreferences|SQLiteDatabase|RoomDatabase|"
    r"openFileOutput|FileOutputStream)\b"
)
IOS_STORAGE = re.compile(
    r"\b(?:UserDefaults|NSUserDefaults|NSKeyedArchiver|CoreData|NSPersistentContainer|"
    r"FileManager\.default|sqlite3_)\b"
)
SENSITIVE_MARKERS = re.compile(
    r"\b(?:RAW_CONSULTATION|RAW_PERSONA|FINANCIAL_EVIDENCE|CREDENTIAL)\b"
)
FORBIDDEN_RUNTIME_DEPENDENCIES = {
    "@react-native-async-storage/async-storage",
    "react-native-mmkv",
}

AUDITED_NATIVE_STORAGE_ADAPTERS = {
    "apps/mobile/android/app/src/main/java/com/apgic/ci/OfflineMutationStorageModule.kt":
        "APGIC_AUDITED_STORAGE_ADAPTER: INTERNAL_OFFLINE_MUTATION_QUEUE",
    "apps/mobile/ios/APGIC/OfflineMutationStorage.m":
        "APGIC_AUDITED_STORAGE_ADAPTER: INTERNAL_OFFLINE_MUTATION_QUEUE",
}

SECURE_CREDENTIAL_ADAPTER_REQUIREMENTS = {
    "apps/mobile/android/app/src/main/java/com/apgic/ci/OfflineMutationStorageModule.kt": (
        "APGIC_SECURE_CREDENTIAL_ADAPTER: SYSTEM_KEYSTORE_V1",
        "AndroidKeyStore",
        "AES/GCM/NoPadding",
        "KeyGenParameterSpec",
        "saveCredential",
        "loadCredential",
        "clearCredential",
        "clearUserScopedState",
        "4096",
    ),
    "apps/mobile/ios/APGIC/OfflineMutationStorage.m": (
        "APGIC_SECURE_CREDENTIAL_ADAPTER: SYSTEM_KEYSTORE_V1",
        "kSecClassGenericPassword",
        "SecItemAdd",
        "SecItemUpdate",
        "SecItemCopyMatching",
        "SecItemDelete",
        "kSecAttrAccessibleWhenUnlockedThisDeviceOnly",
        "saveCredential",
        "loadCredential",
        "clearCredential",
        "clearUserScopedState",
        "4096",
    ),
}


def scan_files(root: Path, base: str, suffixes: set[str], pattern: re.Pattern[str]) -> list[str]:
    errors: list[str] = []
    folder = root / base
    if not folder.exists():
        return errors
    for path in folder.rglob("*"):
        if not path.is_file() or path.suffix.lower() not in suffixes:
            continue
        text = path.read_text(encoding="utf-8", errors="ignore")
        match = pattern.search(text)
        if match:
            rel = path.relative_to(root).as_posix()
            marker = AUDITED_NATIVE_STORAGE_ADAPTERS.get(rel)
            if marker is not None:
                if marker not in text:
                    errors.append(f"{rel}: audited storage adapter marker missing")
                if SENSITIVE_MARKERS.search(text):
                    errors.append(
                        f"{rel}: audited INTERNAL offline queue adapter must not reference sensitive DataClass markers"
                    )
                if "8192" not in text:
                    errors.append(f"{rel}: audited offline queue adapter must enforce the 8192-byte payload bound")
                continue
            line = text.count("\n", 0, match.start()) + 1
            errors.append(
                f"{rel}:{line}: direct native/local persistence "
                "is forbidden until routed through an audited storage adapter and DataClass policy"
            )
    return errors


def braced_body(text: str, marker: str) -> str:
    start = text.find(marker)
    if start < 0:
        return ""
    brace = text.find("{", start)
    if brace < 0:
        return ""
    depth = 0
    for index in range(brace, len(text)):
        char = text[index]
        if char == "{":
            depth += 1
        elif char == "}":
            depth -= 1
            if depth == 0:
                return text[brace + 1:index]
    return ""


def require_body_tokens(rel: str, body_name: str, body: str, tokens: tuple[str, ...]) -> list[str]:
    if not body:
        return [f"{rel}: secure credential method body missing: {body_name}"]
    missing = [token for token in tokens if token not in body]
    if missing:
        return [f"{rel}: {body_name} missing executable secure-storage operations: {missing}"]
    return []


def validate_secure_credential_adapters(root: Path) -> list[str]:
    errors: list[str] = []
    for rel, required in SECURE_CREDENTIAL_ADAPTER_REQUIREMENTS.items():
        path = root / rel
        if not path.is_file():
            errors.append(f"{rel}: secure credential adapter missing")
            continue
        text = path.read_text(encoding="utf-8", errors="ignore")
        missing = [token for token in required if token not in text]
        if missing:
            errors.append(
                f"{rel}: secure credential adapter missing required system-storage proof: {missing}"
            )

        if rel.endswith(".kt"):
            save = braced_body(text, "fun saveCredential")
            load = braced_body(text, "fun loadCredential")
            clear = braced_body(text, "fun clearCredential")
            purge = braced_body(text, "fun clearUserScopedState")
            errors.extend(require_body_tokens(
                rel, "saveCredential", save,
                ("Cipher.getInstance(\"AES/GCM/NoPadding\")", "Cipher.ENCRYPT_MODE", "credentialKey()", "cipher.doFinal", "putString(CREDENTIAL_KEY, \"$iv.$ciphertext\")"),
            ))
            errors.extend(require_body_tokens(
                rel, "loadCredential", load,
                ("Cipher.getInstance(\"AES/GCM/NoPadding\")", "Cipher.DECRYPT_MODE", "GCMParameterSpec", "cipher.doFinal"),
            ))
            errors.extend(require_body_tokens(rel, "clearCredential", clear, ("deleteCredentialMaterial()",)))
            errors.extend(require_body_tokens(
                rel, "clearUserScopedState", purge,
                ("remove(QUEUE_KEY)", "deleteCredentialMaterial()"),
            ))
            if 'putString(CREDENTIAL_KEY, value)' in save:
                errors.append(f"{rel}: credential plaintext must never be written to SharedPreferences")
        else:
            save = braced_body(text, "RCT_REMAP_METHOD(saveCredential")
            load = braced_body(text, "RCT_REMAP_METHOD(loadCredential")
            clear = braced_body(text, "RCT_REMAP_METHOD(clearCredential")
            purge = braced_body(text, "RCT_REMAP_METHOD(clearUserScopedState")
            errors.extend(require_body_tokens(
                rel, "saveCredential", save,
                ("SecItemUpdate", "errSecItemNotFound", "SecItemAdd", "kSecValueData", "kSecAttrAccessibleWhenUnlockedThisDeviceOnly"),
            ))
            errors.extend(require_body_tokens(
                rel, "loadCredential", load,
                ("SecItemCopyMatching", "kSecReturnData", "kSecMatchLimitOne"),
            ))
            errors.extend(require_body_tokens(rel, "clearCredential", clear, ("APGICDeleteCredential()",)))
            errors.extend(require_body_tokens(
                rel, "clearUserScopedState", purge,
                ("removeObjectForKey:APGICOfflineMutationQueueKey", "APGICDeleteCredential()"),
            ))
            if "setObject:value forKey:APGICCredential" in save:
                errors.append(f"{rel}: credential plaintext must never be written to NSUserDefaults")
    return errors


def validate_native_storage(root: Path) -> list[str]:
    errors: list[str] = []

    package_path = root / "apps/mobile/package.json"
    if not package_path.is_file():
        errors.append("apps/mobile/package.json: native package manifest missing")
    else:
        package = json.loads(package_path.read_text(encoding="utf-8"))
        dependencies = set(package.get("dependencies", {}))
        forbidden = sorted(dependencies & FORBIDDEN_RUNTIME_DEPENDENCIES)
        if forbidden:
            errors.append(
                "apps/mobile/package.json: direct persistent-storage runtime dependencies require "
                f"an approved storage adapter first: {forbidden}"
            )

    errors.extend(scan_files(root, "apps/mobile/src", {".ts", ".tsx", ".js", ".jsx"}, JS_STORAGE))
    errors.extend(scan_files(root, "apps/mobile/android/app/src/main", {".kt", ".java"}, ANDROID_STORAGE))
    errors.extend(scan_files(root, "apps/mobile/ios/APGIC", {".swift", ".m", ".mm"}, IOS_STORAGE))
    errors.extend(validate_secure_credential_adapters(root))

    manifest = root / "apps/mobile/android/app/src/main/AndroidManifest.xml"
    if not manifest.is_file():
        errors.append("apps/mobile/android/app/src/main/AndroidManifest.xml: Android manifest missing")
    else:
        text = manifest.read_text(encoding="utf-8", errors="ignore")
        if 'android:allowBackup="false"' not in text:
            errors.append("Android manifest must disable application backup for R0 native storage boundary")

    policy = root / "apps/mobile/src/storage-policy.ts"
    if not policy.is_file():
        errors.append("apps/mobile/src/storage-policy.ts: canonical native storage policy missing")
    else:
        text = policy.read_text(encoding="utf-8", errors="ignore")
        for marker in ("RAW_CONSULTATION", "RAW_PERSONA", "FINANCIAL_EVIDENCE", "CREDENTIAL"):
            if marker not in text:
                errors.append(f"storage policy missing sensitive DataClass {marker}")
        if 'target === "SECURE_STORAGE"' not in text:
            errors.append("storage policy must reserve SECURE_STORAGE for credentials")

    secure_bridge = root / "apps/mobile/src/secure-local-storage.ts"
    if not secure_bridge.is_file():
        errors.append("apps/mobile/src/secure-local-storage.ts: secure credential bridge missing")
    else:
        text = secure_bridge.read_text(encoding="utf-8", errors="ignore")
        for token in ("loadCredential", "saveCredential", "clearCredential", "clearUserScopedState"):
            if token not in text:
                errors.append(f"secure credential bridge missing {token}")

    return errors


def main() -> int:
    errors = validate_native_storage(ROOT)
    if errors:
        print("NATIVE STORAGE AUDIT: FAIL")
        for error in errors:
            print(f"ERROR: {error}")
        return 1
    print("NATIVE STORAGE AUDIT: PASS")
    return 0


if __name__ == "__main__":
    sys.exit(main())
