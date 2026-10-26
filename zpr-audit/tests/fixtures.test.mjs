import { readFile } from 'node:fs/promises';
import vm from 'node:vm';
import assert from 'node:assert/strict';
import { test } from 'node:test';

const source = await readFile(new URL('../public/fixtures.js', import.meta.url), 'utf8');
function evaluate(expression) {
  return vm.runInNewContext(source + '\n' + expression);
}

test('forward and reverse sample requests agree in both revisions', () => {
  assert.equal(evaluate(`services.every(service =>
    [false,true].every(old => serviceRequestRows(service,old).every(row => {
      const forward=identityServiceRows(row[0],'Selected sample request',old).find(r=>r[0]===service);
      return forward[2]===row[2] && forward[3]===row[3];
    })))`), true);
});

test('service and group reverse candidates agree without inferring group-wide grants', () => {
  assert.equal(evaluate(`services.every(service=>[false,true].every(old=>
    serviceCandidateRows(service,old).filter(r=>r[1].startsWith('group:')).every(row=>{
      const reverse=groupServiceRows(row[0],old).find(r=>r[0]===service);
      return row[2]===reverse[2]&&row[3]===reverse[3]&&row[2]!=='permitted';
    })))`),true);
  assert.equal(evaluate(`Object.keys(memberships).every(name=>accessSubjects.includes(name))`),true);
});

test('all request identities have membership provenance and outcomes', () => {
  assert.equal(evaluate(`services.every(service => serviceRequestRows(service).every(row =>
    memberships[row[0]] && labels[row[2]]))`), true);
});

test('revision comparisons cannot mutate the base fixture', () => {
  assert.equal(evaluate(`serviceRequestRows('Finance Reports',true);
    serviceRequestRows('Finance Reports').find(r=>r[0]==='Bob Evans')[2]`), 'denied');
  assert.equal(evaluate(`const rows=serviceRequestRows('Finance Reports'); rows[0][2]='denied';
    serviceRequestRows('Finance Reports')[0][2]`), 'permitted');
});

test('Priya policy-impact example agrees with access exploration for both revisions', () => {
  assert.equal(evaluate(`serviceRequestRows('Finance Reports',true).find(r=>r[0]==='Priya Shah')[2]`), 'permitted');
  assert.equal(evaluate(`serviceRequestRows('Finance Reports').find(r=>r[0]==='Priya Shah')[2]`), 'conditional');
});

test('unknown and missing contexts never acquire permission', () => {
  assert.equal(evaluate(`identityServiceRows('Jordan Lee').every(r=>r[2]==='unknown')`), true);
  assert.equal(evaluate(`identityServiceRows('Alice Chen','Trusted source unavailable').every(r=>r[2]==='unknown')`), true);
  assert.equal(evaluate(`identityServiceRows('Alice Chen','Device not selected').every(r=>r[2]==='conditional')`), true);
  assert.throws(() => evaluate(`serviceRequestRows('nonexistent')`), /Unknown sample service/);
});

test('assertion catalog retains unique revision references, roles and provenance', () => {
  assert.equal(evaluate(`new Set(assertionFixtures.map(a=>a.id)).size===assertionFixtures.length`), true);
  assert.equal(evaluate(`new Set(assertionFixtures.map(a=>a.name)).size===assertionFixtures.length`), true);
  assert.equal(evaluate(`assertionFixtures.every(a=>
    /^assertion:[a-z-]+@a[0-9]+$/.test(a.id) &&
    ['enforcing','diagnostic'].includes(a.roleKey) && services.includes(a.service) &&
    [a.name,a.role,a.definition,a.policy,a.outcomes,a.source,a.limit].every(value=>value.length>0))`), true);
  assert.equal(evaluate(`matchingAssertions('','diagnostic')[0].role`), 'Diagnostic only · sample intent');
  assert.equal(evaluate(`matchingAssertions('','diagnostic')[0].limit.includes('must not be presented as an enforcing denial')`), true);
});

test('assertion filters combine role and case-insensitive evidence search without changing fixtures', () => {
  assert.equal(evaluate(`matchingAssertions('  ASSERTION:MANAGED-DEVICE@A2  ')[0].name`), 'Managed device');
  assert.equal(evaluate(`matchingAssertions('Device registry','enforcing').length`), 1);
  assert.equal(evaluate(`matchingAssertions('directory','diagnostic').length`), 1);
  assert.equal(evaluate(`matchingAssertions('Device registry','diagnostic').length`), 0);
  assert.equal(evaluate(`matchingAssertions('no such assertion').length`), 0);
  assert.equal(evaluate(`matchingAssertions().length`), 3);
  assert.equal(evaluate(`const before=JSON.stringify(assertionFixtures);
    matchingAssertions('Finance','enforcing');before===JSON.stringify(assertionFixtures)`), true);
  assert.throws(() => evaluate(`matchingAssertions('','invalid')`), /Unknown assertion role/);
});
