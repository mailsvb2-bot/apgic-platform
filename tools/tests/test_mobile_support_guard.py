import unittest

from tools.mobile_support_guard import validate_policy


def valid_policy() -> dict:
    return {
        "policy_version": "mobile-support-test-v1",
        "scope": "CI_ONLY",
        "production_approved": False,
        "production_evidence": False,
        "minimum_ios_major": 16,
        "minimum_android_api": 29,
        "accessibility": {
            "voice_over": True,
            "talk_back": True,
            "text_scaling": True,
            "reduced_motion": True,
            "focus_order": True,
            "touch_target_min_dp": 44,
        },
        "device_test_classes": ["PHONE_COMPACT", "PHONE_LARGE", "TABLET"],
        "targeted_physical_capability_paths": [
            "CAMERA_MICROPHONE",
            "PUSH_BACKGROUND",
            "UNIVERSAL_APP_LINKS",
            "AUDIO_ROUTE",
        ],
        "evidence_refs": [],
    }


class MobileSupportPolicyTests(unittest.TestCase):
    def test_ci_policy_is_explicit_and_device_agnostic(self) -> None:
        validate_policy(valid_policy(), "ci")

    def test_model_allowlist_is_rejected(self) -> None:
        policy = valid_policy()
        policy["device_model_allowlist"] = ["some-phone"]
        with self.assertRaises(SystemExit):
            validate_policy(policy, "ci")

    def test_missing_accessibility_requirement_fails(self) -> None:
        policy = valid_policy()
        policy["accessibility"]["talk_back"] = False
        with self.assertRaises(SystemExit):
            validate_policy(policy, "ci")

    def test_production_requires_real_surface_and_device_evidence(self) -> None:
        policy = valid_policy()
        policy["scope"] = "PRODUCTION"
        policy["production_approved"] = True
        policy["production_evidence"] = True
        with self.assertRaises(SystemExit):
            validate_policy(policy, "production")

        policy["evidence_refs"] = [
            "surface://IOS/a11y-candidate",
            "surface://ANDROID/a11y-candidate",
            "device-matrix://candidate",
        ]
        validate_policy(policy, "production")


if __name__ == "__main__":
    unittest.main()
