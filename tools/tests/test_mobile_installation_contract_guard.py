import copy
import json
import unittest

from tools.mobile_installation_contract_guard import (
    ANDROID_BRIDGE,
    ANDROID_E2E,
    GO_INSTALLATION,
    HTTP_API,
    IOS_BRIDGE,
    IOS_E2E,
    MIGRATION,
    NATIVE_CLIENT,
    OPENAPI,
    POSTGRES_STORE,
    SCHEMA,
    validate_mobile_installation_contract,
    validate_mobile_installation_native_e2e,
    validate_mobile_installation_runtime,
)


class MobileInstallationContractGuardTest(unittest.TestCase):
    def setUp(self):
        self.go = GO_INSTALLATION.read_text(encoding="utf-8")
        self.migration = MIGRATION.read_text(encoding="utf-8")
        self.schema = json.loads(SCHEMA.read_text(encoding="utf-8"))
        self.http = HTTP_API.read_text(encoding="utf-8")
        self.store = POSTGRES_STORE.read_text(encoding="utf-8")
        self.openapi = OPENAPI.read_text(encoding="utf-8")
        self.native_client = NATIVE_CLIENT.read_text(encoding="utf-8")
        self.android_bridge = ANDROID_BRIDGE.read_text(encoding="utf-8")
        self.ios_bridge = IOS_BRIDGE.read_text(encoding="utf-8")
        self.android_e2e = ANDROID_E2E.read_text(encoding="utf-8")
        self.ios_e2e = IOS_E2E.read_text(encoding="utf-8")

    def test_repository_contract_matches_go_and_sql(self):
        self.assertEqual(validate_mobile_installation_contract(self.go, self.migration, self.schema), [])

    def test_runtime_path_is_contracted(self):
        self.assertEqual(
            validate_mobile_installation_runtime(self.http, self.store, self.openapi),
            [],
        )

    def test_installed_app_native_e2e_path_is_contracted(self):
        self.assertEqual(
            validate_mobile_installation_native_e2e(
                self.native_client,
                self.android_bridge,
                self.ios_bridge,
                self.android_e2e,
                self.ios_e2e,
            ),
            [],
        )

    def test_native_e2e_guard_rejects_non_debug_bridge_or_missing_lifecycle(self):
        broken_android = self.android_bridge.replace("BuildConfig.DEBUG", "true")
        broken_ios = self.ios_e2e.replace("installation-e2e:PASS", "installation-e2e:SKIPPED")
        errors = validate_mobile_installation_native_e2e(
            self.native_client,
            broken_android,
            self.ios_bridge,
            self.android_e2e,
            broken_ios,
        )
        self.assertTrue(any("debug-only" in error for error in errors))
        self.assertTrue(any("iOS installed-app E2E proof missing" in error for error in errors))

    def test_native_e2e_guard_rejects_separate_test_only_http_implementation(self):
        broken_client = self.native_client.replace(
            "registerMobileInstallation(\n    clientConfig,",
            "testOnlyRegister(\n    clientConfig,",
        )
        errors = validate_mobile_installation_native_e2e(
            broken_client,
            self.android_bridge,
            self.ios_bridge,
            self.android_e2e,
            self.ios_e2e,
        )
        self.assertTrue(
            any("E2E must compose production mobile installation functions" in error for error in errors)
        )

    def test_runtime_guard_requires_trusted_session_and_serialized_store(self):
        broken_http = self.http.replace("requiredClientSessionIdentity", "callerSuppliedIdentity")
        broken_store = self.store.replace("pg_advisory_xact_lock", "missing_lock")
        errors = validate_mobile_installation_runtime(broken_http, broken_store, self.openapi)
        self.assertTrue(any("HTTP runtime missing" in error for error in errors))
        self.assertTrue(any("PostgreSQL runtime missing" in error for error in errors))

    def test_revoked_state_is_required(self):
        broken = copy.deepcopy(self.schema)
        broken["$defs"]["state"]["enum"].remove("REVOKED")
        errors = validate_mobile_installation_contract(self.go, self.migration, broken)
        self.assertTrue(any("installation states differ" in error for error in errors))

    def test_push_generation_must_be_positive(self):
        broken = copy.deepcopy(self.schema)
        broken["properties"]["push_generation"]["minimum"] = 0
        errors = validate_mobile_installation_contract(self.go, self.migration, broken)
        self.assertTrue(any("push_generation" in error for error in errors))

    def test_active_push_endpoint_uniqueness_is_required(self):
        broken_sql = self.migration.replace(
            "CREATE UNIQUE INDEX client_installations_active_push_endpoint_idx",
            "CREATE INDEX client_installations_active_push_endpoint_idx",
        )
        errors = validate_mobile_installation_contract(self.go, broken_sql, self.schema)
        self.assertTrue(any("SQL invariant missing" in error for error in errors))


if __name__ == "__main__":
    unittest.main()
