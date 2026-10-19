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
import shlex
import signal
import socket
import subprocess
import sys
import time
import uuid


class Refused(RuntimeError):
    pass


def require_job_safe_command(argv, depth=0):
    """Admission is NOT authority to signal arbitrary jobs.

    Reject known shell/process stop front doors, including shell-wrapped forms.
    This is a cooperative accident guard, not a sandbox for arbitrary programs.
    Supervisor-owned child termination below remains independent of this rule.
    """
    if depth > 12 or not argv:
        raise Refused('Unverifiable stop-command wrapper; use owner-mediated job control')
    words = list(argv)
    name = Path(words[0]).name
    if name == 'sirsi-maat-run':
        if '--' not in words:
            raise Refused('Supervisor payload separator missing')
        require_job_safe_command(words[words.index('--') + 1:], depth + 1)
        return
    if name in ('pkill', 'killall', 'kill', 'skill', 'fuser'):
        if name == 'kill' and words[1:] in (['-l'], ['-L']):
            return
        raise Refused('Compute admission does not authorize process signals; '
                      'request the exact job from its lifecycle controller, never pattern/PID kill')
    if ((name == 'launchctl' and len(words) > 1 and
         (words[1] in ('bootout', 'unload', 'remove', 'stop', 'kill') or
          (words[1] == 'kickstart' and '-k' in words[2:]))) or
        (name == 'systemctl' and any(word in ('stop', 'restart', 'kill', 'disable')
                                    for word in words[1:])) or
        (name == 'brew' and words[1:2] == ['services'] and
         any(word in ('stop', 'restart', 'kill') for word in words[2:]))):
        raise Refused('Compute admission does not authorize service retirement/restart; '
                      'route the exact service instance to its lifecycle controller')
    if name in ('env', 'command', 'exec', 'nohup', 'sudo', 'timeout'):
        tail = words[1:]
        while tail and (tail[0].startswith('-') or '=' in tail[0]):
            tail.pop(0)
        if name == 'timeout' and tail:
            tail.pop(0)
        require_job_safe_command(tail, depth + 1)
    elif name in ('sh', 'bash', 'zsh', 'dash', 'ksh'):
        if len(words) > 2 and words[1].startswith('-') and 'c' in words[1]:
            lexer = shlex.shlex(words[2], posix=True, punctuation_chars=';&|()<>')
            lexer.whitespace_split = True
            lexer.commenters = '#'
            try:
                tokens = list(lexer)
            except ValueError as error:
                raise Refused('Malformed shell payload') from error
            segment = []
            for token in tokens + [';']:
                if token and set(token) <= set(';&|()<>'):
                    if segment:
                        while segment and '=' in segment[0] and not segment[0].startswith('/'):
                            segment.pop(0)
                        if segment:
                            require_job_safe_command(segment, depth + 1)
                    segment = []
                else:
                    segment.append(token)


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
        self.executor_controls = False
        self.release_scope = None

    def register_executor(self, agent):
        if not re.fullmatch(r'[A-Za-z0-9_.-]{1,128}', agent or ''):
            raise Refused('A concrete fabric agent is required')
        identity = 'thr-'+uuid.uuid4().hex
        # A job is not its launching IDE conversation. CLI register/close
        # publish/remove provider-session markers, so inherited provider IDs
        # must never reach these control subprocesses. Keep the payload's
        # environment intact; isolate only this executor's control calls.
        self.executor_controls = True
        try:
            row = self.call('thread', 'register', '--agent', agent, '--surface', 'worker', '--thread', identity,
                            '--anchor-pid', str(os.getpid()), '--repo', os.getcwd(),
                            '--json', timeout=30)
        except TransientTransport:
            # Retain the requested identity and resolve that exact registration;
            # never create another executor to discover an uncertain outcome.
            self.executor_thread = identity
            row = self.registration_observation(identity)
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
        control_env = None
        if self.executor_controls:
            control_env = os.environ.copy()
            for key in ('CLAUDE_CODE_SESSION_ID', 'CODEX_SESSION_ID', 'CODEX_THREAD_ID'):
                control_env.pop(key, None)
            control_env.pop('SIRSI_THREAD_ID', None)
            if self.executor_thread:
                control_env['SIRSI_THREAD_ID'] = self.executor_thread
        try:
            response = subprocess.run([self.cli, *args], check=True, capture_output=True,
                                      text=True, timeout=timeout, env=control_env)
        except subprocess.TimeoutExpired as error:
            if args[:2] in (('maat', 'heartbeat'), ('maat', 'status'), ('thread', 'heartbeat'), ('thread', 'register')):
                raise TransientTransport('Control transport timed out') from error
            raise
        except subprocess.CalledProcessError as error:
            detail = (error.stderr or '')[-1600:]
            # Denial takes precedence over transport text; unknown errors refuse.
            denied = re.search(r'(?i)\b(401|403|unauthori[sz]ed|forbidden|denied|revoked|released|expired|scope|mismatch)\b', detail)
            transient = re.search(r'(?i)(\bHTTP (502|503|504)\b|TLS handshake timeout|relay[^\n]*OUTCOME UNKNOWN)', detail)
            # RegisterThread saves its own row before opportunistic terminal
            # pruning. A lost prune CAS can therefore report failure AFTER our
            # exact registration committed. Resolve only our generated row;
            # never repeat registration or relax the lifecycle fence.
            partial_registration = (args[:2] == ('thread', 'register') and
                                    re.search(r'thread "[A-Za-z0-9_-]+" prune lost lifecycle fence', detail))
            if not denied and (transient or partial_registration):
                raise TransientTransport('Control transport uncertain: '+detail) from error
            raise Refused('Control call '+str(args[:2])+' failed: '+detail) from error
        return (json.loads(response.stdout) if response.stdout.strip() else None) if json_result else response.stdout

    def registration_observation(self, thread):
        """Bound registration transport retries; no compute permission is created."""
        prior_deadline = self.deadline
        self.deadline = min(prior_deadline, time.monotonic() + 120) if prior_deadline is not None else time.monotonic() + 120
        backoff = 1
        try:
            while True:
                if time.monotonic() >= self.deadline:
                    raise TransientTransport('Registration observation recovery budget exhausted')
                try:
                    return self.call('thread', 'heartbeat', '--thread', thread, '--json', timeout=30)
                except TransientTransport:
                    remaining = self.deadline - time.monotonic()
                    if remaining <= 0:
                        raise
                    time.sleep(min(backoff, remaining))
                    backoff = min(8, backoff * 2)
        finally:
            self.deadline = prior_deadline

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
        observed = self.registration_observation(thread)
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
                         '--regime', 'build' if self.cpu_cores else 'loaded', '--lease-ttl', '240', '--exempt-pid',
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
        try:
            return self.call('maat', 'release', identity, '--json')
        except TransientTransport as uncertain:
            # The write may already have committed. Never blindly replay it or
            # release a reservation selected only by a lane/work name.
            if not self.release_scope or self.release_scope.get('id') != identity:
                raise
            prior_deadline = self.deadline
            self.deadline = time.monotonic() + 30
            try:
                for attempt in range(3):
                    try:
                        row = self.lookup(self.release_scope['resource'])
                        if any(row.get(k) != v for k, v in self.release_scope.items()):
                            raise Refused('Uncertain release readback changed exact reservation identity')
                        if row.get('status') in ('released', 'expired', 'invalidated', 'done'):
                            return row
                        if row.get('status') != 'active':
                            raise Refused('Uncertain release readback has unknown status')
                        # Exact readback proves this same owned grant remains
                        # active. A repeated release is now deliberate, not a
                        # replay used to discover an unknown write outcome.
                        return self.call('maat', 'release', identity, '--json')
                    except TransientTransport:
                        if attempt == 2 or time.monotonic() + 2 >= self.deadline:
                            raise uncertain
                        time.sleep(1 << attempt)
                raise uncertain
            finally:
                self.deadline = prior_deadline


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


def check_deadlines(execution_deadline, lease, started, timeout, record):
    """A run budget and a renewable grant are independent limits, not one error."""
    now = time.monotonic()
    record.update(execution_timeout_sec=timeout,
                  execution_elapsed_sec=round(max(0, now - started), 3),
                  proven_lease_remaining_sec=round(max(0, lease.deadline - now), 3))
    if now >= execution_deadline:
        record['stop_cause'] = record.get('execution_bound', 'execution_timeout')
        if record['stop_cause'] == 'job_timeout':
            raise Refused('Recurring job total lifetime exceeded')
        raise Refused('Execution timeout exceeded (--timeout '+str(timeout)+' seconds); '
                      'lease renewal does not extend the configured run budget')
    if now >= lease.deadline:
        record['stop_cause'] = 'proven_lease_expired'
        raise Refused('Proven admission deadline exceeded; no confirmed renewal remains')


def refusal_details(reply):
    """Persist the canonical decision's non-secret identities, never a guessed holder."""
    if not isinstance(reply, dict):
        return {}
    fields = ('id', 'resource', 'holder', 'work', 'status', 'share', 'cores',
              'heartbeat_at', 'lease_ttl_sec', 'est_end', 'reason', 'exempt_pid')
    result = {}
    for key in ('reservation', 'conflict'):
        row = reply.get(key)
        if isinstance(row, dict):
            result[key] = {field: row[field] for field in fields if field in row}
    if isinstance(reply.get('pending_cedes'), list):
        result['pending_cedes'] = [x for x in reply['pending_cedes'] if isinstance(x, str)]
    return result


def supervise(command, resource, agent, thread, work, timeout, backend, record):
    owned = None
    child = None
    code = 78
    reservation_requested = reservation_replied = False
    cpu_cores = getattr(backend, 'cpu_cores', None)
    record['execution_timeout_sec'] = timeout
    try:
        require_job_safe_command(command)
        backend.registered(agent, thread)
        confirmed_at = time.monotonic()
        # A destination-local executor owns this job, not the primary holder
        # returned by who-is-on. Loaded jobs previously skipped Reserve when
        # any row existed, creating phantom refusals; an old same-lane/work
        # row could instead be borrowed without this executor renewing it.
        # Ask the atomic scheduler for this executor's own exact decision.
        fresh_executor = isinstance(backend, Native) and backend.executor_thread is not None
        row = None if fresh_executor else backend.lookup(resource)
        if fresh_executor or row is None or (cpu_cores and not valid(row, resource, agent, work, cpu_cores)):
            reservation_requested = True
            confirmed_at = time.monotonic()
            reply = backend.reserve(resource, agent, work)
            reservation_replied = True
            if not isinstance(reply, dict):
                raise Refused('Reservation reply is not an object')
            record['admission_decision'] = refusal_details(reply)
            if 'granted' in reply:
                if reply['granted'] is not True:
                    raise Refused('Reservation was not granted: '+json.dumps(record['admission_decision'], sort_keys=True))
                row = reply.get('reservation')
            else:
                row = reply
            # Release only a newly acquired reservation explicitly bound to us.
            if isinstance(row, dict) and row.get('holder') == agent and row.get('resource') == resource and isinstance(row.get('id'), str) and row['id']:
                owned = row['id']
                if isinstance(backend, Native):
                    backend.owned_admission = owned
                    backend.release_scope = {key: row.get(key) for key in
                                             ('id', 'resource', 'holder', 'work', 'exempt_pid')}
        if not valid(row, resource, agent, work, cpu_cores):
            raise Refused('Fresh exact host/holder/work sufficient admission required: '+
                          json.dumps(record.get('admission_decision', refusal_details({'reservation': row})), sort_keys=True))
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
        job_deadline = getattr(backend, 'job_deadline', None)
        if isinstance(job_deadline, (int, float)) and time.monotonic() >= job_deadline:
            record['stop_cause'] = 'job_timeout'
            raise Refused('Recurring job total lifetime exceeded before spawn')
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
        started_at = time.monotonic()
        deadline = started_at + timeout
        if isinstance(job_deadline, (int, float)) and job_deadline <= deadline:
            deadline = job_deadline
            record['execution_bound'] = 'job_timeout'
        control_at = time.monotonic() + min(30, max(0.001, (lease.deadline - time.monotonic()) / 3))
        backoff = 1
        if isinstance(backend, Native):
            backend.deadline = min(deadline, lease.deadline)
        while True:
            check_deadlines(deadline, lease, started_at, timeout, record)
            if time.monotonic() >= control_at:
                try:
                    # Local child monitoring is cheap and frequent. Remote
                    # revocation observation/renewal is at most 30s apart,
                    # shortened for remaining proven TTL. Uncertain mutations
                    # always require exact readback before the next renewal.
                    row = backend.lookup(resource)
                    lease.check(row)
                    check_deadlines(deadline, lease, started_at, timeout, record)
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
                check_deadlines(deadline, lease, started_at, timeout, record)
            try:
                code = child.wait(timeout=min(2, remaining, max(0.001, control_at - time.monotonic())))
                try:
                    after = backend.lookup(resource)
                    lease.check(after)
                except TransientTransport:
                    # Terminal compute is never restarted or kept alive for a
                    # readback. Cleanup runs immediately; uncertainty is debt.
                    raise Refused('Child completion admission readback unavailable')
                check_deadlines(deadline, lease, started_at, timeout, record)
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


def supervise_iterations(command, resource, agent, thread, work, timeout, backend,
                         record, iterations=1, interval=0, job_timeout=None, persist=None):
    """One real executor, fresh admission per iteration, no detached payloads.

    launchd may own this foreground supervisor; an IDE's PID is not reused.
    Waiting consumes no compute reservation. First failure stops the job.
    """
    if iterations == 1:
        return supervise(command, resource, agent, thread, work, timeout, backend, record)
    if not getattr(backend, 'executor_thread', None):
        raise Refused('Recurring jobs require their own registered executor')
    end = time.monotonic() + job_timeout
    backend.job_deadline = end
    record['iterations'] = []
    for index in range(iterations):
        remaining = end - time.monotonic()
        if remaining <= 0:
            record.update(status='refused_or_stopped', stop_cause='job_timeout', exit_code=78)
            return 78
        # Never borrow a previous iteration's terminal grant or retain its
        # cleanup state. Each iteration acquires/reconciles its own grant.
        backend.owned_admission = None
        backend.release_scope = None
        backend.deadline = None
        current = {'iteration': index + 1}
        code = supervise(command, resource, agent, thread, work, min(timeout, remaining), backend, current)
        record['iterations'].append(current)
        record.update(status='waiting' if code == 0 else 'refused_or_stopped', exit_code=code)
        if persist:
            persist()
        if code != 0:
            return code
        if index + 1 == iterations:
            record['status'] = 'success'
            return 0
        wake = time.monotonic() + interval
        while time.monotonic() < wake:
            remaining = end - time.monotonic()
            if remaining <= 0:
                record.update(status='refused_or_stopped', stop_cause='job_timeout', exit_code=78)
                return 78
            time.sleep(max(0, min(30, remaining, wake - time.monotonic())))
            try:
                backend.heartbeat_executor()
            except TransientTransport as error:
                # No payload or reservation exists while idle. Retain the
                # diagnostic; the next iteration must independently register
                # and obtain fresh admission before it can spawn anything.
                record['idle_transport_error'] = str(error)
    return 0


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--work', required=True)
    parser.add_argument('--agent', default=os.environ.get('SIRSI_AGENT_ID'))
    parser.add_argument('--thread', default=os.environ.get('SIRSI_THREAD_ID'))
    parser.add_argument('--register-executor', action='store_true',
                        help='Bind this job process on its execution host; never reuse a foreign host PID')
    parser.add_argument('--timeout', type=int, default=300)
    parser.add_argument('--iterations', type=int, default=1,
                        help='Bounded foreground iterations; each obtains a fresh admission')
    parser.add_argument('--interval', type=int, default=0,
                        help='Seconds without a compute lease between iterations')
    parser.add_argument('--job-timeout', type=int,
                        help='Total recurring supervisor lifetime, at most 86400 seconds')
    parser.add_argument('--cpu-cores', type=int,
                        help='Explicit cooperative CPU-only build mask; may use a sufficient granted build floor, never GPU/model admission')
    parser.add_argument('command', nargs=argparse.REMAINDER)
    args = parser.parse_args()
    command = args.command[1:] if args.command[:1] == ['--'] else args.command
    if not command or not re.fullmatch(r'[A-Za-z0-9_.-]{1,128}', args.work) or not 1 <= args.timeout <= 7200:
        parser.error('Supply a bounded timeout, simple work identity and foreground command after --')
    if args.cpu_cores is not None and not 1 <= args.cpu_cores <= 64:
        parser.error('CPU build mask must be between 1 and 64')
    if not 1 <= args.iterations <= 1000 or not 0 <= args.interval <= 3600:
        parser.error('Iterations must be 1..1000 and interval 0..3600 seconds')
    if args.iterations > 1 and (not args.register_executor or args.job_timeout is None):
        parser.error('Recurring jobs require --register-executor and an explicit --job-timeout')
    if args.job_timeout is not None and not 1 <= args.job_timeout <= 86400:
        parser.error('Job timeout must be 1..86400 seconds')
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
    def persist():
        with target.open('w') as output:
            json.dump(record, output, indent=2); output.flush(); os.fsync(output.fileno())
    code = 78
    try:
        require_job_safe_command(command)
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
        code = supervise_iterations(command, resource, args.agent, args.thread,
                                    args.work, args.timeout, backend, record,
                                    args.iterations, args.interval, args.job_timeout, persist)
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
        persist()
        print('Ma\'at terminal: '+str(target), file=sys.stderr)
    return code


def check_tool_stop(payload):
    """Pure pretool check; does not register, acquire, spawn, or signal."""
    if not isinstance(payload, dict):
        raise Refused('Malformed hook input')
    if payload.get('tool_name') not in ('Bash', 'bash', 'exec_command', None):
        # The existing hook also receives Edit/Write/NotebookEdit. Their
        # original file-permission guard still runs; no shell argv exists.
        return
    tool = payload.get('tool_input', payload.get('input', {}))
    if not isinstance(tool, dict):
        raise Refused('Malformed tool input')
    command = tool.get('command', tool.get('cmd'))
    if not isinstance(command, str) or len(command) > 65536:
        raise Refused('Missing or oversized shell command')
    require_job_safe_command(['sh', '-c', command])


if __name__ == '__main__':
    if sys.argv[1:] == ['--check-stop-command']:
        try:
            raw = sys.stdin.read(65537)
            if len(raw) > 65536:
                raise Refused('Hook payload exceeds bound')
            check_tool_stop(json.loads(raw))
        except (ValueError, TypeError, Refused) as error:
            print(str(error), file=sys.stderr)
            sys.exit(2)
        sys.exit(0)
    sys.exit(main())
