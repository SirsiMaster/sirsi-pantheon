"""Actual shell route/lifecycle fixtures use ONLY owned inert subprocesses.

Selector/supervisor stubs do not run metadata, SSH, canonical CLI or payload.
Native proof itself is separately covered by fabric-locality.test.py.
"""
import json
import os
from pathlib import Path
import shutil
import subprocess
import tempfile
import unittest

SCRIPT=Path(__file__).with_name('fabric-run.sh')


class SameHostDispatch(unittest.TestCase):
    def invoke(self, decision=0, code=0, transport=None, proxy=None, agent='own-lane', router='own-lane', plan=False, missing=False):
        with tempfile.TemporaryDirectory(prefix='fabric-self-owned-') as name:
            root=Path(name);root.chmod(0o700)
            home=root/'home';home.mkdir(mode=0o700);(home/'.local').mkdir(mode=0o700);(home/'.local/bin').mkdir(mode=0o700)
            script=root/'fabric-run.sh';shutil.copyfile(SCRIPT,script);script.chmod(0o700)
            selector=root/'fabric-endpoints.py'
            selector.write_text('import json,os,sys\nprint(json.dumps({"selector_pid":os.getpid(),"exec_parent_pid":os.getppid()}),file=sys.stderr)\nsys.exit('+str(decision)+')\n');selector.chmod(0o700)
            log=root/'supervisor.jsonl';supervisor=home/'.local/bin/sirsi-maat-run'
            if not missing:
                supervisor.write_text('#!/usr/bin/python3\nimport json,os,sys\nfd=os.open('+repr(str(log))+',os.O_WRONLY|os.O_CREAT|os.O_APPEND,0o600)\nwith os.fdopen(fd,"a") as f: f.write(json.dumps({"pid":os.getpid(),"argv":sys.argv[1:],"proxy":os.environ.get("ALL_PROXY")})+"\\n")\nsys.exit('+str(code)+')\n');supervisor.chmod(0o700)
            env={k:v for k,v in os.environ.items() if not k.startswith('SIRSI_') and k not in ('ALL_PROXY','all_proxy','HTTP_PROXY','http_proxy','HTTPS_PROXY','https_proxy','BASH_ENV','ENV','PYTHONPATH','PYTHONHOME')}
            env.update(HOME=str(home),SIRSI_AGENT_ID=agent,SIRSI_ROUTER_AGENT=router)
            if proxy:env['ALL_PROXY']=proxy
            payload=['/usr/bin/printf','%s',"a space'; $(false) ; 'literal",'','--work=not-an-option']
            argv=['/bin/bash',str(script)]
            if plan:argv+=['--plan']
            if transport:argv+=['--transport',transport]
            argv+=['m5','--work','same-host-own','--cpu-cores','2','--timeout','37','--',*payload]
            result=subprocess.run(argv,env=env,text=True,capture_output=True,timeout=5,check=False)
            calls=[json.loads(line) for line in log.read_text().splitlines()] if log.exists() else []
            observations=[json.loads(x) for x in result.stderr.splitlines() if x.startswith('{')]
            return result,calls,payload,observations

    def test_same_endpoint_uses_one_normal_executor_exact_args_and_no_stdout_pollution(self):
        result,calls,payload,observed=self.invoke(transport='direct')
        self.assertEqual(result.returncode,0,result.stderr);self.assertEqual(result.stdout,'')
        self.assertEqual(len(calls),1)
        self.assertEqual(calls[0]['argv'],['--agent','own-lane','--register-executor','--work','same-host-own','--cpu-cores','2','--timeout','37','--',*payload])
        self.assertNotIn('/usr/bin/ssh',str(calls))
        self.assertEqual(observed[0]['exec_parent_pid'],calls[0]['pid'],'selector parent must become actual supervisor by exec')

    def test_local_failure_is_terminal_once_and_does_not_clear_payload_proxy(self):
        for proxy in (None,'socks5://127.0.0.1:19001'):
            with self.subTest(proxy=proxy):
                result,calls,_,_=self.invoke(code=78,proxy=proxy)
                self.assertEqual(result.returncode,78);self.assertEqual(len(calls),1)
                self.assertNotIn('/usr/bin/ssh',str(calls));self.assertEqual(calls[0]['proxy'],proxy)

    def test_local_explicit_direct_is_still_nonnetwork_and_preserves_mask(self):
        result,calls,_,_=self.invoke(transport='direct',proxy='socks5://127.0.0.1:19001')
        self.assertEqual(result.returncode,0,result.stderr);self.assertEqual(calls[0]['proxy'],'socks5://127.0.0.1:19001')
        self.assertNotIn('/usr/bin/ssh',str(calls))

    def test_remote_keeps_strict_origin_destination_supervision_no_local_retry(self):
        result,calls,_,_=self.invoke(decision=3,code=255,transport='direct')
        self.assertEqual(result.returncode,255);self.assertEqual(len(calls),1)
        args=calls[0]['argv']
        self.assertEqual(args[:10],['--agent','own-lane','--register-executor','--work','fabric-ssh-same-host-own','--cpu-cores','1','--timeout','157','--'])
        self.assertEqual(args[10],'/usr/bin/ssh')
        for flag in ('-oStrictHostKeyChecking=yes','-oHostKeyAlias=sirsi-m5','-oPreferredAuthentications=publickey','-oProxyJump=none'):
            self.assertIn(flag,args)
        self.assertIn("'--cpu-cores' '2' '--timeout' '37'",args[-1])

    def test_remote_unknown_requires_proxy_and_never_bypasses_present_proxy(self):
        for options in ({'decision':3},{'decision':3,'transport':'direct','proxy':'socks5://127.0.0.1:19001'}):
            with self.subTest(options=options):
                result,calls,_,_=self.invoke(**options)
                self.assertEqual(result.returncode,78);self.assertEqual(calls,[])

    def test_selector_refusal_missing_native_and_scope_conflict_never_fall_back(self):
        for options in ({'decision':78,'transport':'direct'},{'decision':0,'missing':True,'transport':'direct'},
                        {'router':'foreign-lane','transport':'direct'}):
            with self.subTest(options=options):
                result,calls,_,_=self.invoke(**options)
                self.assertEqual(result.returncode,78);self.assertEqual(calls,[])

    def test_plan_never_observes_metadata_or_invokes_executor(self):
        result,calls,_,observed=self.invoke(decision=78,transport='direct',plan=True)
        self.assertEqual(result.returncode,0,result.stderr);self.assertEqual(calls,[]);self.assertEqual(observed,[])
        self.assertIn('SELF_DISPATCH=conditional',result.stdout);self.assertIn('LOCAL_COMMAND=',result.stdout);self.assertIn('REMOTE_COMMAND=',result.stdout)

    def test_metadata_cleanup_fatal_status_starts_no_supervisor_or_ssh(self):
        # The production selector's killed/unreaped boundary is separately
        # injected by LocalityPolicy; this invokes the actual shell on its78.
        result,calls,_,observed=self.invoke(decision=78,transport='direct')
        self.assertEqual(result.returncode,78);self.assertEqual(calls,[])
        self.assertEqual(result.stdout,'');self.assertEqual(len(observed),1)


if __name__=='__main__':unittest.main()
