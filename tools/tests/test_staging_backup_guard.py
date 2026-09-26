import unittest
from tools.staging_backup_guard import validate

class StagingBackupGuardTest(unittest.TestCase):
    def test_staging_backup_restore_path_is_safe(self):
        self.assertEqual(validate(), [])

if __name__ == "__main__":
    unittest.main()
