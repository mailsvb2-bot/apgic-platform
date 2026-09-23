import unittest

from tools.multisurface_guard import launch_surface_errors


class LaunchSurfaceEvidenceTests(unittest.TestCase):
    def test_verified_launch_requirement_requires_all_surface_evidence(self) -> None:
        requirements = [
            {
                "requirement_id": "APGIC-X-001",
                "release_profile": "R1",
                "status": "VERIFIED",
                "evidence_refs": ["surface://WEB/build-1"],
            }
        ]
        errors = launch_surface_errors(
            requirements,
            [],
            {"R0", "R1", "R2", "R3", "R4"},
            {
                "WEB": "surface://WEB/",
                "IOS": "surface://IOS/",
                "ANDROID": "surface://ANDROID/",
            },
        )
        self.assertEqual(len(errors), 1)
        self.assertIn("ANDROID", errors[0])
        self.assertIn("IOS", errors[0])

    def test_approved_compatibility_exception_can_cover_missing_surface(self) -> None:
        requirements = [
            {
                "requirement_id": "APGIC-X-002",
                "release_profile": "R2",
                "status": "RELEASED",
                "evidence_refs": [
                    "surface://WEB/build-1",
                    "surface://ANDROID/build-1",
                ],
            }
        ]
        exceptions = [
            {
                "exception_id": "compat-1",
                "requirement_id": "APGIC-X-002",
                "status": "APPROVED",
                "surfaces": ["IOS"],
                "reason": "Temporary platform incompatibility",
                "approved_by": "release-governance",
            }
        ]
        errors = launch_surface_errors(
            requirements,
            exceptions,
            {"R0", "R1", "R2", "R3", "R4"},
            {
                "WEB": "surface://WEB/",
                "IOS": "surface://IOS/",
                "ANDROID": "surface://ANDROID/",
            },
        )
        self.assertEqual(errors, [])

    def test_in_progress_requirement_does_not_claim_surface_completion(self) -> None:
        requirements = [
            {
                "requirement_id": "APGIC-X-003",
                "release_profile": "R0",
                "status": "IN_PROGRESS",
                "evidence_refs": [],
            }
        ]
        errors = launch_surface_errors(
            requirements,
            [],
            {"R0"},
            {
                "WEB": "surface://WEB/",
                "IOS": "surface://IOS/",
                "ANDROID": "surface://ANDROID/",
            },
        )
        self.assertEqual(errors, [])


if __name__ == "__main__":
    unittest.main()
