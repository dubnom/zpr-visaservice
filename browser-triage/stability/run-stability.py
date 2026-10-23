#!/usr/bin/env python3
"""Run the exact 10 IDs on the assigned integration and disposable beta worktrees."""
import argparse
import datetime
import json
import os
from pathlib import Path
import re
import subprocess

parser = argparse.ArgumentParser()
parser.add_argument('--beta-worktree', required=True, type=Path)
parser.add_argument('--maturing-worktree', type=Path, help='Defaults to this evidence worktree; must be pinned to the integration commit.')
args = parser.parse_args()
root = Path(__file__).resolve().parents[2]
evidence = root / 'browser-triage/stability'
targets = json.loads((evidence / 'targets.json').read_text())
manifest = []
for label, worktree, expected in [
    ('maturing', (args.maturing_worktree or root).resolve(), '315c10650b510911ab1b1516b699ab5a72df6adf'),
    ('beta', args.beta_worktree.resolve(), '4627e4cd99f15e1ee227f788574ab5055ee7a353'),
]:
    actual = subprocess.check_output(['git', 'rev-parse', 'HEAD'], cwd=worktree, text=True).strip()
    assert actual == expected, (label, actual)
    runtime = worktree / '.browser-runtime'
    runtime.mkdir(exist_ok=True)
    for workers in (1, 2):
        for project in ('desktop', 'tablet'):
            selected = [t for t in targets if t['project'] == project]
            pattern = '(?:' + '|'.join(re.escape(t['title']) for t in selected) + ')$'
            name = f'{label}-w{workers}-{project}'
            command = ['npm', 'run', 'test:browser', '--', *sorted({t['file'] for t in selected}),
                       '--project=' + project, '--grep=' + pattern, '--repeat-each=10',
                       '--workers=' + str(workers), '--retries=0', '--trace=off',
                       '--reporter=json,list']
            env = dict(os.environ)
            for key in list(env):
                if key == 'CI' or 'SIMULATOR' in key or key.startswith('ZPR_LIVE'):
                    env.pop(key)
            env['TMPDIR'] = str(runtime)
            env['PLAYWRIGHT_JSON_OUTPUT_FILE'] = str(evidence / f'{name}.json')
            start = datetime.datetime.now(datetime.timezone.utc).isoformat()
            print(f'{start} START {name}', flush=True)
            with (evidence / f'{name}.log').open('w') as log:
                log.write('cwd: ' + str(worktree / 'zpr-dashboard') + '\n')
                log.write('command: ' + json.dumps(command) + '\n')
                log.write('CI/live/Simulator opt-ins unset; TMPDIR=' + str(runtime) + '\n')
                log.flush()
                result = subprocess.run(command, cwd=worktree / 'zpr-dashboard', env=env,
                                        stdout=log, stderr=subprocess.STDOUT)
            entry = dict(name=name, commit=actual, workers=workers, project=project,
                         repeats=10, start=start, end=datetime.datetime.now(datetime.timezone.utc).isoformat(),
                         exit_code=result.returncode, command=command)
            manifest.append(entry)
            (evidence / 'runs.json').write_text(json.dumps(manifest, indent=2) + '\n')
            report = json.loads((evidence / f'{name}.json').read_text())
            print(f"DONE {name} exit={result.returncode} stats={report['stats']}", flush=True)
