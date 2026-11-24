"""Run every Go test with a fresh PostgreSQL database and process.

Several packages persist task/provider fixtures and start background managers.
Separate processes and databases prevent those fixtures from affecting another
test. Test discovery is automatic: no failing test is filtered out.
"""
import argparse
import json
import os
from pathlib import Path
import re
import subprocess
import tempfile
import urllib.parse
import uuid

parser = argparse.ArgumentParser()
parser.add_argument('--output', default='artifacts/go-tests')
parser.add_argument('--package', action='append', dest='packages')
args = parser.parse_args()
admin = os.environ['ARTEX_TEST_ADMIN_DSN']
output = Path(args.output).resolve()
output.mkdir(parents=True, exist_ok=True)
root = Path(__file__).resolve().parents[1]


def database_sql(statement):
    subprocess.run(['psql', '--dbname', admin, '-X', '-v', 'ON_ERROR_STOP=1', '-c', statement],
                   check=True, capture_output=True, text=True)


def test_dsn(name):
    return urllib.parse.urlunsplit(urllib.parse.urlsplit(admin)._replace(path='/' + name))


packages = subprocess.check_output([
    'go', 'list', '-f', '{{.ImportPath}}|{{.Dir}}|{{len .TestGoFiles}}|{{len .XTestGoFiles}}',
    *(args.packages or ['./...'])], cwd=root, text=True).splitlines()
summary = {'passed': [], 'failed': [], 'skipped_subtests': []}
with tempfile.TemporaryDirectory(prefix='artex-test-binaries-') as temporary:
    for index, package in enumerate(packages):
        import_path, directory, internal, external = package.split('|')
        if int(internal) + int(external) == 0:
            continue
        binary = Path(temporary) / f'package-{index}.test'
        subprocess.run(['go', 'test', '-c', '-o', str(binary), import_path], cwd=root, check=True)
        discovery_env = dict(os.environ)
        discovery_env.pop('ARTEX_PG_DSN', None)
        listing = subprocess.check_output([str(binary), '-test.list', '^Test'], cwd=directory,
                                          env=discovery_env, text=True)
        tests = [name for name in listing.splitlines() if re.fullmatch(r'Test\w+', name)]
        for name in tests:
            database = 'artex_test_' + uuid.uuid4().hex
            database_sql(f'CREATE DATABASE "{database}"')
            env = dict(os.environ, ARTEX_PG_DSN=test_dsn(database), NORMA_RIPGREP_NO_INSTALL='1')
            key = import_path.rsplit('/', 1)[-1] + '-' + name
            label = import_path + '/' + name
            try:
                result = subprocess.run([str(binary), '-test.v', '-test.timeout=90s',
                                         '-test.run', '^' + re.escape(name) + '$'],
                                        cwd=directory, env=env, capture_output=True, text=True,
                                        timeout=100)
                log = result.stdout + result.stderr
                (output / (key + '.log')).write_text(log)
                db_skipped = any(reason in log.lower() for reason in (
                    'postgres unavailable', 'no database config', 'requires isolated artex_pg_dsn'))
                if result.returncode or db_skipped:
                    summary['failed'].append(label)
                    print('FAIL', label, flush=True)
                    print(log[-12000:], flush=True)
                else:
                    summary['passed'].append(label)
                    summary['skipped_subtests'].extend(line for line in log.splitlines() if '--- SKIP:' in line)
                    print('PASS', label, flush=True)
            except subprocess.TimeoutExpired:
                summary['failed'].append(label)
                print('FAIL (timeout)', label, flush=True)
            finally:
                database_sql(f'DROP DATABASE "{database}" WITH (FORCE)')
(output / 'summary.json').write_text(json.dumps(summary, indent=2) + '\n')
print(f'{len(summary["passed"])} passed; {len(summary["failed"])} failed; '
      f'{len(summary["skipped_subtests"])} reported skips', flush=True)
raise SystemExit(1 if summary['failed'] else 0)
