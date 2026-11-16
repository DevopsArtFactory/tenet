import importlib.util
from pathlib import Path
import unittest

spec = importlib.util.spec_from_file_location("readiness", Path(__file__).resolve().parents[1] / "scripts/check-public-readiness.py")
readiness = importlib.util.module_from_spec(spec)
spec.loader.exec_module(readiness)


class PublicReadinessTests(unittest.TestCase):
    def setUp(self):
        self.repo = {"visibility": "public", "description": "Tenet", "homepage": "https://example.test"}
        self.reporting = {"enabled": True}
        self.protection = {
            "required_pull_request_reviews": {},
            "required_status_checks": {"strict": True, "checks": [{"context": name} for name in readiness.REQUIRED_CHECKS]},
            "enforce_admins": {"enabled": True},
            "allow_force_pushes": {"enabled": False},
            "allow_deletions": {"enabled": False},
        }

    def test_complete_configuration(self):
        self.assertTrue(all(readiness.evaluate(self.repo, self.reporting, self.protection).values()))

    def test_unavailable_settings_fail_closed(self):
        checks = readiness.evaluate(self.repo, None, None)
        self.assertFalse(checks["private vulnerability reporting"])
        self.assertFalse(checks["required pull requests"])
        self.assertFalse(checks["required CI checks and up-to-date branch"])

    def test_missing_check_or_admin_bypass_is_not_ready(self):
        self.protection["required_status_checks"]["checks"].pop()
        self.protection["enforce_admins"]["enabled"] = False
        checks = readiness.evaluate(self.repo, self.reporting, self.protection)
        self.assertFalse(checks["required CI checks and up-to-date branch"])
        self.assertFalse(checks["administrator enforcement"])

    def test_private_repository_is_not_public_ready(self):
        self.repo["visibility"] = "private"
        self.assertFalse(readiness.evaluate(self.repo, self.reporting, self.protection)["public visibility"])


if __name__ == "__main__":
    unittest.main()
