"""No real process is signalled by these ownership negative controls."""
import importlib.util
import os
from pathlib import Path
import tempfile
import unittest
from unittest import mock

spec = importlib.util.spec_from_file_location('runner', Path(__file__).with_name('admitted-run.py'))
runner = importlib.util.module_from_spec(spec)
spec.loader.exec_module(runner)


class StopOwnership(unittest.TestCase):
    def test_direct_and_wrapped_stops_refuse(self):
        for argv in (["pkill", "-f", "distill.py trainstream"],
                     ["/usr/bin/killall", "Python"], ["/bin/kill", "8578"],
                     ["env", "A=b", "pkill", "-f", "train"],
                     ["bash", "-lc", 'echo ready; pkill -f "distill.py trainstream"'],
                     ["zsh", "-c", "command /bin/kill -TERM 8578"],
                     ["bash", "-c", 'sh -c "pkill -f train"'],
                     ["nohup", "pkill", "train"]):
            with self.subTest(argv=argv), self.assertRaises(runner.Refused):
                runner.require_job_safe_command(argv)

    def test_reads_models_and_signal_names_are_not_stops(self):
        for argv in (["python", "distill.py", "trainstream"],
                     ["pgrep", "-fl", "train"], ["kill", "-l"],
                     ["echo", "pkill"], ["bash", "-c", 'printf "%s" pkill'],
                     ["env", "A=b", "python", "gen_batch.py"]):
            with self.subTest(argv=argv):
                runner.require_job_safe_command(argv)

    def test_no_registration_reservation_or_spawn_on_refused_stop(self):
        backend = mock.Mock()
        record = {}
        with mock.patch.object(runner.subprocess, 'Popen') as spawn:
            code = runner.supervise(['pkill', '-f', 'train'], 'm5', 'lane',
                                    'thread', 'stop-own-training', 30, backend, record)
        self.assertNotEqual(code, 0)
        backend.registered.assert_not_called()
        backend.reserve.assert_not_called()
        spawn.assert_not_called()

    def test_pretool_front_door_does_not_launder_kill_authority(self):
        for command in (
            '~/.local/bin/sirsi-maat-run --work stop-own-training -- pkill -f train',
            '/Users/a/.local/bin/sirsi-maat-run --work x -- zsh -c \'pkill -f "distill.py trainstream"\'',
            'pkill -f train; echo finished',
        ):
            with self.subTest(command=command), self.assertRaises(runner.Refused):
                runner.check_tool_stop({'tool_input': {'command': command}})

    def test_pretool_registration_and_coordination_stay_available(self):
        for command in ('sirsi thread register --agent lane --json',
                        'sirsi router close item --ack --result handled',
                        'sirsi router task update lane task --status pending',
                        'sirsi router send --from lane --to ra --instructions hi'):
            runner.check_tool_stop({'tool_input': {'command': command}})

    def test_main_refuses_before_executor_registration(self):
        with tempfile.TemporaryDirectory() as folder, \
                mock.patch.dict(os.environ, {'HOME': folder}), \
                mock.patch.object(runner, 'local_resource', return_value='m5'), \
                mock.patch.object(runner, 'Native') as native, \
                mock.patch.object(runner.sys, 'argv', ['runner', '--agent', 'lane',
                     '--register-executor', '--work', 'stop-own-training',
                     '--', 'pkill', '-f', 'train']):
            self.assertEqual(runner.main(), 78)
            native.return_value.register_executor.assert_not_called()
            native.return_value.reserve.assert_not_called()

    def test_service_stop_is_not_laundered_by_compute_admission(self):
        for argv in (['launchctl', 'bootout', 'gui/501/foreign'],
                     ['launchctl', 'kill', 'TERM', 'gui/501/foreign'],
                     ['launchctl', 'kickstart', '-k', 'gui/501/foreign'],
                     ['brew', 'services', 'stop', 'foreign'],
                     ['systemctl', '--user', 'restart', 'foreign']):
            with self.subTest(argv=argv), self.assertRaises(runner.Refused):
                runner.require_job_safe_command(argv)

    def test_service_observation_is_not_a_stop(self):
        for argv in (['launchctl', 'print', 'gui/501/foreign'],
                     ['systemctl', '--user', 'status', 'foreign'],
                     ['brew', 'services', 'list']):
            runner.require_job_safe_command(argv)


if __name__ == '__main__':
    unittest.main()
