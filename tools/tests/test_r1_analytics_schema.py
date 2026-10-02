from __future__ import annotations

import json
import unittest
from pathlib import Path

from jsonschema import Draft202012Validator, FormatChecker
from jsonschema.exceptions import ValidationError

ROOT = Path(__file__).resolve().parents[2]
SCHEMA_PATH = ROOT / "contracts/jsonschema/r1-mobile-cross-surface-v1.schema.json"


class R1AnalyticsSchemaTest(unittest.TestCase):
    @classmethod
    def setUpClass(cls) -> None:
        schema = json.loads(SCHEMA_PATH.read_text(encoding="utf-8"))
        analytics = schema["$defs"]["CanonicalAnalyticsEvent"]
        cls.validator = Draft202012Validator(analytics, format_checker=FormatChecker())

    def base_event(self) -> dict[str, object]:
        return {
            "event_name": "workspace_switched",
            "event_version": "1",
            "occurred_at": "2026-10-02T10:00:00Z",
            "journey_id": "journey-schema-proof",
            "identity_ref": "identity-schema-proof",
            "business_properties": {
                "workspace_id": "workspace-schema-proof",
                "workspace_kind": "SPECIALIST",
            },
        }

    def assert_valid(self, event: dict[str, object]) -> None:
        self.validator.validate(event)

    def assert_invalid(self, event: dict[str, object]) -> None:
        with self.assertRaises(ValidationError):
            self.validator.validate(event)

    def test_same_business_event_is_valid_for_each_surface_namespace(self) -> None:
        for surface in ("WEB", "IOS", "ANDROID"):
            with self.subTest(surface=surface):
                event = self.base_event()
                event["platform_extensions"] = {
                    surface: {"app_version": "1.0.0", "network_reachable": True}
                }
                self.assert_valid(event)

    def test_multiple_surface_namespaces_are_rejected(self) -> None:
        event = self.base_event()
        event["platform_extensions"] = {
            "WEB": {"renderer": "NEXTJS"},
            "IOS": {"app_version": "1.0.0"},
        }
        self.assert_invalid(event)

    def test_raw_consultation_media_keys_are_rejected_from_business_payload(self) -> None:
        for forbidden in ("raw_transcript", "transcript", "raw_audio", "raw_video"):
            with self.subTest(forbidden=forbidden):
                event = self.base_event()
                business = dict(event["business_properties"])
                business[forbidden] = "sensitive-content"
                event["business_properties"] = business
                self.assert_invalid(event)

    def test_required_business_semantics_cannot_be_silently_dropped(self) -> None:
        event = self.base_event()
        del event["journey_id"]
        self.assert_invalid(event)

    def test_unknown_event_and_unknown_top_level_field_are_rejected(self) -> None:
        unknown_event = self.base_event()
        unknown_event["event_name"] = "ios_workspace_switched"
        self.assert_invalid(unknown_event)

        unknown_field = self.base_event()
        unknown_field["platform"] = "IOS"
        self.assert_invalid(unknown_field)


if __name__ == "__main__":
    unittest.main()
