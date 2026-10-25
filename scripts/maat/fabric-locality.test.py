"""Native facts are mocked; no metadata/network/CLI command is run by this suite."""
import importlib.util
import io
import json
from contextlib import redirect_stderr, redirect_stdout
from pathlib import Path
from types import SimpleNamespace
import unittest
from unittest.mock import Mock, MagicMock, patch

spec=importlib.util.spec_from_file_location('fabric_endpoints',Path(__file__).with_name('fabric-endpoints.py'))
policy=importlib.util.module_from_spec(spec);spec.loader.exec_module(policy)
UUID='11111111-2222-4333-8444-555555555555'  # Synthetic fixture, not target enrollment.
AGENT=SimpleNamespace(pw_uid=501,pw_name='thekryptodragon')
ADDRESS=policy.PEERS['m5']['tail']
INVENTORY=('lo0: flags=8049<UP,LOOPBACK,RUNNING,MULTICAST> mtu 16384\n'
           '\tinet 127.0.0.1 netmask 0xff000000\n'
           '\tinet6 ::1 prefixlen 128\n'
           'stf0: flags=0<> mtu 1280\n'
           'utun42: flags=8051<UP,POINTOPOINT,RUNNING,MULTICAST> mtu 1280\n'
           '\tinet '+ADDRESS+' --> '+ADDRESS+' netmask 0xffffffff\n'
           '\tnd6 options=201<PERFORMNUD,DAD>\n')


class LocalityPolicy(unittest.TestCase):
    def classify(self, **changes):
        facts=dict(interfaces=policy.parse_interfaces(INVENTORY),native_uuid=UUID,uid=501,euid=501,account=AGENT)
        facts.update(changes)
        return policy.locality('m5',**facts)

    def test_exact_enrolled_kernel_endpoint_and_account_select_local(self):
        with patch.object(policy,'origin_host',side_effect=AssertionError('hostname cannot prove self')),patch.object(policy,'resolve_lan',side_effect=AssertionError('no DNS')):
            self.assertEqual(self.classify(),('local','kernel-endpoint-and-own-uid','utun42'))
        for target in ('m1','studio'):
            peer=policy.PEERS[target]
            account=SimpleNamespace(pw_uid=702,pw_name=peer['user'])
            self.assertEqual(policy.locality(target,{peer['tail']:('en0',True,False)},UUID,702,702,account)[0],'local')
            self.assertEqual(policy.locality(target,policy.parse_interfaces(INVENTORY),UUID,702,702,account)[0],'ssh')

    def test_no_names_substrings_loopback_down_or_uid_shortcut(self):
        for changes in ({'interfaces':{}},{'interfaces':{'192.0.2.1':('en0',True,False)}},
                        {'interfaces':{ADDRESS:('utun42',False,False)}},
                        {'interfaces':{ADDRESS:('lo0',True,True)}},
                        {'native_uuid':None},{'native_uuid':''},{'native_uuid':'M5'},
                        {'uid':502},{'euid':0},{'uid':True},
                        {'account':None},{'account':SimpleNamespace(pw_uid=502,pw_name=AGENT.pw_name)},
                        {'account':SimpleNamespace(pw_uid=501,pw_name='another-owner')}):
            with self.subTest(changes=changes):
                self.assertEqual(self.classify(**changes)[0],'ssh','unproven locality must retain SSH')

    def test_parser_closed_duplicates_flags_and_complete_ipv4_rows(self):
        bad=[INVENTORY+INVENTORY,
             INVENTORY+'en9: flags=1<UP> mtu 1500\n\tinet '+ADDRESS+' netmask 0xffffffff\n',
             INVENTORY.replace('8051<UP,POINTOPOINT,RUNNING,MULTICAST>','8050<UP,POINTOPOINT,RUNNING,MULTICAST>'),
             INVENTORY.replace('8051<UP,POINTOPOINT,RUNNING,MULTICAST>','8051<UP,UP,POINTOPOINT,RUNNING,MULTICAST>'),
             INVENTORY.replace('8051<UP,POINTOPOINT,RUNNING,MULTICAST>','8051<UP,UNKNOWNFLAG>'),
             INVENTORY.replace('netmask 0xffffffff','netmask 0xff00ff00'),
             INVENTORY.replace('netmask 0xffffffff','netmask 0xffffffff trailing'),
             INVENTORY.replace('inet '+ADDRESS,'inet '+ADDRESS+'.9'),
             INVENTORY.replace('inet '+ADDRESS,'inet_bad '+ADDRESS),
             '\tinet '+ADDRESS+' netmask 0xffffffff\n',
             INVENTORY.replace('utun42:','\tutun42:'),INVENTORY+'broken header\n',
             INVENTORY+'\t'+'x'*2049+'\n', 'x'*262145, '\n'*4097, INVENTORY+'\x00']
        for text in bad:
            with self.subTest(length=len(text)),self.assertRaises(policy.PolicyError):policy.parse_interfaces(text)
        only_property='en0: flags=1<UP> mtu 1500\n\tdescription: inet '+ADDRESS+'\n'
        self.assertEqual(policy.parse_interfaces(only_property),{})

    def test_point_to_point_destination_and_broadcast_are_not_local_assignments(self):
        text='en0: flags=1<UP> mtu 1500\n\tinet 192.0.2.1 --> '+ADDRESS+' netmask 0xffffffff broadcast '+ADDRESS+'\n'
        parsed=policy.parse_interfaces(text)
        self.assertNotIn(ADDRESS,parsed);self.assertEqual(self.classify(interfaces=parsed)[0],'ssh')
        self.assertEqual(policy.parse_interfaces('en0: flags=1<UP> mtu 1500\n\tinet 192.0.2.1 netmask 255.255.255.0 broadcast 192.0.2.255\n')['192.0.2.1'],('en0',True,False))

    def test_native_uuid_is_nonempty_observation_not_target_enrollment(self):
        good='"IOPlatformUUID" = "'+UUID.lower()+'"'
        self.assertEqual(policy.parse_native_uuid(good),UUID.upper())
        for raw in (None,'',good+'\n'+good,'"IOPlatformUUID" = "M5"', 'x'*65537):
            with self.subTest(raw_type=type(raw).__name__):self.assertIsNone(policy.parse_native_uuid(raw))
        self.assertTrue(all('machine_id' not in peer for peer in policy.PEERS.values()))

    def test_metadata_reader_bounds_timeout_size_and_clean_native_environment(self):
        stream=Mock();key=SimpleNamespace(fileobj=stream)
        for content,want in ((b'bounded','bounded'),(b'x'*9,None)):
            process=Mock(stdout=stream);process.poll.return_value=None;process.wait.return_value=0
            ready=MagicMock();ready.get_map.side_effect=[{1:key},{}];ready.select.return_value=[(key,1)]
            with self.subTest(size=len(content)),patch.object(policy.subprocess,'Popen',return_value=process) as launch,patch.object(policy.selectors,'DefaultSelector') as selector,patch.object(policy.os,'read',return_value=content):
                selector.return_value.__enter__.return_value=ready
                self.assertEqual(policy.metadata_output(['/sbin/ifconfig','-a'],8),want)
                self.assertEqual(launch.call_args.args[0],['/sbin/ifconfig','-a'])
                self.assertEqual(set(launch.call_args.kwargs['env']),{'PATH','LC_ALL','LANG'})
                process.stdout.close.assert_called()
        process=Mock(stdout=stream);process.poll.return_value=None;process.wait.return_value=0
        ready=MagicMock();ready.get_map.return_value={1:key}
        with patch.object(policy.subprocess,'Popen',return_value=process),patch.object(policy.selectors,'DefaultSelector') as selector,patch.object(policy.time,'monotonic',side_effect=[0,4]):
            selector.return_value.__enter__.return_value=ready
            self.assertIsNone(policy.metadata_output(['/sbin/ifconfig','-a'],8));process.kill.assert_called_once()

    def test_dispatch_observation_is_closed_stderr_only_and_parent_pid_is_native(self):
        facts={'/sbin/ifconfig':INVENTORY,'/usr/sbin/ioreg':'"IOPlatformUUID" = "'+UUID+'"'}
        with patch.object(policy,'metadata_output',side_effect=lambda argv,limit:facts[argv[0]]) as read,patch.object(policy.os,'getuid',return_value=501),patch.object(policy.os,'geteuid',return_value=501),patch.object(policy.pwd,'getpwnam',return_value=AGENT),patch.object(policy.os,'getpid',return_value=555),patch.object(policy.os,'getppid',return_value=444),patch.object(policy,'runtime_proxy',side_effect=AssertionError('locality not network')),patch.object(policy.os,'execv',side_effect=AssertionError('selector grants no execution')),redirect_stdout(io.StringIO()) as output,redirect_stderr(io.StringIO()) as error:
            self.assertEqual(policy.main(['dispatch','m5']),0)
            self.assertEqual(output.getvalue(),'')
            row=json.loads(error.getvalue());self.assertEqual(row['decision'],'local');self.assertEqual(row['exec_parent_pid'],444);self.assertEqual(row['selector_pid'],555)
            self.assertEqual(set(row),{'schema','decision','reason','target','endpoint','native_machine_id','interface','uid','euid','selector_pid','exec_parent_pid','authority'})
            self.assertEqual(row['native_machine_id'],UUID);self.assertNotIn('agent',row)
            self.assertEqual(read.call_count,2)

    def test_unavailable_inventory_retains_remote_without_network_or_raw_error(self):
        with patch.object(policy,'metadata_output',return_value=None),patch.object(policy,'select_endpoint',side_effect=AssertionError('dispatch never probes')),redirect_stdout(io.StringIO()) as out,redirect_stderr(io.StringIO()) as err:
            self.assertEqual(policy.main(['dispatch','m5']),3);self.assertEqual(out.getvalue(),'')
            self.assertEqual(json.loads(err.getvalue())['reason'],'interface-inventory-unavailable')


if __name__=='__main__':unittest.main()
