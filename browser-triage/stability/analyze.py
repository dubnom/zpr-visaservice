#!/usr/bin/env python3
"""Validate exact-ID coverage and summarize the retained Playwright JSON evidence."""
import collections
import hashlib
import json
from pathlib import Path
import re

folder = Path(__file__).resolve().parent
targets = json.loads((folder / 'targets.json').read_text())
by_identity = {(t['file'], t['title'], t['project']): t['id'] for t in targets}
summary = {t['id']: dict(t, runs={}) for t in targets}
reports = []
ansi = re.compile(r'\x1b\[[0-9;]*m')

def walk(suites):
    for suite in suites:
        yield from suite.get('specs', [])
        yield from walk(suite.get('suites', []))

for run in json.loads((folder / 'runs.json').read_text()):
    path = folder / (run['name'] + '.json')
    report = json.loads(path.read_text())
    assert not report.get('errors'), report.get('errors')
    counts = collections.defaultdict(collections.Counter)
    errors = collections.defaultdict(collections.Counter)
    indices = collections.defaultdict(set)
    durations = collections.defaultdict(list)
    for spec in walk(report['suites']):
        for test in spec['tests']:
            identity = (spec['file'], spec['title'], test['projectName'])
            assert identity in by_identity, ('unexpected test identity', identity)
            ident = by_identity[identity]
            assert len(test['results']) == 1, ('unexpected retry', test)
            result = test['results'][0]
            counts[ident][result['status']] += 1
            indices[ident].add(spec['id'])
            durations[ident].append(result['duration'])
            for error in result.get('errors', []):
                message = ansi.sub('', error.get('message', ''))
                # Keep assertion details but not absolute invocation paths/code frames.
                message = message.split('\n  ')[0].split('\nCall log:')[0]
                errors[ident][message] += 1
    selected = {t['id'] for t in targets if t['project'] == run['project']}
    assert set(counts) == selected, (run['name'], set(counts), selected)
    for ident in selected:
        assert sum(counts[ident].values()) == 10, (run['name'], ident, counts[ident])
        # Playwright's JSON reporter emits a different ID for each repeat, without
        # repeatEachIndex. Require the original exact ID and 10 distinct IDs for
        # the same unchanged file/title/project identity.
        assert ident in indices[ident] and len(indices[ident]) == 10, (run['name'], ident, indices[ident])
        assert set(counts[ident]) <= {'passed', 'failed', 'timedOut'}, counts[ident]
        summary[ident]['runs'][run['name']] = dict(status=dict(counts[ident]),
            errors=dict(errors[ident]), duration_ms=durations[ident], observed_repeat_ids=sorted(indices[ident]))
    reports.append(dict(name=run['name'], commit=run['commit'], workers=run['workers'],
                        stats=report['stats'], sha256=hashlib.sha256(path.read_bytes()).hexdigest()))
assert len(reports) == 8, len(reports)
assert all(len(t['runs']) == 4 for t in summary.values())
output = dict(total_executions=sum(sum(r['status'].values()) for t in summary.values() for r in t['runs'].values()),
              reports=reports, targets=list(summary.values()))
assert output['total_executions'] == 400
(folder / 'summary.json').write_text(json.dumps(output, indent=2) + '\n')
print('Validated 400 executions: 10 exact IDs × 10 repetitions × 2 commits × 2 worker counts.')
for target in summary.values():
    print(target['id'], target['title'])
    for name, result in target['runs'].items():
        print(' ', name, result['status'], 'errors:', result['errors'])
