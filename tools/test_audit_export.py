import unittest
from pathlib import Path
import subprocess
import tempfile
from audit_export import audit, problems

class AuditTests(unittest.TestCase):
    def test_documentation_address_allowed(self):
        self.assertEqual(problems('example.txt', b'192.0.2.10 127.0.0.1'), [])

    def test_private_address_rejected(self):
        text = ('192.' + '168.' + '42.7').encode()
        self.assertIn('non-documentation-private-IP', problems('code.c', text))

    def test_archive_and_sensitive_filename_rejected(self):
        self.assertIn('binary-or-archive', problems('payload', b'\x7fELF'))
        self.assertIn('forbidden-path', problems('runtime.key', b'not-a-key'))

    def test_exact_private_value_never_reported(self):
        value = b'synthetic-external-value'
        result = problems('code.go', b'prefix ' + value, [value])
        self.assertEqual(result, ['known-private-value'])
        self.assertNotIn(value.decode(), str(result))

    def test_removed_material_is_still_caught_in_git_history(self):
        with tempfile.TemporaryDirectory(prefix='export-audit-test-') as tmp:
            root = Path(tmp)
            def git(*args):
                return subprocess.run(['git', '-c', 'user.name=Audit fixture',
                    '-c', 'user.email=audit@example.invalid', '-c', 'commit.gpgsign=false',
                    '-c', 'core.hooksPath=/dev/null', *args], cwd=root,
                    check=True, capture_output=True)
            git('init', '--initial-branch=main')
            fixture = root / 'synthetic.txt'
            fixture.write_text('192.' + '168.' + '42.7')
            git('add', '--force', 'synthetic.txt')
            git('commit', '-m', 'Synthetic negative fixture')
            fixture.unlink()
            git('add', '-u')
            git('commit', '-m', 'Remove fixture from worktree')
            _, blobs, failures = audit(root)
            self.assertGreater(blobs, 0)
            self.assertTrue(any(name.startswith('history:') and
                'non-documentation-private-IP' in labels for name, labels in failures))

if __name__ == '__main__':
    unittest.main()
