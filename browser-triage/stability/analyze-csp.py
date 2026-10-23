#!/usr/bin/env python3
"""Extract native input/CSP audit evidence from observer-only diagnostic runs."""
import collections
import json
from pathlib import Path

folder = Path(__file__).resolve().parent
findings = []
for mode in ('diagnostic', 'input-probe', 'gateway-probe'):
    for label in ('maturing', 'beta'):
        name = label + '-csp-' + mode
        report = json.loads((folder / (name + '.json')).read_text())
        counts = collections.defaultdict(collections.Counter)
        def walk(suites):
            for suite in suites:
                for spec in suite.get('specs', []):
                    for test in spec['tests']:
                        for result in test['results']:
                            counts[spec['title']][result['status']] += 1
                walk(suite.get('suites', []))
        walk(report['suites'])
        assert all(sum(c.values()) == 10 for c in counts.values())
        assert len(counts) == {'diagnostic': 3, 'input-probe': 1, 'gateway-probe': 2}[mode]
        styles = collections.Counter()
        line_styles = collections.Counter()
        inputs = []
        browsers = set()
        violations = 0
        audits = 0
        for line in (folder / (name + '.log')).read_text().splitlines():
            if 'CSP_DIAGNOSTIC ' not in line:
                continue
            try:
                event = json.loads(line.split('CSP_DIAGNOSTIC ', 1)[1])
            except ValueError:
                continue
            if 'browserVersion' in event:
                browsers.add(event['browserVersion'])
            if 'directive' in event:
                violations += 1
            if 'audit' in event:
                audits += 1
                attributes = event.get('node', {}).get('attributes', [])
                if 'style' in attributes:
                    styles[attributes[attributes.index('style') + 1]] += 1
            if 'lineStyleMutation' in event:
                mutation = event['lineStyleMutation']
                for style in {mutation['oldStyle'], mutation['currentStyle']} - {None}:
                    line_styles[style] += 1
            if 'input' in event and event['input']['matchingBrackets']:
                inputs.append(event['input'])
        assert violations > 0 and (styles or line_styles), (name, violations, styles)
        if mode != 'diagnostic':
            assert inputs, ('No native bracket/input evidence', name)
        findings.append(dict(name=name, stats=report['stats'], tests={k: dict(v) for k, v in counts.items()},
                             browser_versions=sorted(browsers), observed_violation_events=violations,
                             observed_cdp_audits=audits, rejected_inline_styles=dict(styles),
                             observed_line_style_mutations=dict(line_styles),
                             representative_native_inputs=inputs[:3]))
(folder / 'csp-findings.json').write_text(json.dumps(findings, indent=2) + '\n')
for finding in findings:
    print(finding['name'], finding['tests'], finding['rejected_inline_styles'])
print('Validated 120 supplementary diagnostic executions; these are separate from the 400-run stability matrix.')
