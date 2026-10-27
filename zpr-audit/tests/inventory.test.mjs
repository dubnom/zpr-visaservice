import {readFile} from 'node:fs/promises';
import vm from 'node:vm';
import {test} from 'node:test';
import assert from 'node:assert/strict';
const source=await readFile(new URL('../public/inventory.js',import.meta.url),'utf8');
const parse=vm.runInNewContext(source+';parseAssertionInventory',{TextEncoder});
const validateReview=vm.runInNewContext(source+';validateInventoryReview',{TextEncoder});
const validateProvenance=vm.runInNewContext(source+';validateInventoryProvenance',{TextEncoder});
const reviewLines=vm.runInNewContext(source+';sourceReviewLines',{TextEncoder});
const matchingLines=vm.runInNewContext(source+';matchingSourceLines',{TextEncoder});
const lineNumber=vm.runInNewContext(source+';sourceLineNumber',{TextEncoder});
const compareSnapshots=vm.runInNewContext(source+';compareAssertionSnapshots',{TextEncoder});
const filterComparison=vm.runInNewContext(source+';filterComparisonLines',{TextEncoder});
const record={organization_id:'org-1',organization_name:'Actual organization',settings:{revision:3,source:'assert true;',enabled:false,interval_seconds:60}};
test('import preserves explicit organization and settings without inventing outcomes',()=>{
 const result=parse(JSON.stringify({...record,base_dn:'not retained',secret:'not retained'}));
 assert.equal(result.organization_name,record.organization_name);
 assert.equal(result.settings.source,record.settings.source);
 assert.equal(result.settings.enabled,false);
 assert.equal(result.secret,undefined);
 assert.equal(result.outcomes,undefined);
});
test('malformed or incomplete imports are rejected rather than defaulted',()=>{
 for(const value of [null,[],{}, {...record,organization_name:''},{...record,settings:{...record.settings,enabled:'false'}},{...record,settings:{...record.settings,revision:-1}},{...record,settings:{...record.settings,interval_seconds:1.5}}]){
  assert.throws(()=>parse(JSON.stringify(value)));
 }
 assert.throws(()=>parse('not json'));
 assert.throws(()=>parse(JSON.stringify({...record,settings:{...record.settings,source:'é'.repeat(32769)}})),/64 KiB/);
});
test('service errors and failed settings loads cannot become successful evidence',()=>{
 assert.throws(()=>parse(JSON.stringify({error:'Policy Repository assertion settings are unavailable'})),/service-error response/);
 for(const settings_error of ['repository unavailable',null,false,{}]){
  assert.throws(()=>parse(JSON.stringify({...record,settings_error})),/settings load failure/);
 }
 assert.throws(()=>parse(JSON.stringify({...record,scope:'another-organization-scope'})),/organization-scoped/);
});
test('Control Room status envelope imports only settings, not runtime observations',()=>{
 const status={...record,scope:'organization',settings_error:'',configured:true,running:false,
  last_run:{organization_id:'other-org',revision:99,status:'passed'},sources:[{name:'Directory'}],change_watchers:[]};
 const result=parse(JSON.stringify(status));
 assert.equal(result.settings.revision,3);
 assert.equal(result.organization_id,'org-1');
 assert.equal(result.last_run,undefined);
 assert.equal(result.sources,undefined);
 assert.equal(result.settings_error,undefined);
});
test('review requires preparer/purpose and validates real UTC retrieval timestamps',()=>{
 const importedAt='2026-10-26T14:00:00.000Z';
 const review={system:'control-room',reference:'deployment label',preparer:' Reviewer ',purpose:' Review ',notes:'',retrieved:''};
 assert.equal(validateReview(review,importedAt).preparer,'Reviewer');
 assert.equal(validateReview({...review,retrieved:'2026-10-26T13:59:59Z'},importedAt).retrieved,'2026-10-26T13:59:59Z');
 for(const retrieved of ['2026-02-30T12:00:00Z','2026-10-26T13:00:00','2026-10-26T15:00:00Z','not a timestamp']){
  assert.throws(()=>validateReview({...review,retrieved},importedAt));
 }
 for(const patch of [{preparer:' '},{purpose:''},{system:'simulator'},{notes:'x'.repeat(4001)}]){
  assert.throws(()=>validateReview({...review,...patch},importedAt));
 }
});
test('source review preserves blank lines and searches literal text without evaluating it',()=>{
 assert.deepEqual(Array.from(reviewLines('a\r\n\r\nB\n')),['a','','B','']);
 assert.equal(reviewLines('').length,0);
 assert.deepEqual(Array.from(matchingLines('assert [A];\nassert [a];\nother','[a]')),[1,2]);
 assert.deepEqual(Array.from(matchingLines('assert [A];\nassert [a];','[a]',true)),[2]);
 assert.equal(matchingLines('assert true;','').length,0);
 assert.equal(lineNumber('3',3),3);
 for(const value of ['0','-1','1.5','1e2','4','','NaN']){
  assert.throws(()=>lineNumber(value,3));
 }
});
test('snapshot comparison is organization-bound and distinguishes values, lines and revision warnings',()=>{
 const baseline={...record,settings:{...record.settings,revision:2,source:'one\r\ntwo',enabled:true}};
 const current={...record,settings:{...record.settings,source:'one\nthree'}};
 const before=JSON.stringify([baseline,current]),result=compareSnapshots(baseline,current);
 assert.equal(result.sourceChanged,true);
 assert.equal(result.lines.length,1);
 assert.equal(result.lines[0].line,2);
 assert.equal(result.lines[0].baseline,'two');
 assert.equal(result.lines[0].current,'three');
 assert.equal(result.fields.find(field=>field.field==='enabled').changed,true);
 assert.equal(JSON.stringify([baseline,current]),before);
 assert.throws(()=>compareSnapshots({...baseline,organization_id:'other'},current),/Cross-organization/);
 assert.match(compareSnapshots(current,baseline).warning,/newer/);
 assert.match(compareSnapshots({...current,settings:{...current.settings,source:'different'}},current).warning,/Same revision/);
 assert.equal(compareSnapshots(record,record).lines.length,0);
});
test('line-ending changes and missing versus blank lines are not conflated',()=>{
 const withSource=source=>({...record,settings:{...record.settings,source}});
 const endings=compareSnapshots(withSource('a\r\n'),withSource('a\n'));
 assert.equal(endings.sourceChanged,true);
 assert.equal(endings.lines.length,0);
 const missing=compareSnapshots(withSource(''),withSource('\n'));
 assert.equal(missing.lines.length,2);
 assert.equal(missing.lines[0].baseline,null);
 assert.equal(missing.lines[0].current,'');
});
test('positional classification counts changed, added, removed and unchanged including blank lines',()=>{
 const snapshot=source=>({...record,settings:{...record.settings,source}});
 const result=compareSnapshots(snapshot('same\nold\n'),snapshot('same\nnew\n\nextra'));
 assert.equal(JSON.stringify(result.counts),JSON.stringify({changed:1,added:1,removed:0,unchanged:2}));
 assert.equal(result.lines[0].kind,'changed');
 assert.equal(result.lines[1].kind,'added');
 assert.equal(result.lines[1].line,4);
 assert.equal(result.lines[1].baseline,null);
 const reverse=compareSnapshots(snapshot('same\nnew\n\nextra'),snapshot('same\nold\n'));
 assert.equal(reverse.counts.removed,1);
 assert.equal(reverse.lines[1].kind,'removed');
 const empty=compareSnapshots(snapshot(''),snapshot(''));
 assert.equal(Object.values(empty.counts).reduce((a,b)=>a+b,0),0);
 const endings=compareSnapshots(snapshot('a\r\n'),snapshot('a\n'));
 assert.equal(endings.counts.unchanged,2);
 assert.equal(endings.counts.changed,0);
});
test('comparison filters search literally on either side without matching display placeholders',()=>{
 const snapshot=source=>({...record,settings:{...record.settings,source}});
 const lines=compareSnapshots(snapshot('[Alpha]\nbase\nremoved'),snapshot('current\n[alpha]')).lines;
 assert.equal(filterComparison(lines,'[alpha]').length,2);
 assert.equal(filterComparison(lines,'[Alpha]','all',true).length,1);
 assert.equal(filterComparison(lines,'[alpha]','changed',true)[0].line,2);
 assert.equal(filterComparison(lines,'removed','removed')[0].line,3);
 assert.equal(filterComparison(lines,'No line at this position').length,0);
 assert.equal(filterComparison(lines,'','removed').length,1);
 assert.equal(filterComparison(lines,'','added').length,0);
 assert.equal(filterComparison(lines,'.*').length,0);
});
test('maximum-size multiline sources preserve complete positional counts',()=>{
 const snapshot=source=>parse(JSON.stringify({...record,settings:{...record.settings,source}}));
 const result=compareSnapshots(snapshot('a\n'.repeat(32768)),snapshot('b\n'.repeat(32768)));
 assert.equal(result.lines.length,32768);
 assert.equal(result.counts.changed,32768);
 assert.equal(result.counts.unchanged,1);
 assert.equal(result.lines.at(-1).line,32768);
});
test('provenance validates independently against each snapshot import time',()=>{
 const provenance={system:'policy-service',reference:' baseline export ',retrieved:'2026-10-26T14:00:00Z'};
 assert.equal(validateProvenance(provenance,'2026-10-26T14:00:00.000Z').reference,'baseline export');
 assert.throws(()=>validateProvenance(provenance,'2026-10-26T13:59:59.999Z'),/after this snapshot/);
 for(const patch of [{system:'simulator'},{reference:'x'.repeat(1001)},{retrieved:'2026-02-30T12:00:00Z'},{retrieved:'2026-10-26T14:00:00+00:00'}]){
  assert.throws(()=>validateProvenance({...provenance,...patch},'2026-10-26T15:00:00.000Z'));
 }
 assert.equal(validateProvenance({system:'unspecified',reference:'',retrieved:''},'2026-10-26T15:00:00.000Z').retrieved,'');
 assert.equal(validateProvenance({...provenance,reference:'x'.repeat(1000)},'2026-10-26T14:00:00.000Z').reference.length,1000);
});
test('comparison status separates revision order, content equality and organization names',()=>{
 const snapshot=patch=>({...record,settings:{...record.settings,...patch}});
 assert.equal(compareSnapshots(record,record).status,'Same revision and settings');
 assert.equal(compareSnapshots(record,snapshot({revision:4})).status,'Revision-only settings change');
 assert.equal(compareSnapshots(record,snapshot({source:'different'})).status,'Same-revision conflict');
 assert.equal(compareSnapshots(record,snapshot({enabled:true})).status,'Same-revision conflict');
 assert.equal(compareSnapshots(record,snapshot({interval_seconds:0})).contentChanged,true);
 assert.equal(compareSnapshots(record,snapshot({revision:7,source:'different'})).status,'Settings differ across revisions');
 assert.equal(compareSnapshots(record,snapshot({revision:2})).status,'Reversed revision order');
 assert.equal(compareSnapshots(record,snapshot({revision:2})).revisionDelta,-1);
 assert.equal(compareSnapshots(snapshot({revision:0}),snapshot({revision:Number.MAX_SAFE_INTEGER})).revisionDelta,Number.MAX_SAFE_INTEGER);
 const renamed=compareSnapshots(record,{...record,organization_name:'Another name'});
 assert.equal(renamed.nameChanged,true);
 assert.equal(renamed.contentChanged,false);
 assert.equal(renamed.status,'Same revision and settings');
});
test('source measures count UTF-8 bytes and displayed lines without normalizing exact evidence',()=>{
 const snapshot=source=>({...record,settings:{...record.settings,source}});
 const result=compareSnapshots(snapshot('é\r\n'),snapshot('😀\n'));
 assert.equal(result.sourceSize.bytes.baseline,4);
 assert.equal(result.sourceSize.bytes.current,5);
 assert.equal(result.sourceSize.lines.baseline,2);
 assert.equal(result.sourceSize.lines.current,2);
 const empty=compareSnapshots(snapshot(''),snapshot('\n'));
 assert.equal(empty.sourceSize.bytes.baseline,0);
 assert.equal(empty.sourceSize.lines.baseline,0);
 assert.equal(empty.sourceSize.lines.current,2);
 const endings=compareSnapshots(snapshot('a\r\n'),snapshot('a\n'));
 assert.equal(endings.contentChanged,true);
 assert.equal(endings.status,'Same-revision conflict');
 assert.equal(endings.sourceSize.bytes.current-endings.sourceSize.bytes.baseline,-1);
});
