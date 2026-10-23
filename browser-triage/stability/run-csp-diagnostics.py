#!/usr/bin/env python3
"""Run additional unchanged CSP cases with event/CDP observations, after stability runs."""
import json
import os
from pathlib import Path
import re
import subprocess
import sys

root = Path(__file__).resolve().parents[2]
folder = root / 'browser-triage/stability'
targets = [t for t in json.loads((folder / 'targets.json').read_text())
           if 'Gateway analysis scope' in t['title'] or 'GUI raw scenario analysis' in t['title']]
mode = sys.argv[2] if len(sys.argv) > 2 else 'diagnostic'
assert mode in ('diagnostic', 'input-probe', 'gateway-probe')
if mode == 'input-probe':
    targets = [t for t in targets if 'GUI raw scenario' in t['title']]
if mode == 'gateway-probe':
    targets = [t for t in targets if 'Gateway analysis scope' in t['title']]
manifest = []
for label, worktree in [('maturing', root), ('beta', Path(sys.argv[1]).resolve())]:
    name = label + '-csp-' + mode
    command = ['npm', 'run', 'test:browser', '--', 'dashboard.spec.mjs', '--project=tablet',
               '--grep=(?:' + '|'.join(re.escape(t['title']) for t in targets) + ')$',
               '--repeat-each=10', '--workers=1', '--retries=0', '--reporter=json,list']
    if mode != 'diagnostic':
        command.append('--trace=off')
    env = dict(os.environ)
    for key in list(env):
        if key == 'CI' or 'SIMULATOR' in key or key.startswith('ZPR_LIVE'):
            env.pop(key)
    env['TMPDIR'] = str(worktree / '.browser-runtime')
    if mode != 'diagnostic':
        env['CSP_OBSERVE_INPUT'] = '1'
    env['NODE_OPTIONS'] = '--import=' + (folder / 'csp-observer.mjs').as_uri()
    env['PLAYWRIGHT_JSON_OUTPUT_FILE'] = str(folder / (name + '.json'))
    print('START', name, flush=True)
    with (folder / (name + '.log')).open('w') as log:
        log.write('command: ' + json.dumps(command) + '\n')
        log.write('NODE_OPTIONS=' + env['NODE_OPTIONS'] + '\n')
        log.flush()
        result = subprocess.run(command, cwd=worktree / 'zpr-dashboard', env=env,
                                stdout=log, stderr=subprocess.STDOUT)
    manifest.append(dict(name=name, command=command, exit_code=result.returncode,
                         stats=json.loads((folder / (name + '.json')).read_text())['stats']))
    (folder / (mode + '-runs.json')).write_text(json.dumps(manifest, indent=2) + '\n')
    print('DONE', name, manifest[-1]['stats'], flush=True)
