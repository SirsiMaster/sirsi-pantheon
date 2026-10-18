#!/usr/bin/env python3
"""Cooperative native Ma'at launch supervisor, not a security sandbox."""
import argparse
import ctypes
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


class TransientTransport(Refused):
    """A bounded transport failure; never evidence that a mutation committed."""


class _BSDInfo(ctypes.Structure):
    # Darwin's public sys/proc_info.h: PROC_PIDTBSDINFO (flavor 3).
    _fields_ = [(name, ctypes.c_uint32) for name in (
        'flags', 'status', 'xstatus', 'pid', 'ppid', 'uid', 'gid',
        'ruid', 'rgid', 'svuid', 'svgid', 'reserved')] + [
        ('comm', ctypes.c_char * 16), ('name', ctypes.c_char * 32),
        *[(name, ctypes.c_uint32) for name in (
            'nfiles', 'pgid', 'jobc', 'tdev', 'tpgid')],
        ('nice', ctypes.c_int32), ('start_sec', ctypes.c_uint64),
        ('start_usec', ctypes.c_uint64)]


def process_identity(pid):
    """Read kernel identity without spawning ps or trusting a cached PID."""
    if sys.platform != 'darwin' or type(pid) is not int or pid <= 1:
        raise Refused('Native process identity unavailable on this execution host')
    library = ctypes.CDLL('/usr/lib/libproc.dylib', use_errno=True)
    lookup = library.proc_pidinfo
    lookup.argtypes = [ctypes.c_int, ctypes.c_int, ctypes.c_uint64,
                       ctypes.c_void_p, ctypes.c_int]
    lookup.restype = ctypes.c_int
    row = _BSDInfo()
    size = ctypes.sizeof(row)
    read = lookup(pid, 3, 0, ctypes.byref(row), size)
    if read != size or row.pid != pid or row.start_sec <= 0:
        raise Refused('Kernel process identity unavailable or changed for PID '+str(pid))
    # Match Go time.ANSIC / ps lstart, independent of caller locale.
    stamp = time.localtime(row.start_sec)
    start = ('Mon Tue Wed Thu Fri Sat Sun'.split()[stamp.tm_wday]+' '+
             'Jan Feb Mar Apr May Jun Jul Aug Sep Oct Nov Dec'.split()[stamp.tm_mon-1]+
             f' {stamp.tm_mday:2d} {stamp.tm_hour:02d}:{stamp.tm_min:02d}:{stamp.tm_sec:02d} {stamp.tm_year:04d}')
    return row.ppid, start


def local_resource():
    name = socket.gethostname().split('.')[0].lower()
    names = {'mac': 'm5', 'm5': 'm5', 'm1': 'm1', 'm5-studio': 'studio'}
    if name not in names:
        raise Refused('Host has no enrolled local resource mapping')
    return names[name]


def valid(row, resource, agent, work, cpu_cores=None):
    if not isinstance(row, dict):
        return False
    if (row.get('status'), row.get('resource'), row.get('holder'), row.get('work')) != ('active', resource, agent, work):
        return False
    if row.get('share') != 'full':
        if (row.get('share') != 'floor' or type(cpu_cores) is not int or cpu_cores < 1 or
                row.get('regime') != 'build' or type(row.get('cores')) is not int or row['cores'] < cpu_cores):
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
        self.cpu_cores = None
        self.owned_admission = None
        self.deadline = None

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
        if self.deadline is not None:
            timeout = min(timeout, self.deadline - time.monotonic())
            if timeout <= 0:
                raise Refused('Proven admission deadline exceeded')
        try:
            response = subprocess.run([self.cli, *args], check=True, capture_output=True,
                                      text=True, timeout=timeout)
        except subprocess.TimeoutExpired as error:
            if args[:2] in (('maat', 'heartbeat'), ('maat', 'status'), ('thread', 'heartbeat')):
                raise TransientTransport('Control transport timed out') from error
            raise
        except subprocess.CalledProcessError as error:
            detail = (error.stderr or '')[-1600:]
            # Denial takes precedence over transport text; unknown errors refuse.
            denied = re.search(r'(?i)\b(401|403|unauthori[sz]ed|forbidden|denied|revoked|released|expired|scope|mismatch)\b', detail)
            transient = re.search(r'(?i)(\bHTTP (502|503|504)\b|TLS handshake timeout|relay[^\n]*OUTCOME UNKNOWN)', detail)
            if not denied and transient:
                raise TransientTransport('Control transport uncertain: '+detail) from error
            raise Refused('Control call '+str(args[:2])+' failed: '+detail) from error
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
        self.executor_identity = matches[0]

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
            parent, start = process_identity(cursor)
            if cursor == anchor:
                if start != row.get('start_time'):
                    raise Refused('Registered anchor start identity changed')
                return
            cursor = parent
        raise Refused('Registered anchor is not this launcher ancestry')

    def lookup(self, resource):
        if self.owned_admission:
            # Read-only exact-ID recovery: never replay an uncertain heartbeat
            # just to discover whether it succeeded. Status includes terminal rows.
            rows = self.call('maat', 'status', resource, '--json')
            if not isinstance(rows, list):
                raise Refused('Reservation readback is malformed')
            matches = [row for row in rows if isinstance(row, dict) and
                       row.get('id') == self.owned_admission]
            if len(matches) != 1:
                raise Refused('Exact admission missing or duplicated')
            return matches[0]
        return self.call('maat', 'who-is-on', resource, '--json')

    def reserve(self, resource, agent, work):
        return self.call('maat', 'reserve', resource, '--holder', agent, '--work', work,
                         '--regime', 'build' if self.cpu_cores else 'loaded', '--lease-ttl', '120', '--exempt-pid',
                         str(os.getpid()), '--repro', str(Path(__file__).resolve()), '--json')

    def heartbeat(self, identity):
        if self.executor_identity is None:
            raise Refused('Verified executor identity required for renewal')
        self.verify_runtime(self.executor_identity, self.executor_identity['agent_id'],
                            self.executor_identity['thread_id'])
        return self.call('maat', 'heartbeat', identity, '--json')

    def heartbeat_executor(self):
        if self.executor_thread:
            self.verify_runtime(self.executor_identity, self.executor_identity['agent_id'],
                                self.executor_identity['thread_id'])
            self.call('thread', 'heartbeat', '--thread', self.executor_thread, '--json', timeout=30, json_result=False)

    def release(self, identity):
        self.call('maat', 'release', identity, '--json')


class ProvenLease:
    """Only a confirmed mutation can advance our monotonic validity bound."""
    FIELDS = ('id', 'resource', 'holder', 'work', 'exempt_pid', 'share', 'regime',
              'cores', 'mem_gb', 'mem_share')

    def __init__(self, row, started, resource, agent, work, cpu_cores, owned):
        self.scope = {key: row.get(key) for key in self.FIELDS}
        self.resource, self.agent, self.work = resource, agent, work
        self.cpu_cores = cpu_cores
        if owned and row.get('exempt_pid') != os.getpid():
            raise Refused('Own admission executor does not match this supervisor')
        self.confirm(row, started)

    def check(self, row):
        if (not valid(row, self.resource, self.agent, self.work, self.cpu_cores) or
                any(row.get(key) != value for key, value in self.scope.items())):
            raise Refused('Exact admission scope/executor lost or replaced')

    def confirm(self, row, started):
        self.check(row)
        stamp = dt.datetime.fromisoformat(row['heartbeat_at'].replace('Z', '+00:00'))
        age = (dt.datetime.now(dt.timezone.utc) - stamp).total_seconds()
        # Anchor BEFORE the request, not at response receipt. Timestamp age also
        # discounts pre-existing/rounded heartbeats; readback never calls confirm.
        self.deadline = started + row['lease_ttl_sec'] - max(0, age)
        if time.monotonic() >= self.deadline:
            raise Refused('Confirmed lease already exhausted')


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
    cpu_cores = getattr(backend, 'cpu_cores', None)
    try:
        backend.registered(agent, thread)
        confirmed_at = time.monotonic()
        row = backend.lookup(resource)
        if row is None or (cpu_cores and not valid(row, resource, agent, work, cpu_cores)):
            reservation_requested = True
            confirmed_at = time.monotonic()
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
                if isinstance(backend, Native):
                    backend.owned_admission = owned
        if not valid(row, resource, agent, work, cpu_cores):
            raise Refused('Fresh exact host/holder/work full-share admission required; foreign/floor/expired/unknown admission refuses')
        identity = row['id']
        lease = ProvenLease(row, confirmed_at, resource, agent, work, cpu_cores, owned)
        if isinstance(backend, Native):
            backend.owned_admission = identity  # exact readback, borrowed or owned
            backend.deadline = lease.deadline
        backend.registered(agent, thread)
        before = backend.lookup(resource)
        lease.check(before)
        if time.monotonic() >= lease.deadline:
            raise Refused('Proven admission deadline exceeded before spawn')
        record.update(admission=identity, borrowed=owned is None, status='admitted')
        child_env = os.environ.copy()
        if cpu_cores:
            # Cooperative CPU build mask, not a kernel sandbox or GPU admission.
            # Only an explicit caller opt-in can consume a bounded build floor.
            for key in ('GOMAXPROCS','OMP_NUM_THREADS','OPENBLAS_NUM_THREADS',
                        'VECLIB_MAXIMUM_THREADS','NUMEXPR_NUM_THREADS','BLIS_NUM_THREADS'):
                child_env[key] = str(cpu_cores)
            record.update(cpu_cores=cpu_cores, share=before['share'])
        child = subprocess.Popen(command, start_new_session=True, env=child_env)
        record.update(pid=child.pid, status='running')
        deadline = time.monotonic() + timeout
        control_at = time.monotonic() + min(30, max(0.001, (lease.deadline - time.monotonic()) / 3))
        backoff = 1
        if isinstance(backend, Native):
            backend.deadline = min(deadline, lease.deadline)
        while True:
            if time.monotonic() >= min(deadline, lease.deadline):
                raise Refused('Execution or proven admission deadline exceeded')
            if time.monotonic() >= control_at:
                try:
                    # Local child monitoring is cheap and frequent. Remote
                    # revocation observation/renewal is at most 30s apart,
                    # shortened for remaining proven TTL. Uncertain mutations
                    # always require exact readback before the next renewal.
                    row = backend.lookup(resource)
                    lease.check(row)
                    if time.monotonic() >= min(deadline, lease.deadline):
                        raise Refused('Execution or proven admission deadline exceeded')
                    if child.poll() is None:
                        if owned:
                            started = time.monotonic()
                            renewed = backend.heartbeat(owned)
                            if time.monotonic() >= lease.deadline:
                                raise Refused('Renewal arrived after proven deadline')
                            lease.confirm(renewed, started)
                            if isinstance(backend, Native):
                                backend.deadline = min(deadline, lease.deadline)
                        if hasattr(backend, 'heartbeat_executor'):
                            backend.heartbeat_executor()
                    control_at = time.monotonic() + min(30, max(0.001, (lease.deadline - time.monotonic()) / 3))
                    backoff = 1
                except TransientTransport as error:
                    record['transport_retries'] = record.get('transport_retries', 0) + 1
                    record['last_transport_error'] = str(error)
                    control_at = time.monotonic() + backoff
                    backoff = min(8, backoff * 2)
            remaining = min(deadline, lease.deadline) - time.monotonic()
            if remaining <= 0:
                raise Refused('Execution or proven admission deadline exceeded')
            try:
                code = child.wait(timeout=min(2, remaining, max(0.001, control_at - time.monotonic())))
                try:
                    after = backend.lookup(resource)
                    lease.check(after)
                except TransientTransport:
                    # Terminal compute is never restarted or kept alive for a
                    # readback. Cleanup runs immediately; uncertainty is debt.
                    raise Refused('Child completion admission readback unavailable')
                if time.monotonic() >= min(deadline, lease.deadline):
                    raise Refused('Child completion exceeded proven deadline')
                record['status'] = 'success' if code == 0 else 'child_failed'
                break
            except subprocess.TimeoutExpired:
                pass
    except (Exception, KeyboardInterrupt) as error:
        record.update(status='refused_or_stopped', reason=str(error))
        print('Ma\'at refused/stopped: '+str(error), file=sys.stderr)
    finally:
        if record.get('status') == 'refused_or_stopped':
            code = 78
        if isinstance(backend, Native):
            backend.deadline = None  # cleanup must run even after lease expiry
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
    parser.add_argument('--cpu-cores', type=int,
                        help='Explicit cooperative CPU-only build mask; may use a sufficient granted build floor, never GPU/model admission')
    parser.add_argument('command', nargs=argparse.REMAINDER)
    args = parser.parse_args()
    command = args.command[1:] if args.command[:1] == ['--'] else args.command
    if not command or not re.fullmatch(r'[A-Za-z0-9_.-]{1,128}', args.work) or not 1 <= args.timeout <= 7200:
        parser.error('Supply a bounded timeout, simple work identity and foreground command after --')
    if args.cpu_cores is not None and not 1 <= args.cpu_cores <= 64:
        parser.error('CPU build mask must be between 1 and 64')
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
    backend.cpu_cores = args.cpu_cores
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
