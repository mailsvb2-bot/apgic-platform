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
    r"\b(?:UserDefaults|NSKeyedArchiver|CoreData|NSPersistentContainer|"
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
