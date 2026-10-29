#!/usr/bin/env python3
"""Development qualification, never a product installer or a local-host override."""
import hashlib
import json
import os
import pathlib
import pwd
import signal
import shutil
import subprocess
import sys
import tempfile
import time

TOKEN = 'sirsimaster/tools/sirsi-pantheon'
TAP = '/opt/homebrew/Library/Taps/sirsimaster/homebrew-tools'
CASK = TAP + '/Casks/sirsi-pantheon.rb'
APP = '/Applications/Pantheon.app'
CLI = '/opt/homebrew/bin/sirsi'
LABEL = 'ai.sirsi.pantheon'
PINS = {
    '140': {'tap': '1a894d05520e253a1d0f92c45e50ec7ccdeb8b79',
            'blob': '9d004d0f339cbc35f2d2a080e2eca09c4d7fe0a3',
            'cask': 'e8f513fac2a5aed6545ad2b92e716a557c77f7b0d1ef09c568477ed0d2659f72',
            'commit': 'b4d09aeb531a3e5a72fa56d205fa02644b708e5d',
            'cli': '30eb56af1e1b8c4ca6408466ff49c38debfb0b5f653501fed43fe09559e6325f'},
    '141': {'tap': 'a4388a1fa081a4b913ea9fe054596f765beddc65',
            'blob': '01c86d8ed66d5d31e378e3e5f06028ea4f879389',
            'cask': '54125c3e428b7125dcc91a3d63ca6ea88030e32f89e250a71cfae876ffc22a19',
            'commit': 'c1651420728e90063f55ffd314659e6b51cacd9e',
            'cli': '56a35f5625515c0de22b73443bcb0f19ab23fb6a456da10971fd28334bf1b90f'},
}


def digest(data):
    return hashlib.sha256(data).hexdigest()


class Refused(RuntimeError):
    pass


class Interrupted(RuntimeError):
    pass


class RecoveryStopped(RuntimeError):
    def __init__(self, debt):
        self.debt = debt
        super().__init__('unsafe owned operation stops all subsequent recovery')


class Ops:
    """Only production backend. No executable/path overrides from CLI or env."""
    def __init__(self):
        os.umask(0o077)
        self.root = pathlib.Path(tempfile.mkdtemp(prefix='pantheon141-lifecycle.',
                                                 dir=os.environ['RUNNER_TEMP']))
        self.deadline = time.monotonic() + 1800
        self.serial = 0
        self.env = dict(os.environ, HOMEBREW_NO_AUTO_UPDATE='1',
                        HOMEBREW_NO_ANALYTICS='1', HOMEBREW_NO_INSTALL_CLEANUP='1',
                        HOMEBREW_NO_ENV_HINTS='1')
        self.calls = []
        self.unjoined = []
        self.active_child = None
        self.spawn_pending = False
        self.cancel_requested = False
        self.cancel_signal = None
        self.recovering = False
        print('EVIDENCE_ROOT=' + str(self.root), flush=True)

    def run(self, argv, allowed=(0,)):
        end = min(time.monotonic() + 300, self.deadline)
        if end <= time.monotonic():
            raise Refused('finite execution deadline reached')
        if self.cancel_requested and not self.recovering:
            raise Interrupted('cancellation requested before command creation')
        self.serial += 1
        stem = self.root / ('%03d' % self.serial)
        with open(str(stem) + '.out', 'xb') as out, open(str(stem) + '.err', 'xb') as err:
            # Signals only set a flag. Creation/handle adoption cannot be
            # interrupted by our handler before the join path owns this child.
            self.spawn_pending = True
            try:
                child = subprocess.Popen(argv, stdout=out, stderr=err,
                                         env=self.env, start_new_session=True)
            except BaseException:
                self.unjoined.append('spawn result unknown')
                raise
            self.active_child = child
            self.spawn_pending = False
            try:
                while True:
                    if self.cancel_requested and not self.recovering:
                        raise Interrupted('cancellation requested after owned spawn')
                    remaining = end - time.monotonic()
                    if remaining <= 0:
                        raise subprocess.TimeoutExpired(argv, 300)
                    try:
                        code = child.wait(timeout=min(0.25, remaining))
                        break
                    except subprocess.TimeoutExpired:
                        continue
            except BaseException:
                # Signal only this still-live child-created process group.
                try:
                    if child.poll() is None and os.getpgid(child.pid) == child.pid:
                        os.killpg(child.pid, signal.SIGTERM)
                        try:
                            child.wait(timeout=5)
                        except subprocess.TimeoutExpired:
                            os.killpg(child.pid, signal.SIGKILL)
                            child.wait(timeout=5)
                    os.killpg(child.pid, 0)
                except ProcessLookupError:
                    pass
                except BaseException:
                    self.unjoined.append(child.pid)
                else:
                    self.unjoined.append(child.pid)
                if not self.unjoined:
                    self.active_child = None
                self.calls.append({'argv': argv, 'interrupted': True, 'pid_pgid': child.pid})
                raise
        stdout = pathlib.Path(str(stem) + '.out').read_bytes()
        stderr = pathlib.Path(str(stem) + '.err').read_bytes()
        self.calls.append({'argv': argv, 'exit': code, 'pid_pgid': child.pid,
                           'stdout_sha256': digest(stdout), 'stderr_sha256': digest(stderr)})
        # An exited leader is not proof its owned group was joined.
        try:
            os.killpg(child.pid, 0)
        except ProcessLookupError:
            self.active_child = None
        except OSError:
            self.unjoined.append(child.pid)
            raise Refused('owned operation group absence unknown; no racing recovery')
        else:
            self.unjoined.append(child.pid)
            raise Refused('owned operation group remains; recovery must not race it')
        if code not in allowed:
            raise Refused('command failed: ' + repr(argv) + ': ' + str(code))
        return code, stdout, stderr

    def account(self):
        uid, euid = os.getuid(), os.geteuid()
        entry = pwd.getpwuid(uid)
        platform = self.run(['/usr/bin/uname', '-sm'])[1].decode().strip()
        version = self.run(['/usr/bin/sw_vers', '-productVersion'])[1].decode().strip()
        return {'uid': uid, 'euid': euid, 'name': entry.pw_name, 'home': entry.pw_dir,
                'env_home': os.environ.get('HOME'), 'platform': platform,
                'version': version, 'hosted': os.environ.get('RUNNER_ENVIRONMENT'),
                'actions': os.environ.get('GITHUB_ACTIONS'),
                'runner_os': os.environ.get('RUNNER_OS'), 'arch': os.environ.get('RUNNER_ARCH')}

    def exists(self, path):
        return os.path.lexists(path)

    def caller_path(self):
        return shutil.which('sirsi', path=self.env.get('PATH'))

    def recovery(self):
        signal.signal(signal.SIGINT, signal.SIG_IGN)
        signal.signal(signal.SIGTERM, signal.SIG_IGN)
        self.deadline = time.monotonic() + 300
        self.recovering = True

    def provenance(self):
        source = pathlib.Path(__file__).resolve()
        project = source.parent.parent
        files = [source, source.with_suffix('.sh'),
                 project / '.github/workflows/qualify-hosted-commercial-lifecycle.yml']
        return {'run_id': os.environ.get('GITHUB_RUN_ID'),
                'run_attempt': os.environ.get('GITHUB_RUN_ATTEMPT'),
                'workflow_ref': os.environ.get('GITHUB_WORKFLOW_REF'),
                'github_sha': os.environ.get('GITHUB_SHA'),
                'python': sys.version,
                'git_version': self.run(['/usr/bin/git', '--version'])[1].decode().strip(),
                'brew_version': self.brew('--version').strip(),
                'checkout_commit': self.run(['/usr/bin/git', '-C', str(project), 'rev-parse', 'HEAD'])[1].decode().strip(),
                'checkout_tree': self.run(['/usr/bin/git', '-C', str(project), 'rev-parse', 'HEAD^{tree}'])[1].decode().strip(),
                'qualification_files': {str(p.relative_to(project)): digest(p.read_bytes()) for p in files}}

    def brew(self, *args):
        return self.run(['/opt/homebrew/bin/brew', *args])[1].decode()

    def inventory(self):
        return self.brew('list', '--cask').splitlines()

    def taps(self):
        return self.brew('tap').splitlines()

    def namespace(self, uid, allow_owned=False):
        for path in [os.path.expanduser('~/Library/LaunchAgents/' + LABEL + '.plist'),
                     os.path.expanduser('~/Library/LaunchDaemons/' + LABEL + '.plist'),
                     '/Library/LaunchAgents/' + LABEL + '.plist',
                     '/Library/LaunchDaemons/' + LABEL + '.plist']:
            if self.exists(path):
                raise Refused('matching launch plist exists: ' + path)
        for domain in ['system', 'user/' + str(uid), 'gui/' + str(uid)]:
            code, out, err = self.run(['/bin/launchctl', 'print', domain + '/' + LABEL],
                                      allowed=(0, 113, 125))
            message = (out + err).decode(errors='replace')
            if code == 0:
                raise Refused('matching launch service exists: ' + domain)
            service_absent = code == 113 and 'Could not find service "' + LABEL + '"' in message
            domain_absent = (code == 125 and domain.startswith('gui/') and
                             'Could not find domain for' in message)
            if not (service_absent or domain_absent):
                raise Refused('launch absence unproven: ' + domain + ': ' + message)
        apps = self.run(['/usr/bin/lsappinfo', 'list'])[1].decode(errors='replace')
        if LABEL in apps:
            raise Refused('matching running application exists')

    def tap_state(self):
        head = self.run(['/usr/bin/git', '-C', TAP, 'rev-parse', 'HEAD'])[1].decode().strip()
        status = self.run(['/usr/bin/git', '-C', TAP, 'status', '--porcelain'])[1].decode()
        return {'head': head, 'status': status}

    def read_cask(self):
        p = pathlib.Path(CASK)
        if p.is_symlink() or not p.is_file():
            raise Refused('cask must be a regular non-symlink file')
        return p.read_bytes()

    def backup(self, data):
        p = self.root / 'original-cask.rb'
        fd = os.open(p, os.O_WRONLY | os.O_CREAT | os.O_EXCL | os.O_NOFOLLOW, 0o600)
        with os.fdopen(fd, 'wb') as f:
            f.write(data)
            f.flush()
            os.fsync(f.fileno())
        if p.read_bytes() != data or self.read_cask() != data:
            raise Refused('backup or source continuity failed')

    def restore(self, expected):
        data = (self.root / 'original-cask.rb').read_bytes()
        if digest(data) != expected:
            raise Refused('backup bytes no longer valid; never copy damaged backup')
        self.write_cask(data)
        if digest(self.read_cask()) != expected:
            raise Refused('restored cask digest mismatch')

    def write_cask(self, data):
        if pathlib.Path(CASK).is_symlink():
            raise Refused('cask leaf substituted')
        with open(CASK, 'wb') as f:
            f.write(data)
            f.flush()
            os.fsync(f.fileno())

    def fetch_casks(self):
        url = self.run(['/usr/bin/git', '-C', TAP, 'remote', 'get-url', 'origin'])[1].decode().strip()
        if url.removesuffix('.git').lower() != 'https://github.com/sirsimaster/homebrew-tools':
            raise Refused('unexpected tap origin')
        self.run(['/usr/bin/git', '-C', TAP, 'fetch', '--no-write-fetch-head', '--no-tags',
                  'origin', PINS['140']['tap'], PINS['141']['tap']])
        found = {}
        for version, pin in PINS.items():
            data = self.run(['/usr/bin/git', '-C', TAP, 'show', pin['tap'] + ':Casks/sirsi-pantheon.rb'])[1]
            blob = hashlib.sha1(('blob %d\0' % len(data)).encode() + data).hexdigest()
            if digest(data) != pin['cask'] or blob != pin['blob']:
                raise Refused('immutable cask hash mismatch')
            found[version] = data
        return found

    def verify(self, version, stage):
        pin = PINS[version]
        bundle_cli = APP + '/Contents/MacOS/sirsi'
        if not pathlib.Path(CLI).is_symlink() or os.path.realpath(CLI) != bundle_cli:
            raise Refused('installed caller link points elsewhere')
        if shutil.which('sirsi', path=self.env.get('PATH')) != CLI:
            raise Refused('PATH chooses a different installed caller')
        data = pathlib.Path(bundle_cli).read_bytes()
        actual_hash = digest(data)
        if actual_hash != pin['cli'] or pathlib.Path(CLI).read_bytes() != data:
            raise Refused('installed caller bytes stale or substituted; not executed')
        self.run(['/usr/bin/codesign', '--verify', '--deep', '--strict', '-R',
                  '=anchor apple generic and certificate leaf[subject.OU] = "9D382WV988"', APP])
        self.run(['/usr/sbin/spctl', '--assess', '--type', 'execute', '--verbose=2', APP])
        bundle_version = self.run(['/usr/bin/plutil', '-extract', 'CFBundleShortVersionString',
                                   'raw', APP + '/Contents/Info.plist'])[1].decode().strip()
        if bundle_version != '0.24.' + version:
            raise Refused('app version mismatch before caller execution')
        # This is the real installed entry point, after exact-byte/trust checks.
        caller = json.loads(self.run([CLI, 'version', '--json'])[1])
        if (caller.get('version') != 'v0.24.' + version or caller.get('dirty') is not False
                or caller.get('commit') != pin['commit']):
            raise Refused('installed caller version/source does not match exact release')
        record = {'stage': stage, 'cli_sha256': actual_hash, 'caller': caller,
                  'resolved_link': os.path.realpath(CLI)}
        self.emit(stage + '.json', record)
        return actual_hash

    def emit(self, name, record):
        with open(self.root / name, 'x', encoding='utf8') as f:
            json.dump(record, f, indent=2)
            f.flush()
            os.fsync(f.fileno())


class Lifecycle:
    def __init__(self, ops):
        self.o = ops
        self.account = None
        self.admitted = False
        self.tap_intent = False
        self.install_intent = False
        self.backup_ready = False
        self.cask_mutated = False
        self.original_hash = None
        self.original_tap = None
        self.initial_taps = []
        self.results = []

    def preflight(self):
        a = self.o.account()
        self.account = a
        if not (a['name'] == 'runner' and a['uid'] == a['euid'] and a['uid'] > 0
                and a['home'] == a['env_home'] == '/Users/runner'
                and a['platform'] == 'Darwin arm64' and a['version'].split('.')[0] == '15'
                and a['hosted'] == 'github-hosted' and a['actions'] == 'true'
                and a['runner_os'] == 'macOS' and a['arch'] == 'ARM64'):
            raise Refused('actual account/platform is not the supported disposable hosted runner')
        if self.o.brew('--prefix').strip() != '/opt/homebrew':
            raise Refused('unsupported Homebrew prefix')
        for path in [APP, CLI]:
            if self.o.exists(path):
                raise Refused('preexisting path, including dangling symlink: ' + path)
        if self.o.caller_path() is not None:
            raise Refused('preexisting sirsi selected elsewhere on PATH')
        # Successful full inventory is mandatory; a probe error is never absence.
        if 'sirsi-pantheon' in self.o.inventory():
            raise Refused('preexisting target cask registration')
        self.o.namespace(a['uid'])
        self.initial_taps = self.o.taps()
        if 'sirsimaster/tools' not in self.initial_taps and self.o.exists(TAP):
            raise Refused('preexisting partial/unlisted tap')
        self.admitted = True

    def body(self):
        self.preflight()
        if 'sirsimaster/tools' not in self.initial_taps:
            self.tap_intent = True  # arm before partial creation can fail
            self.o.brew('tap', 'sirsimaster/tools')
        self.original_tap = self.o.tap_state()
        if self.original_tap['status']:
            raise Refused('original tap is dirty; never overwrite it')
        original = self.o.read_cask()
        self.original_hash = digest(original)
        self.o.backup(original)
        self.backup_ready = True
        casks = self.o.fetch_casks()
        first_hash = None
        for version, verb, stage in [('140', 'install', 'install140'),
                                      ('141', 'upgrade', 'upgrade141'),
                                      ('140', 'reinstall', 'rollback140')]:
            self.cask_mutated = True  # arm before a partial replacement
            self.o.write_cask(casks[version])
            self.install_intent = True
            self.o.brew(verb, '--cask', TOKEN)
            value = self.o.verify(version, stage)
            if first_hash is None:
                first_hash = value
            elif stage == 'rollback140' and value != first_hash:
                raise Refused('rollback caller differs from first verified installation')
            self.o.namespace(self.account['uid'], allow_owned=True)
            self.results.append({'stage': stage, 'cli_sha256': value})

    def cleanup(self):
        debt = []
        self.o.recovery()
        def ensure_quiescent():
            if (self.o.unjoined or getattr(self.o, 'active_child', None) is not None
                    or getattr(self.o, 'spawn_pending', False)):
                raise RecoveryStopped(debt + ['owned operation active/unknown; recovery stopped: '
                                             + repr(self.o.unjoined)])
        ensure_quiescent()
        if not self.admitted:
            return debt  # no mutating operation was admitted
        def attempt(label, action):
            ensure_quiescent()
            try:
                result = action()
            except Exception as error:
                debt.append(label + ': ' + repr(error))
                result = None
            # Sticky barrier, including errors raised by cleanup itself.
            ensure_quiescent()
            return result
        if self.install_intent:
            items = attempt('pre-uninstall inventory', self.o.inventory)
            if items is not None and 'sirsi-pantheon' in items:
                attempt('supported uninstall', lambda: self.o.brew('uninstall', '--cask', TOKEN))
            final = attempt('final cask inventory', self.o.inventory)
            if final is None or 'sirsi-pantheon' in final:
                debt.append('target cask absence unproven')
            for path in [APP, CLI]:
                if self.o.exists(path):
                    debt.append('retained owned installation path: ' + path)
            attempt('final launch/app namespace', lambda: self.o.namespace(self.account['uid']))
        if self.cask_mutated:
            if self.backup_ready:
                attempt('restore validated original cask', lambda: self.o.restore(self.original_hash))
            else:
                debt.append('mutation without validated backup; original not overwritten by cleanup')
        if self.original_tap is not None:
            final = attempt('final tap state', self.o.tap_state)
            if final != self.original_tap:
                debt.append('original tap HEAD/worktree not restored')
        if self.tap_intent:
            taps = attempt('post-tap inventory', self.o.taps)
            if taps is not None and 'sirsimaster/tools' in taps:
                attempt('untap exact introduced tap', lambda: self.o.brew('untap', 'sirsimaster/tools'))
            taps = attempt('final tap inventory', self.o.taps)
            if taps is None or 'sirsimaster/tools' in taps or self.o.exists(TAP):
                debt.append('introduced tap/partial tap retained or absence unproven')
        return debt

    def execute(self):
        original = None
        provenance = None
        try:
            provenance = self.o.provenance()
            self.body()
        except Exception as error:
            original = repr(error)
        try:
            debt = self.cleanup()
        except RecoveryStopped as error:
            debt = error.debt
        except Exception as error:
            debt = ['cleanup controller failure: ' + repr(error)]
        receipt = {'status': 'passed' if original is None and not debt else 'failed_or_cleanup_debt',
                   'original_failure': original, 'cleanup_debt': debt,
                   'account': self.account, 'transitions': self.results,
                   'provenance': provenance,
                   'original_tap': self.original_tap, 'calls': self.o.calls,
                   'retained_source_metadata': 'Exact fetched objects may remain in a preexisting tap. '
                                               'Fetch does not write FETCH_HEAD; original HEAD/worktree is compared.',
                   'limits': 'Retained evidence and fetched Git objects are not byte-for-byte disk cleanup. '
                             'Forced host teardown/missing terminal means UNKNOWN, never PASS. '
                             'No loaded GUI or entire app inventory equality claim.'}
        self.o.emit('terminal.json', receipt)
        return receipt


def main():
    if len(sys.argv) != 1:
        raise Refused('No local override or alternate executable/path arguments supported')
    ops = Ops()
    def interrupted(signum, _frame):
        # Flag-only across Popen/ownership adoption; run() observes and joins.
        ops.cancel_signal = signum
        ops.cancel_requested = True
    signal.signal(signal.SIGINT, interrupted)
    signal.signal(signal.SIGTERM, interrupted)
    receipt = Lifecycle(ops).execute()
    print(json.dumps(receipt, indent=2), flush=True)
    return 0 if receipt['status'] == 'passed' else 1


if __name__ == '__main__':
    sys.exit(main())
