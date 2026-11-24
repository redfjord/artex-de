"""Regression tests for the entrypoint using harmless local child processes."""

import json
import os
from pathlib import Path
import shutil
import signal
import subprocess
import sys
import tempfile
import time
import unittest


@unittest.skipUnless(os.name == "posix", "start.sh requires a POSIX shell")
class StartTests(unittest.TestCase):
    def setUp(self):
        self.directory = tempfile.TemporaryDirectory(prefix="artex-start-test-")
        self.root = Path(self.directory.name)
        shutil.copyfile(Path(__file__).resolve().parents[1] / "start.sh", self.root / "start.sh")
        self.write_executable("artex", """
import json, os, signal, sys, time
from pathlib import Path
attempts = Path('attempts')
with attempts.open('a') as log:
    log.write('attempt\\n')
Path('arguments').write_text(json.dumps(sys.argv[1:]))
mode = os.environ['TEST_MODE']
if mode in ('crash', 'restart'):
    if len(attempts.read_text().splitlines()) == 1:
        sys.exit(1 if mode == 'crash' else 75)
    sys.exit(0)
if mode == 'crash-many':
    sys.exit(1 if len(attempts.read_text().splitlines()) <= 8 else 0)
if mode == 'restart-after-crashes':
    codes = [1, 1, 75, 1, 0]
    sys.exit(codes[len(attempts.read_text().splitlines()) - 1])
if mode == 'active':
    def stop(signum, frame):
        Path('child-stopped').write_text('stopped')
        sys.exit(0)
    signal.signal(signal.SIGTERM, stop)
    Path('child-ready').touch()
    while True:
        time.sleep(60)
""")
        (self.root / "bin").mkdir()
        self.write_executable("bin/sleep", """
import os, sys, time
from pathlib import Path
with Path('delays').open('a') as log:
    log.write(sys.argv[1] + '\\n')
Path('backoff-ready').touch()
# Hold the backoff long enough that the stop tests don't depend on sub-second timing.
if os.environ.get('HOLD_BACKOFF') == '1':
    time.sleep(30)
""")
        self.process = None
        self.output = (self.root / "output").open("w")

    def write_executable(self, name, code):
        target = self.root / name
        target.write_text(f"#!{sys.executable}\n" + code)
        target.chmod(0o700)

    def tearDown(self):
        if self.process is not None:
            try:
                os.killpg(self.process.pid, signal.SIGKILL)
            except ProcessLookupError:
                pass
            self.process.wait(timeout=5)
        self.output.close()
        self.directory.cleanup()

    def start(self, mode="normal", hold_backoff=False, args=()):
        env = dict(os.environ, TEST_MODE=mode, HOLD_BACKOFF="1" if hold_backoff else "0")
        env["PATH"] = str(self.root / "bin") + os.pathsep + env.get("PATH", "")
        self.process = subprocess.Popen(
            ["/bin/sh", str(self.root / "start.sh"), *args],
            cwd=self.root, env=env, stdout=self.output, stderr=self.output,
            start_new_session=True,
        )

    def wait_for(self, marker):
        deadline = time.monotonic() + 5
        while time.monotonic() < deadline:
            if (self.root / marker).exists():
                return
            if self.process.poll() is not None:
                break
            time.sleep(0.01)
        self.fail(f"Did not reach {marker}: {(self.root / 'output').read_text()}")

    def assert_stopped(self, attempts):
        self.assertEqual(self.process.wait(timeout=3), 0)
        self.assertEqual(len((self.root / "attempts").read_text().splitlines()), attempts)

    def test_normal_exit_and_arguments(self):
        self.start(args=("-addr", "127.0.0.1:9000"))
        self.assert_stopped(1)
        self.assertEqual(json.loads((self.root / "arguments").read_text()), ["-addr", "127.0.0.1:9000"])
        self.assertFalse((self.root / "delays").exists())

    def test_requested_restart_has_no_backoff(self):
        self.start(mode="restart")
        self.assert_stopped(2)
        self.assertFalse((self.root / "delays").exists())

    def test_crash_still_restarts_after_backoff(self):
        self.start(mode="crash")
        self.assert_stopped(2)
        self.assertEqual((self.root / "delays").read_text(), "1\n")

    def test_backoff_doubles_and_stops_growing_at_sixty(self):
        self.start(mode="crash-many")
        self.assert_stopped(9)
        self.assertEqual((self.root / "delays").read_text().splitlines(),
                         ["1", "2", "4", "8", "16", "32", "60", "60"])

    def test_requested_restart_resets_backoff(self):
        self.start(mode="restart-after-crashes")
        self.assert_stopped(5)
        self.assertEqual((self.root / "delays").read_text().splitlines(), ["1", "2", "1"])

    def stop_during_backoff(self, signum):
        self.start(mode="crash", hold_backoff=True)
        self.wait_for("backoff-ready")
        self.process.send_signal(signum)
        self.assert_stopped(1)

    def test_term_during_backoff_stops_without_restarting(self):
        self.stop_during_backoff(signal.SIGTERM)

    def test_int_during_backoff_stops_without_restarting(self):
        self.stop_during_backoff(signal.SIGINT)

    def stop_active_child(self, signum):
        self.start(mode="active")
        self.wait_for("child-ready")
        self.process.send_signal(signum)
        self.assert_stopped(1)
        self.assertTrue((self.root / "child-stopped").exists())

    def test_term_still_stops_active_child(self):
        self.stop_active_child(signal.SIGTERM)

    def test_int_still_stops_active_child(self):
        self.stop_active_child(signal.SIGINT)


if __name__ == "__main__":
    unittest.main()
