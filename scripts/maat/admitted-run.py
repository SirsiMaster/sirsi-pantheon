#!/usr/bin/env python3
"""Cooperative native Ma'at launch supervisor, not a security sandbox."""
import argparse
import datetime as dt
import hashlib
import json
import os
from pathlib import Path
import re
import signal
import socket
import subprocess
import sys
import time
import uuid


class Refused(RuntimeError):
    pass


def local_resource():
    name = socket.gethostname().split('.')[0].lower()
    names = {'mac': 'm5', 'm5': 'm5', 'm1': 'm1', 'm5-studio': 'studio'}
    if name not in names:
        raise Refused('Host has no enrolled local resource mapping')
    return names[name]


def valid(row, resource, agent, work):
    if not isinstance(row, dict):
        return False
    if (row.get('status'), row.get('resource'), row.get('holder'), row.get('work'), row.get('share')) != ('active', resource, agent, work, 'full'):
        return False
    if not isinstance(row.get('id'), str) or not row['id'].strip():
        return False
    try:
        stamp = dt.datetime.fromisoformat(row['heartbeat_at'].replace('Z', '+00:00'))
        ttl = row['lease_ttl_sec']
        age = (dt.datetime.now(dt.timezone.utc) - stamp).total_seconds()
        return type(ttl) is int and 0 < ttl and 0 <= age < ttl
    except (KeyError, TypeError, ValueError):
        return False


class Native:
    def __init__(self):
        self.cli = str(Path.home() / '.local/bin/sirsi')
        self.executor_thread = None
        self.executor_identity = None

    def register_executor(self, agent):
        if not re.fullmatch(r'[A-Za-z0-9_.-]{1,128}', agent or ''):
            raise Refused('A concrete fabric agent is required')
        identity = 'thr-'+uuid.uuid4().hex
        row = self.call('thread', 'register', '--agent', agent, '--surface', 'worker', '--thread', identity,
                        '--anchor-pid', str(os.getpid()), '--repo', os.getcwd(),
                        '--json', timeout=30)
        if (not isinstance(row, dict) or row.get('agent_id') != agent or
                row.get('pid') != os.getpid() or row.get('status') != 'active' or
                row.get('thread_id') != identity):
            observed = {key: row.get(key) for key in ('agent_id','pid','status','thread_id','skipped')} if isinstance(row,dict) else type(row).__name__
            raise Refused('Executor registration did not bind this actual process: '+str(observed))
        self.executor_thread = row['thread_id']
        self.executor_identity = row
        return self.executor_thread

    def close_executor(self):
        if self.executor_thread:
            self.call('thread', 'close', '--thread', self.executor_thread, '--json', timeout=30, json_result=False)

    def call(self, *args, timeout=60, json_result=True):
        try:
            response = subprocess.run([self.cli, *args], check=True, capture_output=True,
                                      text=True, timeout=timeout)
        except subprocess.CalledProcessError as error:
            raise Refused('Control call '+str(args[:2])+' failed: '+(error.stderr or '')[-1600:]) from error
        return (json.loads(response.stdout) if response.stdout.strip() else None) if json_result else response.stdout

    def registered(self, agent, thread):
        if not agent or not thread:
            raise Refused('Set your actual registered SIRSI_AGENT_ID and SIRSI_THREAD_ID')
        if self.executor_identity is not None:
            # A destination job already has its own canonical handshake.
            # Recheck the retained runtime before refreshing its heartbeat.
            self.verify_runtime(self.executor_identity, agent, thread)
        # The installed CLI already returns the exact canonical row from an
        # own-thread heartbeat, without a fleet-wide OS reaper pass. Closed or
        # suspended threads cannot be revived. Verify the returned owner and
        # actual kernel ancestry/start below; a heartbeat alone authorizes none.
        observed = self.call('thread', 'heartbeat', '--thread', thread, '--json', timeout=30)
        if not isinstance(observed, dict):
            raise Refused('Registration observation is malformed')
        rows = [observed]
        matches = [entry.get('thread', entry) for entry in rows if isinstance(entry, dict)]
        matches = [row for row in matches if isinstance(row, dict) and
                   row.get('thread_id') == thread and row.get('agent_id') == agent and
                   row.get('status') in ('active', 'idle')]
        if len(matches) != 1:
            raise Refused('Actual active lane/thread registration required; do not impersonate another session')
        self.verify_runtime(matches[0], agent, thread)

    def verify_runtime(self, row, agent, thread):
        if (row.get('thread_id') != thread or row.get('agent_id') != agent or
                row.get('status') not in ('active','idle')):
            raise Refused('Runtime row does not belong to this active agent/thread')
        anchor = row.get('pid')
        if type(anchor) is not int or anchor <= 1:
            raise Refused('Registration lacks a concrete process anchor')
        cursor = os.getpid()
        ancestors = set()
        while cursor > 1 and cursor not in ancestors and len(ancestors) < 64:
            ancestors.add(cursor)
            cursor = int(subprocess.check_output(['/bin/ps', '-p', str(cursor), '-o', 'ppid='], text=True, timeout=2).strip())
        start = subprocess.check_output(['/bin/ps', '-p', str(anchor), '-o', 'lstart='], text=True, timeout=2).strip()
        if anchor not in ancestors or start != row.get('start_time'):
            raise Refused('Registered anchor is not this launcher ancestry or its start identity changed')

    def lookup(self, resource):
        return self.call('maat', 'who-is-on', resource, '--json')

    def reserve(self, resource, agent, work):
        return self.call('maat', 'reserve', resource, '--holder', agent, '--work', work,
                         '--regime', 'loaded', '--lease-ttl', '120', '--exempt-pid',
                         str(os.getpid()), '--repro', str(Path(__file__).resolve()), '--json')

    def heartbeat(self, identity):
        self.call('maat', 'heartbeat', identity, '--json')
        self.heartbeat_executor()

    def heartbeat_executor(self):
        if self.executor_thread:
            self.call('thread', 'heartbeat', '--thread', self.executor_thread, '--json', timeout=30, json_result=False)

    def release(self, identity):
        self.call('maat', 'release', identity, '--json')


def stop(child):
    try:
        os.killpg(child.pid, signal.SIGTERM)
    except ProcessLookupError:
        pass
    try:
        child.wait(timeout=2)
    except subprocess.TimeoutExpired:
        pass
    try:
        os.killpg(child.pid, signal.SIGKILL)
    except ProcessLookupError:
        pass
    child.wait()


def supervise(command, resource, agent, thread, work, timeout, backend, record):
    owned = None
    child = None
    code = 78
    reservation_requested = reservation_replied = False
    try:
        backend.registered(agent, thread)
        row = backend.lookup(resource)
        if row is None:
            reservation_requested = True
            reply = backend.reserve(resource, agent, work)
            reservation_replied = True
            if not isinstance(reply, dict):
                raise Refused('Reservation reply is not an object')
            if 'granted' in reply:
                if reply['granted'] is not True:
                    raise Refused('Reservation was not granted')
                row = reply.get('reservation')
            else:
                row = reply
            # Release only a newly acquired reservation explicitly bound to us.
            if isinstance(row, dict) and row.get('holder') == agent and row.get('resource') == resource and isinstance(row.get('id'), str) and row['id']:
                owned = row['id']
        if not valid(row, resource, agent, work):
            raise Refused('Fresh exact host/holder/work full-share admission required; foreign/floor/expired/unknown admission refuses')
        identity = row['id']
        backend.registered(agent, thread)
        before = backend.lookup(resource)
        if not valid(before, resource, agent, work) or before['id'] != identity:
            raise Refused('Admission changed before spawn')
        record.update(admission=identity, borrowed=owned is None, status='admitted')
        child = subprocess.Popen(command, start_new_session=True)
        record.update(pid=child.pid, status='running')
        deadline = time.monotonic() + timeout
        heartbeat_at = time.monotonic() + 30
        while True:
            if time.monotonic() >= deadline:
                raise Refused('Execution deadline exceeded')
            row = backend.lookup(resource)
            if not valid(row, resource, agent, work) or row['id'] != identity:
                raise Refused('Admission lost or replaced during execution')
            if time.monotonic() >= heartbeat_at:
                if owned:
                    backend.heartbeat(owned)
                elif hasattr(backend, 'heartbeat_executor'):
                    backend.heartbeat_executor()
                heartbeat_at = time.monotonic() + 30
            try:
                code = child.wait(timeout=min(2, max(0.001, deadline - time.monotonic())))
                after = backend.lookup(resource)
                if not valid(after, resource, agent, work) or after['id'] != identity:
                    code = 78
                    raise Refused('Admission changed before child completion readback')
                record['status'] = 'success' if code == 0 else 'child_failed'
                break
            except subprocess.TimeoutExpired:
                pass
    except (Exception, KeyboardInterrupt) as error:
        record.update(status='refused_or_stopped', reason=str(error))
        print('Ma\'at refused/stopped: '+str(error), file=sys.stderr)
    finally:
        if child is not None:
            try:
                stop(child)
            except Exception as error:
                record.update(cleanup_debt='Child group cleanup unconfirmed: '+str(error))
                code = 78
        if owned:
            try:
                backend.release(owned)
            except Exception as error:
                record.update(cleanup_debt='Reservation release unconfirmed: '+str(error))
                code = 78
        elif reservation_requested and not reservation_replied:
            record['cleanup_debt'] = 'Reservation request outcome unknown; inspect own host/holder/work before retry. No child was launched.'
        record['exit_code'] = code
    return code


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--work', required=True)
    parser.add_argument('--agent', default=os.environ.get('SIRSI_AGENT_ID'))
    parser.add_argument('--thread', default=os.environ.get('SIRSI_THREAD_ID'))
    parser.add_argument('--register-executor', action='store_true',
                        help='Bind this job process on its execution host; never reuse a foreign host PID')
    parser.add_argument('--timeout', type=int, default=300)
    parser.add_argument('command', nargs=argparse.REMAINDER)
    args = parser.parse_args()
    command = args.command[1:] if args.command[:1] == ['--'] else args.command
    if not command or not re.fullmatch(r'[A-Za-z0-9_.-]{1,128}', args.work) or not 1 <= args.timeout <= 7200:
        parser.error('Supply a bounded timeout, simple work identity and foreground command after --')
    # No caller-set host or admission exemption; host is derived locally.
    resource = local_resource()
    folder = Path.home()/'.sirsi/evidence/maat-runs'/uuid.uuid4().hex
    folder.mkdir(parents=True, mode=0o700)
    record = {'status':'pending', 'resource':resource, 'work':args.work,
              'argv_sha256':hashlib.sha256(b'\0'.join(os.fsencode(x) for x in command)).hexdigest()}
    def interrupted(signum, frame):
        raise InterruptedError('Supervisor interrupted')
    signal.signal(signal.SIGTERM, interrupted)
    signal.signal(signal.SIGINT, interrupted)
    target = folder/'terminal.json'
    fd = os.open(target, os.O_CREAT|os.O_EXCL|os.O_WRONLY|os.O_NOFOLLOW, 0o600)
    with os.fdopen(fd, 'w') as output:
        json.dump(record, output); output.flush(); os.fsync(output.fileno())
    backend = Native()
    code = 78
    try:
        if args.register_executor:
            # Normal authenticated fabric transport selects the destination.
            # The logical lane is unchanged; its execution instance is local
            # to that destination. Never compare an origin-host PID locally.
            os.environ['SIRSI_AGENT_ID'] = args.agent or ''
            os.environ.pop('SIRSI_THREAD_ID', None)
            args.thread = backend.register_executor(args.agent)
            record.update(executor_thread=args.thread, agent=args.agent)
        # IDs are selectors, not authority: Native.registered verifies actual
        # process ancestry/start before admission and again immediately pre-spawn.
        os.environ['SIRSI_AGENT_ID'] = args.agent or ''
        os.environ['SIRSI_THREAD_ID'] = args.thread or ''
        code = supervise(command, resource, args.agent,
                         args.thread, args.work, args.timeout, backend, record)
    except (Exception, KeyboardInterrupt) as error:
        record.update(status='refused_or_stopped', reason=str(error), exit_code=78)
        code = 78
    finally:
        try:
            backend.close_executor()
        except Exception as error:
            record['cleanup_debt'] = record.get('cleanup_debt', '') + '; Executor registration close unconfirmed: '+str(error)
            record['exit_code'] = 78
            code = 78
        record['exit_code'] = code
        with target.open('w') as output:
            json.dump(record, output, indent=2); output.flush(); os.fsync(output.fileno())
        print('Ma\'at terminal: '+str(target), file=sys.stderr)
    return code


if __name__ == '__main__':
    sys.exit(main())
