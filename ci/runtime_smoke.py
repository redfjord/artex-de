"""Boot the built image and PostgreSQL on an internal network; no target scans."""
import json
from pathlib import Path
import subprocess
import sys
import time
import uuid

IMAGE = sys.argv[1] if len(sys.argv) > 1 else 'failsafe-security/artex:ci'
PG_IMAGE = 'postgres:16-alpine@sha256:0cefaabd882e42bcb2a2f3dc56d3ce4770560ba62fd7f197f1a2e008fd5731ab'
name = 'artex-smoke-' + uuid.uuid4().hex[:10]
pg, app, data, keys = (name + suffix for suffix in ('-pg', '-app', '-data', '-keys'))


def docker(*args, check=True, input=None):
    result = subprocess.run(['docker', *args], input=input, text=True, capture_output=True, timeout=180)
    if check and result.returncode:
        raise RuntimeError(f'docker {args[0]} failed ({result.returncode}):\n{result.stdout}\n{result.stderr}')
    return result


def api(path, method='GET', payload=None, token=None):
    # Values are serialized as Python literals, never interpolated into a shell.
    program = '''import json,urllib.request,urllib.error
request=urllib.request.Request('http://127.0.0.1:8787'+PATH,method=METHOD,
 data=None if PAYLOAD is None else json.dumps(PAYLOAD).encode(),
 headers={'Content-Type':'application/json',**({'Authorization':'Bearer '+TOKEN} if TOKEN else {})})
try:
 response=urllib.request.urlopen(request,timeout=10)
except urllib.error.HTTPError as error:
 response=error
raw=response.read().decode()
try: body=json.loads(raw)
except ValueError: body=raw
print(json.dumps({'status':response.status,'body':body}))
'''
    prelude = f'PATH={path!r}\nMETHOD={method!r}\nPAYLOAD={payload!r}\nTOKEN={token!r}\n'
    return json.loads(docker('exec', app, 'python3', '-c', prelude + program).stdout)


def expect(result, status):
    assert result['status'] == status, f'Expected HTTP {status}, got {result["status"]}'
    return result['body']


try:
    docker('network', 'create', '--internal', name)
    for volume in (data, keys):
        docker('volume', 'create', volume)
    docker('run', '-d', '--name', pg, '--network', name,
           '-e', 'POSTGRES_USER=artex', '-e', 'POSTGRES_PASSWORD=smoke-db-fixture',
           '-e', 'POSTGRES_DB=artex', PG_IMAGE)
    for attempt in range(60):
        if docker('exec', pg, 'pg_isready', '-U', 'artex', check=False).returncode == 0:
            break
        time.sleep(1)
    else:
        raise RuntimeError('PostgreSQL did not become ready')
    docker('run', '-d', '--name', app, '--network', name,
           '--security-opt', 'no-new-privileges:true', '--shm-size=1g',
           '-e', f'ARTEX_PG_DSN=postgres://artex:smoke-db-fixture@{pg}:5432/artex?sslmode=disable',
           '-v', data + ':/app/data', '-v', keys + ':/app/keys',
           '-v', str(Path(__file__).resolve().parent) + ':/ci:ro', IMAGE)
    for attempt in range(90):
        if docker('exec', app, 'curl', '-fsS', 'http://127.0.0.1:8787/api/health', check=False).returncode == 0:
            break
        time.sleep(1)
    else:
        raise RuntimeError('ARTEX did not become healthy')
    expect(api('/'), 200)
    expect(api('/api/tasks'), 401)
    password = 'ARTEX-smoke-fixture-2026!'
    token = expect(api('/api/auth/init', 'POST', {'password': password}), 200)['token']
    expect(api('/api/auth/login', 'POST', {'username': 'ARTEX', 'password': 'wrong', 'captcha': 'smoke-canary'}), 401)
    expect(api('/api/auth/login', 'POST', {'username': 'ARTEX', 'password': password}), 200)
    expect(api('/api/tasks', token=token), 200)
    update = expect(api('/api/update/check?force=1', token=token), 200)
    assert update['disabled'] is True and update['has_update'] is False
    expect(api('/api/update/apply', 'POST', {}, token), 403)
    expect(api('/api/update/rollback', 'POST', {}, token), 403)
    print(docker('exec', app, 'node', '/ci/api-recon-smoke.js').stdout.strip())
    print(docker('exec', app, 'node', '/ci/browser-mcp-smoke.cjs').stdout.strip())
    raw_scan = docker('exec', app, 'nmap', '-sS', '-p', '8787', '127.0.0.1').stdout
    assert '8787/tcp open' in raw_scan, raw_scan
    stopped = time.monotonic()
    docker('stop', '--time', '15', app)
    state = json.loads(docker('inspect', '--format', '{{json .State}}', app).stdout)
    assert state['ExitCode'] == 0 and not state['OOMKilled'], state
    print(f'Graceful SIGTERM: exit 0 in {time.monotonic() - stopped:.2f}s')
    docker('start', app)
    for attempt in range(60):
        if docker('exec', app, 'curl', '-fsS', 'http://127.0.0.1:8787/api/health', check=False).returncode == 0:
            break
        time.sleep(1)
    else:
        raise RuntimeError('ARTEX did not become healthy after restart')
    expect(api('/api/tasks', token=token), 200)
    print('Runtime smoke: embedded UI, PostgreSQL, auth, updater, browsers, raw scan, shutdown and persisted JWT passed')
except Exception:
    for container in (app, pg):
        print(docker('logs', '--tail', '80', container, check=False).stderr, file=sys.stderr)
    raise
finally:
    for container in (app, pg):
        docker('rm', '-f', container, check=False)
    docker('network', 'rm', name, check=False)
    for volume in (data, keys):
        docker('volume', 'rm', volume, check=False)
