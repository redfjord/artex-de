"""End-to-end check of the embedded UI served by a real ARTEX binary.

Boots the binary against a throwaway data directory and the PostgreSQL database
in ARTEX_PG_DSN, then exercises the paths that a remote operator actually hits:
the static export, compression, ETag revalidation, authentication, task
creation, and graceful shutdown.

The compression/caching assertions are the regression guard for the direct
:8787 deployment bug: an embed.FS served through http.ServeFileFS ships the
multi-megabyte uncompressed bundle with no validator, so every navigation
re-downloads it and the UI appears to time out on a slow link.

Usage: python3 ci/e2e_binary.py /path/to/artex
Requires: ARTEX_PG_DSN pointing at an empty database.
"""
import gzip
import http.client
import json
import os
import re
import signal
import socket
import subprocess
import sys
import tempfile
import time
import urllib.error
import urllib.parse
import urllib.request


def free_port():
    with socket.socket() as sock:
        sock.bind(('127.0.0.1', 0))
        return sock.getsockname()[1]


def request(base, path, method='GET', payload=None, token=None, headers=None):
    url = base + path
    body = None if payload is None else json.dumps(payload).encode()
    req = urllib.request.Request(url, data=body, method=method)
    if body is not None:
        req.add_header('Content-Type', 'application/json')
    if token:
        req.add_header('Authorization', 'Bearer ' + token)
    for key, value in (headers or {}).items():
        req.add_header(key, value)
    try:
        with urllib.request.urlopen(req, timeout=15) as response:
            return response.status, dict(response.headers), response.read()
    except urllib.error.HTTPError as error:
        return error.code, dict(error.headers), error.read()


def header(headers, name):
    # Go canonicalizes header names (ETag -> Etag); compare case-insensitively.
    for key, value in headers.items():
        if key.lower() == name.lower():
            return value
    return None


def api(base, path, method='GET', payload=None, token=None):
    status, _, raw = request(base, '/api' + path, method, payload, token)
    assert status in (200, 201, 202), f'{method} {path}: expected 2xx, got {status}: {raw[:200]!r}'
    return json.loads(raw or b'{}')


def wait_healthy(base, process):
    deadline = time.time() + 60
    while time.time() < deadline:
        if process.poll() is not None:
            raise RuntimeError(f'ARTEX exited during startup ({process.returncode})')
        try:
            status, _, _ = request(base, '/api/health')
            if status == 200:
                return
        except (urllib.error.URLError, ConnectionError, TimeoutError):
            pass
        time.sleep(0.5)
    raise RuntimeError('ARTEX did not become healthy')


def main():
    if len(sys.argv) != 2:
        raise SystemExit('usage: e2e_binary.py /path/to/artex')
    binary = os.path.abspath(sys.argv[1])
    dsn = os.environ.get('ARTEX_PG_DSN')
    if not dsn:
        raise SystemExit('ARTEX_PG_DSN is required')
    port = free_port()
    base = f'http://127.0.0.1:{port}'
    with tempfile.TemporaryDirectory(prefix='artex-e2e-') as data:
        env = dict(os.environ, ARTEX_PG_DSN=dsn, NORMA_RIPGREP_NO_INSTALL='1')
        process = subprocess.Popen(
            [binary, '-addr', f'127.0.0.1:{port}', '-proxy', '', '-data', data],
            env=env, stdout=subprocess.PIPE, stderr=subprocess.STDOUT, text=True,
        )
        try:
            wait_healthy(base, process)

            # The static export and its SPA fallback must be served.
            status, headers, html = request(base, '/function/tasks/')
            assert status == 200, f'UI route: {status}'
            assert 'text/html' in header(headers, 'Content-Type') or '', headers
            assert b'<html' in html.lower(), 'UI route did not return HTML'

            status, headers, _ = request(base, '/')
            assert status == 200 and 'text/html' in header(headers, 'Content-Type') or ''

            # A missing static asset must not silently turn into index.html.
            status, _, _ = request(base, '/_next/static/chunks/does-not-exist.js')
            assert status == 404, f'missing chunk: expected 404, got {status}'

            text = html.decode('utf-8', 'replace')
            assets = sorted(set(re.findall(r'/_next/static/[^"\' ]+?\.js', text)))
            assert assets, 'could not find a JavaScript chunk in the exported HTML'

            raw_asset = assets[0]
            _, raw_headers, raw_body = request(base, raw_asset)
            assert header(raw_headers, 'Cache-Control') == 'public, max-age=31536000, immutable', raw_headers
            etag = header(raw_headers, 'ETag')
            assert etag, 'hashed asset has no ETag'

            # Conditional revalidation: the browser keeps the cached bundle.
            status, _, body = request(base, raw_asset, headers={'If-None-Match': etag})
            assert status == 304 and not body, f'ETag revalidation returned {status}'

            # Brotli (preferred) and gzip both shrink the text bundle.
            for encoding in ('br', 'gzip'):
                _, headers, body = request(base, raw_asset, headers={'Accept-Encoding': encoding})
                assert header(headers, 'Content-Encoding') == encoding, (encoding, headers)
                assert len(body) < len(raw_body), f'{encoding} did not reduce the asset'
                if encoding == 'gzip':
                    assert len(gzip.decompress(body)) == len(raw_body), 'gzip body mismatch'

            # HTML must revalidate rather than be cached forever.
            _, html_headers, _ = request(base, '/function/tasks/')
            assert header(html_headers, 'Cache-Control') == 'no-cache', html_headers

            # Authentication, task creation and listing over the same server.
            status, _, _ = request(base, '/api/tasks')
            assert status == 401, f'anonymous /api/tasks: expected 401, got {status}'
            token = api(base, '/auth/init', 'POST', {'password': 'ARTEX-e2e-fixture-2026!'})['token']
            login = api(base, '/auth/login', 'POST', {'username': 'ARTEX', 'password': 'ARTEX-e2e-fixture-2026!'})
            assert login.get('token'), 'login returned no token'
            task = api(base, '/tasks', 'POST', {
                'name': 'e2e task',
                'description': 'local fixture',
                'goal': 'verify the API',
            }, token=token)
            listed = api(base, '/tasks', token=token)
            items = listed.get('tasks', listed) if isinstance(listed, dict) else listed
            assert any(str(item.get('id')) == str(task['id']) for item in items), 'created task not listed'

            # The updater stays disabled regardless of version.
            update = api(base, '/update/check?force=1', token=token)
            assert update.get('disabled') is True and update.get('has_update') is False, update

            # Graceful shutdown: SIGTERM must end the process, not hang.
            process.send_signal(signal.SIGTERM)
            process.wait(timeout=30)
            assert process.returncode == 0, f'shutdown exit code {process.returncode}'
        finally:
            if process.poll() is None:
                process.kill()
                process.wait(timeout=10)
            output = process.stdout.read() if process.stdout else ''
    print(f'e2e ok: {binary} on port {port}')
    return output


if __name__ == '__main__':
    main()