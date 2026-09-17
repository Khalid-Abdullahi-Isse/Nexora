#!/usr/bin/env python3
"""Manage only this checkout's Kong forward, verifying PID identity before stopping."""
import argparse
import json
import os
from pathlib import Path
import signal
import socket
import subprocess
import time

ROOT = Path(__file__).resolve().parents[1]
STATE = ROOT / '.secrets/dev'
STATE.mkdir(parents=True, exist_ok=True, mode=0o700)
RECORD = STATE / 'forward.json'
LOG = STATE / 'forward.log'
COMMAND = ['kubectl', '--context', 'kind-social-media', '-n', 'social-media',
           'port-forward', '--address=127.0.0.1,::1', 'svc/kong-gateway', '8000:8000']

def identity(pid):
    try:
        p = Path('/proc') / str(pid)
        # starttime is field 22; comm can contain spaces/parentheses.
        fields = (p / 'stat').read_text().rsplit(')', 1)[1].split()
        if fields[0] == 'Z':
            return None
        return {'pid': pid, 'start': fields[19],
                'command': (p / 'cmdline').read_bytes().rstrip(b'\0').decode().split('\0')}
    except (FileNotFoundError, ProcessLookupError):
        return None

def owned():
    if not RECORD.exists():
        return None
    record = json.loads(RECORD.read_text())
    if record.get('command') == COMMAND and identity(record['pid']) == record:
        return record
    RECORD.unlink()
    return None

def busy():
    for family, address in [(socket.AF_INET, '127.0.0.1'), (socket.AF_INET6, '::1')]:
        with socket.socket(family) as s:
            if family == socket.AF_INET6:
                s.setsockopt(socket.IPPROTO_IPV6, socket.IPV6_V6ONLY, 1)
            try:
                s.bind((address, 8000))
            except OSError:
                return True
    return False

def check():
    record = owned()
    if busy() and not record:
        raise SystemExit('Port 8000 is already in use by another process. Inspect: ss -ltnp "( sport = :8000 )". Nothing was killed. Stop that process yourself or use its existing forward.')
    return record

p = argparse.ArgumentParser(description=__doc__)
p.add_argument('action', choices=['check', 'start', 'stop'])
a = p.parse_args()
if a.action == 'stop':
    record = owned()
    if record:
        # pidfd prevents accidentally signalling a reused PID.
        try:
            fd = os.pidfd_open(record['pid'])
        except ProcessLookupError:
            fd = None
        if fd is not None:
            try:
                if identity(record['pid']) == record:
                    signal.pidfd_send_signal(fd, signal.SIGTERM)
            finally:
                os.close(fd)
        for _ in range(50):
            if identity(record['pid']) != record:
                break
            time.sleep(.1)
        else:
            raise SystemExit('Forward has not stopped; state retained. No other process was signalled.')
        RECORD.unlink(missing_ok=True)
        print('Stopped the development Kong forward. Cluster and data remain running.')
    else:
        print('No owned Kong forward is running. Other processes were left untouched.')
else:
    record = check()
    if a.action == 'start':
        if record:
            print('Reusing the existing development Kong forward: http://localhost:8000')
        else:
            with LOG.open('w') as log:
                process = subprocess.Popen(COMMAND, stdin=subprocess.DEVNULL, stdout=log,
                                           stderr=subprocess.STDOUT, start_new_session=True)
            for _ in range(100):
                if process.poll() is not None:
                    raise SystemExit('Kong forwarding failed: ' + LOG.read_text())
                record = identity(process.pid)
                if record and record['command'] == COMMAND:
                    RECORD.write_text(json.dumps(record))
                if RECORD.exists() and 'Forwarding from' in LOG.read_text() and busy():
                    print('Kong is available at http://localhost:8000; stop with ./scripts/stop-dev.sh')
                    break
                time.sleep(.1)
            else:
                process.terminate()
                process.wait(timeout=5)
                RECORD.unlink(missing_ok=True)
                raise SystemExit('Timed out starting Kong forward; see ' + str(LOG))
