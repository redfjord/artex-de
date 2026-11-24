"""Regression tests for the narrowed API-recon dependency preflight."""
import os
import json
from pathlib import Path
import shutil
import subprocess
import tempfile
import unittest

ROOT = Path(os.environ.get('ARTEX_TEST_SOURCE', str(Path(__file__).resolve().parents[1])))
NODE = shutil.which('node')
HELPER = ROOT / 'skills/api-recon/scripts/ensure-runtime.sh'


@unittest.skipUnless(NODE, 'Node is required for runtime resolution tests')
class RuntimePreflightTests(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory(prefix='artex-runtime-preflight-')
        self.root = Path(self.tmp.name)
        self.trusted = self.root / 'trusted/puppeteer-core'
        self.trusted.mkdir(parents=True)
        (self.trusted / 'index.js').write_text('module.exports={trusted:true}')
        shutil.copy2(HELPER, self.root / 'ensure-runtime.sh')
        (self.root / 'bin').mkdir()
        (self.root / 'bin/node').symlink_to(NODE)
        npm = self.root / 'bin/npm'
        npm.write_text('''#!/bin/sh
printf '%s\\n' "$*" >> npm-called
if [ "${FAIL_INSTALL:-0}" = 1 ]; then exit 42; fi
mkdir -p node_modules/puppeteer-core
printf '%s\\n' 'module.exports={fixture:true}' > node_modules/puppeteer-core/index.js
''')
        npm.chmod(0o700)
        self.env = {'PATH': str(self.root / 'bin') + ':/usr/bin:/bin', 'HOME': str(self.root)}

    def tearDown(self):
        self.tmp.cleanup()

    def run_helper(self, module=None, install_fail=False):
        env = self.env.copy()
        if module is not None:
            env['ARTEX_PUPPETEER_PATH'] = str(module)
        if install_fail:
            env['FAIL_INSTALL'] = '1'
        return subprocess.run(['/bin/sh', 'ensure-runtime.sh'], cwd=self.root, env=env, capture_output=True, text=True)

    def test_trusted_preinstall_never_runs_npm(self):
        result = self.run_helper(self.trusted)
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertFalse((self.root / 'npm-called').exists())

    def test_missing_preinstall_fails_without_network_fallback(self):
        self.assertNotEqual(self.run_helper(self.root / 'missing').returncode, 0)
        self.assertFalse((self.root / 'npm-called').exists())

    def test_relative_preinstall_path_is_rejected(self):
        self.assertNotEqual(self.run_helper('puppeteer-core').returncode, 0)
        self.assertFalse((self.root / 'npm-called').exists())

    def test_workspace_module_cannot_shadow_preinstall(self):
        shadow = self.root / 'node_modules/puppeteer-core'
        shadow.mkdir(parents=True)
        (shadow / 'index.js').write_text('throw Error("workspace shadow loaded")')
        result = self.run_helper(self.trusted)
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertFalse((self.root / 'npm-called').exists())

    def test_explicit_local_install_uses_locked_script_free_command(self):
        result = self.run_helper()
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual((self.root / 'npm-called').read_text().strip(), 'ci --ignore-scripts --registry=https://registry.npmjs.org/')

    def test_failed_local_install_stops(self):
        self.assertEqual(self.run_helper(install_fail=True).returncode, 42)

    def test_lockfile_uses_official_integrity_checked_script_free_packages(self):
        manifest = json.loads((HELPER.parent / 'package-lock.json').read_text())
        dependencies = [v for k, v in manifest['packages'].items() if k]
        self.assertTrue(dependencies)
        for package in dependencies:
            self.assertTrue(package.get('resolved', '').startswith('https://registry.npmjs.org/'))
            self.assertTrue(package.get('integrity', '').startswith('sha512-'))
            self.assertFalse(package.get('hasInstallScript', False))


if __name__ == '__main__':
    unittest.main()
