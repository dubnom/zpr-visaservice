
'use strict';
function groupLinks(name){const m=memberships[name];if(!m||m.unavailable||!m.groups.length)return escapeHTML(membershipText(name));return m.groups.map(g=>'<button class="link" data-group="'+escapeHTML(g[0])+'">'+escapeHTML(g[0])+'</button> <small>'+escapeHTML(g[1])+'</small>').join('<br>')}
function showGroup(name){
 const members=Object.entries(memberships).filter(([,m])=>!m.unavailable&&m.groups.some(g=>g[0]===name));
 const candidates=name==='Finance'?[['Finance Reports','Group-related candidate','Finance membership is one requirement. Managed device, authenticated session and current source evidence still apply.'],['Payroll API','Not granted by this group','Separate payroll service-principal authorization required.']]:name==='Engineering'?[['Engineering Git','Group-related candidate','Engineering membership matches the sample rule; workload/device context and current policy still apply.'],['Finance Reports','Not granted by this group','Finance membership and managed-device evidence required.']]:name==='Employees'?[['Finance Reports','Revision-dependent candidate','Employee membership matched r41; r42 requires Finance membership and a managed device.'],['Engineering Git','Additional membership required','Engineering membership and the selected request context must be evaluated.']]:[['Finance Reports','Device requirement only','Managed-device membership satisfies only the machine condition; an authorized Finance user/session is still required.'],['Engineering Git','Context required','A user or workload principal and applicable policy are still required.'],['Payroll API','Separate principal required','A managed machine alone does not establish payroll service-principal authorization.']];
 const userGroup=name!=='Managed devices';
 dialog('Group · '+name,'<p><span class="pill">Northstar Labs · '+(userGroup?'Company directory · user group':'Device registry · machine group')+'</span></p><p class="muted">Fictional membership snapshot · 26 Oct 2026, 10:30. Direct and source-reported nested members are included.</p><h3>'+ (userGroup?'Users in this group':'Machines in this group')+' <span class="count">'+members.length+' known sample members</span></h3><table><thead><tr><th>Identity</th><th>Membership path</th></tr></thead><tbody>'+members.map(([identity,m])=>'<tr><td><button class="link" data-member="'+escapeHTML(identity)+'">'+escapeHTML(identity)+'</button><small>'+escapeHTML(m.id)+'</small></td><td>'+escapeHTML(m.groups.find(g=>g[0]===name)[1])+'</td></tr>').join('')+'</tbody></table><div class="note">Partial sample inventory. Identities whose membership is unavailable cannot be included or excluded conclusively.</div><h3>Potentially available services</h3><p class="muted">Group-related policy candidates, not a completed per-member access analysis. Service availability varies by member and selected context.</p><table><thead><tr><th>Service</th><th>Group contribution / remaining constraints</th></tr></thead><tbody>'+candidates.map(c=>'<tr><td><button class="link" data-group-service="'+escapeHTML(c[0])+'" data-group-scope="'+escapeHTML(name)+'">'+escapeHTML(c[0])+'</button></td><td><strong>'+escapeHTML(c[1])+'</strong><small>'+escapeHTML(c[2])+'</small></td></tr>').join('')+'</tbody></table><div class="note">Illustrative policy references only; no group-wide permission is inferred. Historical membership, nested-group resolution and live policy evaluation are Not Implemented.</div><button data-ni="Evaluate all authorized group members against candidate services">Analyze group access · Not Implemented</button>');
}
function showMembership(name){const m=memberships[name];if(!m)return;dialog('Group membership · '+name,'<p><strong>'+escapeHTML(m.id)+'</strong><br>Authority: '+escapeHTML(m.source)+' · Northstar Labs</p><p><span class="pill">Sample snapshot · 26 Oct 2026, 10:30</span></p>'+(m.unavailable?'<p class="status unknown">Membership unavailable</p><p>Missing evidence is not proof of non-membership.</p>':m.groups.length?'<table><thead><tr><th>Group</th><th>Membership path</th></tr></thead><tbody>'+m.groups.map(g=>'<tr><td>'+escapeHTML(g[0])+'</td><td>'+escapeHTML(g[1])+'</td></tr>').join('')+'</tbody></table>':'<p>No group memberships recorded in this fictional snapshot.</p>')+'<div class="note">Nested membership illustrates a source-reported path, not a new traversal rule. Real source identity, group IDs, validity and policy interpretation must be retained. A user’s groups do not become their machine’s groups. Membership alone does not grant access.</div><button data-ni="Retrieve authoritative membership history">Membership history · Not Implemented</button>')}
let direction='service',currentRows=[];
let reportHTML='',reportReturnFocus='report-button';
const pageHelp={
 inventory:{title:'Imported assertion settings',intro:'Import an authorized Policy-Service or Control Room assertion-settings response. This is unverified imported evidence, not a live connection.',steps:['Export GET /api/assertions/settings from Policy-Service or GET /api/assertions from Control Room using authorized production tooling.','Choose the JSON file; review the imported organization, revision and source separately from operator-entered provenance. Optionally import a same-organization baseline.','Review comparison status, revision distance and source byte/line measures. Settings equality excludes revision, file formatting and discarded runtime fields; it proves no semantic equivalence or activation. Open Baseline source provenance to record that file’s source system, reference and retrieval time independently.','Filter differing positions by type or literal text on either side. Navigate differences or jump to a matching position and add a paired snapshot reference to notes. Added/removed describe positions, not semantic edits.','Enter a preparer and purpose, optionally add provenance/notes, then use Report / print. Reports include all differences and complete sources regardless of filters or pages.'],recovery:'Service errors and failed settings loads are rejected. Reset comparison filters if a position is hidden. Baseline replacement resets comparison controls and baseline provenance; existing notes remain self-attributed to the files they cite. Current-snapshot replacement clears all review fields. Each retrieval time must be valid UTC and no later than its own snapshot’s import time. Invalid baseline provenance blocks reporting until corrected or cleared. SHA-256 requires HTTPS or localhost. Do not enter credentials; source files and reports can contain sensitive policy text.'},
 access:{title:'Access explorer',intro:'Review fictional potential access and membership. Permission is not connectivity, a live session or an installed visa.',steps:['Choose a service or identity and the request context and policy revision.','Filter and sort candidates, then inspect an explanation or membership details.','Use Report / print to capture all filtered matches, including those on other pages.'],recovery:'Reset filters if no candidates match. Unavailable evidence is not denial; membership alone is not a grant. Live evaluation is Not Implemented.'},
 policy:{title:'Policy changes',intro:'Compare the fixed r41 → r42 example with membership and source inputs held constant. This is not executable policy analysis.',steps:['Choose r41 before and r42 after to view the supported sample comparison.','Review the selected request impacts and partial activation evidence.','Explore the affected service or prepare a current-view report.'],recovery:'Other revision pairs have no comparison. Publication is not activation everywhere; no historical attributes were replayed.'},
 assertions:{title:'Assertion review',intro:'Browse sample definitions, roles, revisions and dependencies. These are not evaluator results.',steps:['Search assertions or filter by enforcing or diagnostic role.','Select a record and review its outcomes, dependencies and evidence gaps.','Add the selected assertion to a pack, or report all filtered assertions using Report / print.'],recovery:'Reset filters if no records match. A diagnostic observation is not an enforcing decision; satisfaction of one assertion does not prove permission.'},
 history:{title:'Entity history',intro:'Review illustrative observations, evidence gaps and derived facts. Links are not resolved from authoritative sources.',steps:['Choose an entity type and enter a reference.','Show sample history and inspect the timeline and coverage.','Prepare a report with the requested dates using Report / print.'],recovery:'No matching record does not prove absence. Missing acknowledgments do not prove failure. Time ranges do not query or filter the fixtures.'},
 reports:{title:'Report packs',intro:'Assemble tailored reports from fictional snapshots. Reporting dates are metadata only, not historical filtering.',steps:['Choose a template and scope, preview it, then add it to the pack.','Review or reorder reports and enter shared title, preparer and dates.','Preview the pack, download HTML or use Print / Save PDF.'],recovery:'Complete required fields and ensure the start date is not after the end date. Duplicate template/scope pairs are skipped. Exports are editable and not certified evidence; notes are unverified commentary.'}
};
function openPageHelp(){
 const topic=document.querySelector('[data-tab].selected').dataset.tab,help=pageHelp[topic];
 $('audit-help-title').textContent=help.title;
 $('audit-help-intro').textContent=help.intro;
 $('audit-help-steps').innerHTML=help.steps.map(step=>'<li>'+escapeHTML(step)+'</li>').join('');
 $('audit-help-recovery').textContent=help.recovery;
 $('audit-help-docs').href='help.html#'+topic;
 $('audit-help').showModal();
}
let importedInventory=null,inventoryGeneration=0;
let baselineInventory=null,baselineGeneration=0,baselineLoading=false;
let baselineComparison=null,comparisonMatches=[],comparisonPage=0,selectedComparison=null;
const comparisonPageSize=25;
function baselineProvenance(){
 return validateInventoryProvenance(Object.fromEntries(['system','reference','retrieved'].map(field=>[field,$('baseline-'+field).value])),baselineInventory.importedAt);
}
function updateBaselineProvenance(){
 if(!baselineInventory)return;
 try{
  const provenance=baselineProvenance();
  $('baseline-provenance-error').textContent='';
  $('baseline-provenance-status').textContent=provenance.system!=='unspecified'||provenance.reference||provenance.retrieved?
   'Baseline operator statements ready for reporting · unverified.':'No baseline operator provenance supplied.';
 }catch(error){
  $('baseline-provenance-status').textContent='Baseline provenance needs correction before reporting.';
  $('baseline-provenance-error').textContent=error.message;
 }
}
function inventoryProvenanceRows(prefix,provenance){
 return [['Source system',$(prefix+'-system').selectedOptions[0].textContent],
  ['Source reference',provenance.reference||'Not supplied'],['Source retrieved at',provenance.retrieved||'Not supplied']];
}
function comparisonLinesHTML(lines,interactive=false){
 return '<div class="table-wrap comparison-table"><table><thead><tr><th>Line position</th><th>Position type</th><th>Baseline text</th><th>Current text</th></tr></thead><tbody>'+
  lines.map(line=>'<tr'+(interactive?' data-comparison-line="'+line.line+'"'+(line.line===selectedComparison?' class="selected-comparison-line"':''):'')+'><td>'+
   (interactive?'<button class="link" data-comparison-select="'+line.line+'" aria-pressed="'+(line.line===selectedComparison)+'">'+line.line+'</button>':line.line)+
   '</td><td>'+escapeHTML(line.kind)+'</td>'+[line.baseline,line.current].map(text=>'<td>'+
    (text===null?'<em>No line at this position</em>':text===''?'<em>Blank line</em>':'<span class="comparison-text">'+escapeHTML(text)+'</span>')+'</td>').join('')+'</tr>').join('')+
  '</tbody></table></div>';
}
function resetComparisonReview(){
 comparisonPage=0;selectedComparison=null;
 $('comparison-search').value='';$('comparison-kind').value='all';$('comparison-case').value='insensitive';
 $('comparison-position').value='';$('comparison-error').textContent='';
 updateComparisonReview();
}
function updateComparisonReview(){
 comparisonMatches=baselineComparison?filterComparisonLines(baselineComparison.lines,$('comparison-search').value,$('comparison-kind').value,$('comparison-case').value==='sensitive'):[];
 comparisonPage=0;selectedComparison=comparisonMatches[0]?.line??null;
 $('comparison-error').textContent='';renderComparisonReview();
}
function renderComparisonReview(){
 const count=comparisonMatches.length,pages=Math.ceil(count/comparisonPageSize);
 $('comparison-count').textContent=count+' of '+(baselineComparison?.lines.length??0)+' differing positions match. Reports include all differences.';
 $('comparison-page').textContent=count?'Page '+(comparisonPage+1)+' of '+pages+' · 25 positions per page':'No pages';
 $('comparison-page-previous').disabled=!count||comparisonPage===0;
 $('comparison-page-next').disabled=!count||comparisonPage>=pages-1;
 $('comparison-lines').innerHTML=count?comparisonLinesHTML(comparisonMatches.slice(comparisonPage*comparisonPageSize,(comparisonPage+1)*comparisonPageSize),true):
  '<p>'+(!baselineComparison?.lines.length?(baselineComparison?.sourceChanged?'No differing displayed line positions. Exact source text differs in line endings.':'No differing displayed line positions.'):'No differing positions match these filters. Reset comparison filters to review all differences.')+'</p>';
 const index=comparisonMatches.findIndex(line=>line.line===selectedComparison);
 $('comparison-selection').textContent=index<0?'No difference selected.':'Difference '+(index+1)+' of '+count+' · line position '+selectedComparison+' · '+comparisonMatches[index].kind;
 $('comparison-position').value=selectedComparison===null?'':String(selectedComparison);
 $('comparison-previous').disabled=count<2;$('comparison-next').disabled=count<2;
 $('comparison-go').disabled=!count;$('comparison-note').disabled=index<0;
}
function selectComparisonPosition(position){
 const index=comparisonMatches.findIndex(line=>line.line===position);
 if(index<0)throw new Error('Choose a differing position matching the current filters, or reset comparison filters.');
 selectedComparison=position;comparisonPage=Math.floor(index/comparisonPageSize);
 $('comparison-error').textContent='';renderComparisonReview();
}
function moveComparisonPosition(step){
 const index=comparisonMatches.findIndex(line=>line.line===selectedComparison);
 selectComparisonPosition(comparisonMatches[(index+step+comparisonMatches.length)%comparisonMatches.length].line);
}
function addComparisonReference(){
 const line=comparisonMatches.find(line=>line.line===selectedComparison);
 if(!line){$('comparison-error').textContent='Select a differing position before adding a reference.';return}
 const describe=record=>'revision '+record.data.settings.revision+' · file '+record.file+' · imported '+record.importedAt+' · file SHA-256 '+record.digest;
 const reference='Comparison position '+line.line+' ('+line.kind+', positional only) · organization '+importedInventory.data.organization_id+
  ' · baseline: '+describe(baselineInventory)+' · current: '+describe(importedInventory)+' (unverified reviewer reference; applies only to these snapshots)';
 const notes=$('inventory-notes'),value=notes.value+(notes.value?'\n':'')+reference;
 if(value.length>4000){$('comparison-error').textContent='The comparison reference would exceed the 4000-character notes limit.';return}
 notes.value=value;$('comparison-error').textContent='';notes.focus();
}
async function readInventoryFile(file){
 if(file.size>maxInventoryBytes)throw new Error('File exceeds the 256 KiB limit.');
 const bytes=await file.arrayBuffer();
 const data=parseAssertionInventory(new TextDecoder('utf-8',{fatal:true}).decode(bytes));
 const digest=crypto.subtle?[...new Uint8Array(await crypto.subtle.digest('SHA-256',bytes))].map(b=>b.toString(16).padStart(2,'0')).join(''):'Unavailable: SHA-256 requires HTTPS or localhost; this page is not a secure context.';
 return {data,file:file.name,size:bytes.byteLength,digest,importedAt:new Date().toISOString()};
}
function clearBaseline(){
 baselineGeneration++;baselineInventory=null;baselineLoading=false;
 baselineComparison=null;resetComparisonReview();$('comparison-review').classList.add('hidden');
 $('baseline-system').value='unspecified';$('baseline-reference').value='';$('baseline-retrieved').value='';
 $('baseline-provenance').open=false;
 $('baseline-provenance-status').textContent='';$('baseline-provenance-error').textContent='';
 $('baseline-file').value='';$('baseline-results').replaceChildren();
 $('baseline-results').classList.add('hidden');$('baseline-clear').disabled=true;
 $('baseline-status').textContent='No baseline imported.';$('baseline-error').textContent='';
}
function baselineComparisonHTML(includeLines=true){
 const comparison=baselineComparison;
 return '<h3>Baseline provenance · unverified import</h3>'+
  reportTable(['Field','Baseline value'],[['File',baselineInventory.file],['File SHA-256',baselineInventory.digest],
   ['File size',baselineInventory.size+' bytes'],
   ['Imported at',baselineInventory.importedAt+' · browser clock'],['Organization ID',baselineInventory.data.organization_id],
   ['Organization name',baselineInventory.data.organization_name],['Source retrieval time','Not supplied']])+
  '<p>'+escapeHTML(comparison.warning)+'</p>'+
  reportTable(['Comparison summary','Value'],[['Status',comparison.status],['Revision difference (current minus baseline)',String(comparison.revisionDelta)],
   ['Settings content excluding revision',comparison.contentChanged?'Different':'Identical'],
   ['Authority / history','Not verified. Revision distance is not a count of edits, missing exports or activations. File formatting and ignored runtime fields are excluded from settings equality.']])+
  reportTable(['Source measure','Baseline','Current','Difference (current minus baseline)'],[
   ['UTF-8 bytes',String(comparison.sourceSize.bytes.baseline),String(comparison.sourceSize.bytes.current),String(comparison.sourceSize.bytes.current-comparison.sourceSize.bytes.baseline)],
   ['Displayed lines',String(comparison.sourceSize.lines.baseline),String(comparison.sourceSize.lines.current),String(comparison.sourceSize.lines.current-comparison.sourceSize.lines.baseline)]])+
  (comparison.nameChanged?'<p>Organization names differ for the same ID. No identity rename is verified.</p>':'')+
  reportTable(['Settings field','Baseline','Current','Text/value changed'],comparison.fields.map(field=>[field.field,String(field.baseline),String(field.current),field.changed?'Yes':'No']))+
  '<p>Source text changed: '+(comparison.sourceChanged?'Yes':'No')+' · '+comparison.lines.length+' differing line positions. Positional comparison only: insertions can shift later lines; line endings are normalized for the line view. Exact source-change detection includes line endings. No semantic or access-impact analysis performed.</p>'+
  reportTable(['Position classification','Count'],Object.entries(comparison.counts).map(([kind,count])=>[kind,String(count)]))+
  '<p>Added/removed mean a line exists only on one side at that position; changed means both exist with different text. Unchanged counts normalized displayed positions, not exact line-ending equality. Blank lines are present lines. These are not counts of semantic edits or assertions.</p>'+
  (includeLines?(comparison.lines.length?comparisonLinesHTML(comparison.lines):'<p>No differing displayed line positions.</p>'):'');
}
async function importBaseline(){
 const file=$('baseline-file').files[0];
 clearBaseline();if(!file)return;
 const generation=baselineGeneration,current=importedInventory;
 baselineLoading=true;
 $('baseline-clear').disabled=false;
 $('baseline-status').textContent='Reading baseline…';
 try{
  const record=await readInventoryFile(file);
  if(generation!==baselineGeneration||current!==importedInventory)return;
  baselineComparison=compareAssertionSnapshots(record.data,current.data);
  baselineInventory=record;
  $('baseline-results').innerHTML=baselineComparisonHTML(false);
  resetComparisonReview();$('comparison-review').classList.remove('hidden');
  updateBaselineProvenance();
  $('baseline-results').classList.remove('hidden');$('baseline-clear').disabled=false;
  $('baseline-status').textContent='Baseline revision '+record.data.settings.revision+' compared with current revision '+current.data.settings.revision+' · unverified snapshots.';
 }catch(error){
  if(generation!==baselineGeneration)return;
  baselineInventory=null;$('baseline-status').textContent='No baseline imported.';
  $('baseline-clear').disabled=true;
  $('baseline-error').textContent='Baseline import failed: '+error.message;
 }finally{
  if(generation===baselineGeneration)baselineLoading=false;
 }
}
let sourceMatches=[],sourceMatchIndex=-1,selectedSourceLine=null;
function resetSourceReview(){
 sourceMatches=[];sourceMatchIndex=-1;selectedSourceLine=null;
 $('source-search').value='';$('source-line').value='';$('source-case').value='insensitive';
 $('source-summary').textContent='';$('source-matches').textContent='';$('source-error').textContent='';
 ['source-previous','source-next','source-go','source-note'].forEach(id=>$(id).disabled=true);
}
function renderSourceReview(){
 const source=importedInventory.data.settings.source,lines=sourceReviewLines(source);
 $('inventory-source').innerHTML=lines.length?lines.map((line,i)=>'<span class="source-line" data-line="'+(i+1)+'">'+escapeHTML(line)+'</span>').join('\n'):'(Exported source text is empty.)';
 $('source-summary').textContent=lines.length+' source lines · '+new TextEncoder().encode(source).byteLength+' UTF-8 bytes · settings revision '+importedInventory.data.settings.revision+'. Line numbers are display aids, not assertion IDs.';
 $('source-go').disabled=!lines.length;
 updateSourceSearch();
}
function selectSourceLine(line){
 selectedSourceLine=line;
 document.querySelectorAll('#inventory-source .source-line').forEach(element=>element.classList.toggle('selected-source-line',Number(element.dataset.line)===line));
 const element=$('inventory-source').querySelector('[data-line="'+line+'"]');
 // Scroll the source pane, not the whole investigation.
 $('inventory-source').scrollTop=element.offsetTop-$('inventory-source').offsetTop;
 $('source-line').value=String(line);$('source-note').disabled=false;
}
function updateSourceSearch(){
 if(!importedInventory)return;
 const query=$('source-search').value;
 sourceMatches=matchingSourceLines(importedInventory.data.settings.source,query,$('source-case').value==='sensitive');
 const matching=new Set(sourceMatches);
 sourceMatchIndex=sourceMatches.length?0:-1;selectedSourceLine=null;
 document.querySelectorAll('#inventory-source .source-line').forEach(element=>{
  element.classList.toggle('matched-source-line',matching.has(Number(element.dataset.line)));
  element.classList.remove('selected-source-line');
 });
 $('source-note').disabled=true;$('source-error').textContent='';$('source-line').value='';
 $('source-previous').disabled=sourceMatches.length<2;$('source-next').disabled=sourceMatches.length<2;
 if(sourceMatches.length)selectSourceLine(sourceMatches[0]);
 renderSourceMatchCount();
}
function renderSourceMatchCount(){
 $('source-matches').textContent=!$('source-search').value?'No source search. Reports include the complete source.':sourceMatches.length?'Matching line '+(sourceMatchIndex+1)+' of '+sourceMatches.length+' · source line '+sourceMatches[sourceMatchIndex]: 'No matching source lines. This is text search, not an assertion evaluation.';
}
function moveSourceMatch(step){
 sourceMatchIndex=(sourceMatchIndex+step+sourceMatches.length)%sourceMatches.length;
 selectSourceLine(sourceMatches[sourceMatchIndex]);renderSourceMatchCount();
}
function inventoryRows(){
 const record=importedInventory,data=record.data;
 return [['Organization ID',data.organization_id],['Organization name',data.organization_name],
  ['Organization authority','Values asserted by imported Policy-Service response; not independently verified'],
  ['Settings revision',String(data.settings.revision)],['Enabled (exported setting)',String(data.settings.enabled)],
  ['Interval seconds (exported setting)',String(data.settings.interval_seconds)],
  ['Source text size',new TextEncoder().encode(data.settings.source).byteLength+' UTF-8 bytes'],
  ['Source text presence',data.settings.source.length?'Text supplied; not parsed or evaluated':'Empty in export; no wider absence inferred'],
  ['File',record.file],['File size',record.size+' bytes'],['File SHA-256',record.digest],['Imported at',record.importedAt+' · browser clock'],
  ['Source retrieval time','Not supplied; import time is not retrieval time'],
  ['Evidence scope','Settings document only; not an assertion evaluation or complete inventory']];
}
function clearInventory(){
 clearBaseline();
 inventoryGeneration++;importedInventory=null;
 $('inventory-record').classList.add('hidden');$('inventory-clear').disabled=true;
 $('inventory-metadata').replaceChildren();$('inventory-source').textContent='';
 $('inventory-status').textContent='No snapshot imported.';
 resetSourceReview();
 ['reference','retrieved','preparer','purpose','notes'].forEach(field=>$('inventory-'+field).value='');
 $('inventory-system').value='unspecified';
}
async function importInventory(){
 clearInventory();$('inventory-error').textContent='';
 const file=$('inventory-file').files[0];if(!file)return;
 const generation=inventoryGeneration;
 $('inventory-status').textContent='Reading snapshot…';
 try{
  const record=await readInventoryFile(file),data=record.data;
  if(generation!==inventoryGeneration)return;
  importedInventory=record;
  $('inventory-metadata').innerHTML=reportTable(['Field','Imported value'],inventoryRows());
  renderSourceReview();
  $('inventory-record').classList.remove('hidden');$('inventory-clear').disabled=false;
  $('inventory-status').textContent='Imported settings revision '+data.settings.revision+' · '+data.organization_name+' · unverified snapshot, not live data.';
 }catch(error){
  if(generation!==inventoryGeneration)return;
  $('inventory-status').textContent='No snapshot imported.';
  $('inventory-error').textContent='Import failed: '+error.message;
 }
}
function inventoryReport(){
 if(!importedInventory){$('inventory-error').textContent='Import a valid assertion-settings snapshot before preparing a report.';return}
 if(baselineLoading){$('inventory-error').textContent='Wait for the baseline import to finish before preparing a report.';return}
 let review;
 try{
  review=validateInventoryReview(Object.fromEntries(['system','reference','retrieved','preparer','purpose','notes'].map(field=>[field,$('inventory-'+field).value])),importedInventory.importedAt);
 }catch(error){$('inventory-error').textContent='Report preparation failed: '+error.message;return}
 let baselineReview=null;
 if(baselineInventory){
  try{baselineReview=baselineProvenance()}
  catch(error){
   updateBaselineProvenance();$('baseline-provenance').open=true;
   $('inventory-error').textContent='Baseline report preparation failed: '+error.message;return;
  }
 }
 $('inventory-error').textContent='';
 const generated=new Date().toISOString(),reference='IMPORTED-'+crypto.randomUUID();
 const provenance=[...inventoryProvenanceRows('inventory',review),['Prepared by',review.preparer+' · not authenticated'],['Purpose',review.purpose]];
 reportHTML='<h1>Imported assertion-settings inventory</h1><div class="report-warning"><strong>IMPORTED SNAPSHOT — NOT INDEPENDENTLY VERIFIED</strong><p>No live sources queried or evaluations performed. This report contains no sample fixtures.</p></div>'+
  reportTable(['Report metadata','Value'],[['Report reference',reference],['Generated at',generated+' · browser clock']])+
  '<h2>Imported facts and import metadata</h2>'+reportTable(['Field','Imported value'],inventoryRows())+
  '<h2>Operator-entered provenance · unverified</h2>'+reportTable(['Field','Operator statement'],provenance)+
  (review.notes?'<h2>Reviewer notes · unverified commentary</h2><p class="review-notes">'+escapeHTML(review.notes)+'</p>':'')+
  '<h2>Assertion source · as exported</h2><pre class="code">'+escapeHTML(importedInventory.data.settings.source||'(Exported source text is empty.)')+'</pre>'+
  (baselineInventory?'<h2>Baseline operator-entered provenance · unverified</h2>'+reportTable(['Field','Operator statement'],inventoryProvenanceRows('baseline',baselineReview))+
   '<h2>Imported baseline comparison</h2>'+baselineComparisonHTML()+'<h3>Baseline source · complete, as exported</h3><pre class="code">'+escapeHTML(baselineInventory.data.settings.source||'(Exported source text is empty.)')+'</pre>':'<p>No valid baseline loaded; this report contains only the current snapshot.</p>')+
  '<h2>Limitations</h2><p>Organization identity is asserted by the imported response, not verified against a trusted directory. This document does not establish individual assertion roles, evaluation outcomes, completeness, historical validity or source freshness. The SHA-256, when available, fingerprints the input file only; it does not authenticate its origin. This editable report is not signed and has no chain-of-custody guarantee.</p>';
 $('report-document').innerHTML=reportHTML;reportReturnFocus='report-button';
 document.body.classList.add('reporting');$('report-preview').classList.remove('hidden');$('report-back').focus();
}
const reportTemplates={
 identity:{name:'User / machine access review',description:'Review one canonical identity, its memberships and service outcomes in explicitly described sample contexts. Machines and users remain separate identities.',scopes:Object.keys(memberships),sections:'Identity authority · direct/nested memberships · service outcomes · request context and missing evidence'},
 group:{name:'Group membership & access',description:'Review direct/nested members and group-related service candidates. Other policy conditions remain applicable.',scopes:['Finance','Engineering','Employees','Managed devices'],sections:'Members and membership paths · candidate services · remaining constraints'},
 service:{name:'Service access review',description:'Review a selected service across known sample users, groups and machines. This is not an exhaustive access list.',scopes:services,sections:'Selected service · user/group/machine candidates · remaining constraints'},
 policy:{name:'Policy change & impact',description:'Compare the fixed r41 → r42 fixture with identity and device context held constant.',scopes:['Corporate service access · r41 → r42'],sections:'Semantic change summary · sample request comparisons · publication/activation evidence'},
 assertion:{name:'Assertion review',description:'Review sample assertion intent, enforcing/diagnostic role, policy references, evaluation evidence and source dependencies. These are illustrative definitions, not executable policy.',scopes:['All sample assertions',...assertionFixtures.map(a=>a.name)],sections:'Assertion definition and revision · policy references · selected-context outcomes · dependencies and missing evidence'},
 coverage:{name:'Evidence coverage',description:'Document the limits of the sample evidence and distinguish unavailable observations from negative results.',scopes:['Northstar Labs · all sample sources'],sections:'Source coverage · retention assumptions · missing acknowledgments · reconstruction limits'}
};
let selectedTemplate='group',reportPack=[];
function renderCatalog(){
 const term=$('template-search').value.trim().toLowerCase();
 const matches=Object.entries(reportTemplates).filter(([,t])=>(t.name+' '+t.description).toLowerCase().includes(term));
 $('template-count').textContent=matches.length+' of '+Object.keys(reportTemplates).length+' templates match. Search does not change the pack or selected template.';
 $('template-catalog').innerHTML=matches.length?'<div class="seg">'+matches.map(([key,t])=>'<button data-template="'+key+'" class="'+(key===selectedTemplate?'selected':'')+'">'+escapeHTML(t.name)+' <small>('+reportPack.filter(r=>r.key===key).length+' in pack)</small></button>').join('')+'</div>':'<p>No matching templates. Clear the search to see all templates.</p>';
}
function availableTemplateScopes(){
 return selectedTemplate==='identity'?reportTemplates.identity.scopes.filter(name=>memberships[name].id.startsWith($('template-category').value+':')):reportTemplates[selectedTemplate].scopes;
}
function renderTemplateScopes(){
 selectOptions('template-scope',availableTemplateScopes());
 $('pack-add-all').textContent=selectedTemplate==='identity'?'Add all '+$('template-category').selectedOptions[0].textContent.toLowerCase():'Add all scopes';
}
function chooseTemplate(key){
 selectedTemplate=key;const t=reportTemplates[key];
 $('template-name').textContent=t.name;
 $('template-description').textContent=t.description;
 $('template-category-label').classList.toggle('hidden',key!=='identity');
 renderTemplateScopes();
 $('template-sections').textContent='Included sections: '+t.sections;
 $('pack-remove-template').disabled=!reportPack.some(r=>r.key===key);
 renderCatalog();
 $('pack-feedback').textContent='';
}
function renderPack(){
 $('pack-summary').textContent=reportPack.length+' reports · '+new Set(reportPack.map(r=>r.key)).size+' template types · '+new Set(reportPack.map(r=>r.scope)).size+' distinct scopes. Sample evidence only.';
 $('pack-clear').disabled=reportPack.length===0;
 $('pack-remove-template').disabled=!reportPack.some(r=>r.key===selectedTemplate);
 $('pack-empty').classList.toggle('hidden',reportPack.length>0);
 $('pack-coverage').innerHTML=Object.entries(reportTemplates).map(([key,t])=>{
  const count=reportPack.filter(r=>r.key===key).length;
  return '<li>'+escapeHTML(t.name)+': '+(count?count+' of '+t.scopes.length+' available scopes selected':'Not included')+'</li>';
 }).join('');
 $('pack-list').innerHTML=reportPack.map((item,i)=>'<li style="margin-bottom:15px"><strong>'+escapeHTML(reportTemplates[item.key].name)+'</strong><br><small>'+escapeHTML(item.scope)+'</small><div class="row"><button data-pack-preview="'+i+'" aria-label="Preview report '+(i+1)+'">Preview</button><button data-pack-template="'+i+'" aria-label="Choose template for report '+(i+1)+'">Choose template / scope</button><button data-pack-up="'+i+'" '+(i===0?'disabled':'')+' aria-label="Move report '+(i+1)+' up">Up</button><button data-pack-down="'+i+'" '+(i===reportPack.length-1?'disabled':'')+' aria-label="Move report '+(i+1)+' down">Down</button><button data-pack-remove="'+i+'" aria-label="Remove report '+(i+1)+'">Remove</button></div></li>').join('');
 renderCatalog();
}
function previewTemplate(item){
 dialog('Template preview · '+reportTemplates[item.key].name,'<div class="prototype"><strong>SAMPLE ONLY — NOT AUDIT EVIDENCE.</strong> Previewing does not add or change reports in the pack.</div><p>Scope: '+escapeHTML(item.scope)+'</p>'+templateContent(item));
}
function addPackScopes(scopes,key=selectedTemplate){
 let added=0;
 for(const scope of scopes){
  if(!reportPack.some(r=>r.key===key&&r.scope===scope)){reportPack.push({key,scope});added++}
 }
 renderPack();
 $('pack-feedback').textContent=added+' reports added; '+(scopes.length-added)+' already in the pack. Template: '+reportTemplates[key].name+'.';
 return added;
}
function assertionDetails(a){
 return reportTable(['Field','Sample evidence'],[['Canonical reference / revision',a.id],['Role',a.role],['Definition (plain-language summary)',a.definition],['Policy / service references',a.policy],['Selected-context outcomes',a.outcomes],['Source dependency / observation',a.source],['Missing evidence / interpretation',a.limit]]);
}
function assertionReportContent(records){
 return '<p>Assertion roles and outcomes below are fictional review fixtures, not compiler/evaluator results. The shared report period does not filter these snapshots.</p>'+
  (records.length?records.map(a=>'<h3>'+escapeHTML(a.name)+'</h3>'+assertionDetails(a)).join(''):'<p>No sample assertions match these filters. This does not establish absence of assertions or evidence.</p>')+
  '<p>No aggregate pass rate is calculated: these selected examples are not a complete evaluation population. Satisfaction of one assertion is not proof that the overall policy permits access. Exact source, author/review/change history, executable definitions and historical evaluation evidence are Not Implemented.</p>';
}
let selectedAssertion=assertionFixtures[0].id;
function filteredAssertions(){return matchingAssertions($('assertion-search').value,$('assertion-role').value)}
function currentAssertion(){return filteredAssertions().find(a=>a.id===selectedAssertion)}
function renderAssertions(){
 const records=filteredAssertions();
 if(!records.some(a=>a.id===selectedAssertion))selectedAssertion=records.length?records[0].id:null;
 const selected=currentAssertion();
 $('assertion-count').textContent=records.length+' of '+assertionFixtures.length+' sample assertions match. Current-view reports include all matches, not just the selected assertion.';
 $('assertion-list').innerHTML=records.length?records.map(a=>'<button data-assertion="'+escapeHTML(a.id)+'" class="'+(a.id===selectedAssertion?'selected':'')+'" aria-pressed="'+(a.id===selectedAssertion)+'">'+escapeHTML(a.name)+'<small>'+escapeHTML(a.role+' / '+a.id)+'</small></button>').join(''):'<p>No matching sample assertions. Reset filters to see all examples; no absence of evidence is inferred.</p>';
 $('assertion-heading').textContent=selected?selected.name:'No assertion selected';
 $('assertion-details').innerHTML=selected?assertionDetails(selected):'<p>Select an assertion after changing or resetting the filters.</p>';
 ['assertion-policy','assertion-access','assertion-add'].forEach(id=>$(id).disabled=!selected);
 $('assertion-feedback').textContent='';
}
function templateContent(item){
 const scope=escapeHTML(item.scope);
 if(item.key==='identity'){
  const m=memberships[item.scope],rows=identityServiceRows(item.scope);
  return '<p>Identity: '+scope+' / '+escapeHTML(m.id)+'<br>Authority: '+escapeHTML(m.source)+' · sample snapshot 2026-10-26 10:30 UTC</p><h3>Group membership</h3><p>'+escapeHTML(membershipText(item.scope))+'</p><h3>Potential service access · r42</h3>'+reportTable(['Service','Sample outcome','Request context / constraint'],rows.map(r=>[r[0],labels[r[2]],r[3]]))+'<p>These are selected fixture requests, not a sweep of every possible user/device pairing. Missing request fixtures are indeterminate, not denied. A machine review may include a named user or workload context; it does not grant access to every user on that machine.</p>';
 }
 if(item.key==='assertion'){
  return assertionReportContent(assertionFixtures.filter(a=>item.scope==='All sample assertions'||a.name===item.scope));
 }
 if(item.key==='group'){
  const members=Object.entries(memberships).filter(([,m])=>!m.unavailable&&m.groups.some(g=>g[0]===item.scope));
  const candidate=item.scope==='Finance'?'Finance Reports — Finance membership plus managed device/session required':item.scope==='Engineering'?'Engineering Git — Engineering membership plus applicable device/workload context':item.scope==='Employees'?'Finance Reports — r41 employee condition; r42 additionally requires Finance and managed device':'Finance Reports — device requirement only; Finance user/session still required';
  return '<h3>Membership snapshot</h3>'+reportTable(['Identity','Authority','Membership path'],members.map(([name,m])=>[name+' / '+m.id,m.source,m.groups.find(g=>g[0]===item.scope)[1]]))+'<h3>Potential service candidates</h3><p>'+escapeHTML(candidate)+'</p><p>Other services have not been exhaustively analyzed. Membership is not a group-wide grant; unavailable memberships are excluded from this known-member list without concluding non-membership.</p>';
 }
 if(item.key==='service'){
  const rows=serviceCandidateRows(item.scope);
  return '<p>Service: '+scope+' · r42 · fixed sample contexts.</p>'+categoryReport(rows);
 }
 if(item.key==='policy')return '<p>Finance Reports changes from employee membership to Finance membership plus a managed device. Semantic summary only, not executable ZPL.</p>'+reportTable(['Request','r41','r42'],[['Alice / managed laptop → Reports','Permitted','Permitted'],['Bob / managed laptop → Reports','Permitted','Denied'],['Priya / unspecified device → Reports','Permitted','Context required'],['Payroll worker → Payroll API','Permitted','Permitted']])+'<p>r42 published 26 Oct 09:10 UTC; 2 of 3 activation acknowledgments observed at 09:12. Partial activation evidence is not universal enforcement. Fixed sample inputs are held constant; no historical attributes replayed.</p>';
 return reportTable(['Source / evidence','Known limitation'],[['Company directory','Jordan membership unavailable; direct/nested paths are fictional snapshots.'],['Device registry','finance-laptop-22 and backup-worker-03 evidence unavailable.'],['Visa lifecycle','Source-adapter install acknowledgment missing; expiry is derived, not an observed event.'],['Policy activation','Only 2 of 3 acknowledgments observed.'],['History retention','7-day illustrative window, not verified retention or completeness.']]);
}
function browseGroups(){
 dialog('Browse sample groups','<p>Known sample groups only. Unavailable memberships are not counted.</p>'+['user','machine'].map(kind=>{
  const names=groups.filter(group=>(group==='Managed devices'?'machine':'user')===kind);
  return '<section data-group-category="'+kind+'"><h3>'+(kind==='user'?'User groups · Company directory':'Machine groups · Device registry')+'</h3>'+
   names.map(group=>'<p><button data-group="'+escapeHTML(group)+'">'+escapeHTML(group)+'</button> · '+Object.values(memberships).filter(m=>!m.unavailable&&m.id.startsWith(kind+':')&&m.groups.some(g=>g[0]===group)).length+' known '+(kind==='user'?'users':'machines')+'</p>').join('')+'</section>';
 }).join(''));
}
function resetAccessFilters(){
 $('candidate-kind').value='all';
 $('group-filter').value='all';
 $('outcome').value='all';
 $('candidate-search').value='';
 renderAccess();
}
function previewPack(){
 const title=$('pack-title').value.trim(),author=$('pack-author').value.trim(),purpose=$('pack-purpose').value.trim(),start=$('pack-start').value,end=$('pack-end').value;
 if(!reportPack.length||!title||!author||!purpose||!start||!end||start>end){$('pack-error').textContent='Add at least one report and enter a title, preparer, purpose and valid date range.';return}
 $('pack-error').textContent='';
 reportHTML='<h1>'+escapeHTML(title)+'</h1><div class="report-warning"><strong>SAMPLE REPORT PACK — NOT AUDIT EVIDENCE</strong><br>No production sources queried or policy evaluations performed.</div><dl><dt>Pack reference</dt><dd>SAMPLE-'+crypto.randomUUID()+'</dd><dt>Organization</dt><dd>Northstar Labs · fictional</dd><dt>Prepared by</dt><dd>'+escapeHTML(author)+' · user-entered, not authenticated</dd><dt>Purpose / audience</dt><dd>'+escapeHTML(purpose)+'</dd><dt>Reporting period</dt><dd>'+start+' through '+end+' inclusive · UTC dates · metadata only</dd><dt>Generated</dt><dd>'+new Date().toISOString()+' · browser clock</dd><dt>Evidence cutoff</dt><dd>2026-10-26 10:30 UTC · fixed sample snapshot</dd><dt>Template version</dt><dd>Prototype v1 · all templates</dd></dl><h2 id="pack-contents">Contents</h2><ol>'+reportPack.map((item,i)=>'<li><a href="#pack-section-'+(i+1)+'">'+escapeHTML(reportTemplates[item.key].name+' / '+item.scope)+'</a></li>').join('')+'</ol>'+reportPack.map((item,i)=>'<section class="pack-report" id="pack-section-'+(i+1)+'"><h2>'+(i+1)+'. '+escapeHTML(reportTemplates[item.key].name)+'</h2><p><strong>Scope:</strong> '+escapeHTML(item.scope)+'</p>'+templateContent(item)+'<div class="note">Partial fictional evidence. Unknown or unobserved is not denied. Policy permission is not reachability. Mandatory limitations apply to this section.</div><p><a href="#pack-contents">Back to contents</a></p></section>').join('')+'<h2>Mandatory limitations</h2><p>Dates do not filter the fictional records. Membership snapshots are not historical reconstruction. Candidate inventories are partial; other constraints may apply. No source verification, signature, chain of custody, compliance certification or immutable retention is provided. This HTML is editable. Audience labels do not authorize delivery.</p>';
 const notes=$('pack-notes').value.trim();
 if(notes)reportHTML+='<section><h2>Reviewer notes · unverified commentary</h2><p class="review-notes">'+escapeHTML(notes)+'</p></section>';
 reportReturnFocus=document.activeElement.id||'pack-preview';
 $('report-document').innerHTML=reportHTML;document.body.classList.add('reporting');$('report-preview').classList.remove('hidden');$('report-back').focus();
}
function currentReportScope(active){
 if(active==='access')return $('access-heading').textContent+(direction==='service'?' · result type: '+$('candidate-kind').selectedOptions[0].textContent:'');
 if(active==='policy')return 'Corporate service access · requested '+$('before').value+' → '+$('after').value;
 if(active==='assertions')return 'All filtered sample assertions · search: '+($('assertion-search').value.trim()||'None')+' · role: '+$('assertion-role').selectedOptions[0].textContent;
 return $('history-heading').textContent;
}
function reportSetup(){
 const active=document.querySelector('[data-tab].selected').dataset.tab;
 if(active==='inventory'){inventoryReport();return}
 if(active==='reports'){previewPack();return}
 const scope=currentReportScope(active);
 dialog('Prepare sample report','<p>Capture the current <strong>'+escapeHTML(active)+'</strong> view. Report dates document the requested review period; they do not query or filter the fictional records.</p><div class="filters"><label class="wide">Report title<input id="report-title" value="Northstar Audit review"></label><label class="wide">Prepared by<input id="report-author" placeholder="Name / audit team"></label><label>Period start (UTC date)<input type="date" id="report-start" value="2026-10-26"></label><label>Period end (UTC date)<input type="date" id="report-end" value="2026-10-26"></label></div><p class="note">Scope: '+escapeHTML(scope)+'. No real investigation, historical replay or source verification has run. PDF is produced through your browser’s print dialog.</p><p id="report-error" role="alert"></p><button class="primary" id="report-build">Preview sample report</button>');
}
function buildReport(){
 const start=$('report-start').value,end=$('report-end').value,title=$('report-title').value.trim(),author=$('report-author').value.trim();
 if(!start||!end||start>end||!title||!author){$('report-error').textContent='Enter a title, preparer and valid date range with the start on or before the end.';return}
 const active=document.querySelector('[data-tab].selected').dataset.tab;
 const scope=currentReportScope(active);
 const generated=new Date().toISOString();
 const id='SAMPLE-'+crypto.randomUUID();
 let content='';
 if(active==='access'){
  const visible=filteredCandidates().map(({r})=>r);
  content='<h2>Potential access · illustrative results</h2><p>Scope: all filtered matches across pages. Sort: '+escapeHTML($('candidate-sort').selectedOptions[0].textContent)+'. Pagination limits screen display only.</p><p>Candidate search: '+escapeHTML($('candidate-search').value.trim()||'None')+'</p><p>'+escapeHTML($('coverage').textContent)+'</p><p>Outcome filter: '+escapeHTML($('outcome').selectedOptions[0].textContent)+' · membership filter: '+escapeHTML(direction==='service'?$('group-filter').selectedOptions[0].textContent:'Selected identity')+'</p><table><thead><tr><th>Entity</th><th>Membership</th><th>Result</th><th>Reason</th></tr></thead><tbody>'+visible.map(r=>'<tr><td>'+escapeHTML(r[0])+'<br>'+escapeHTML(r[1])+'</td><td>'+escapeHTML(membershipText(direction==='service'?r[0]:$('target').value))+'</td><td>'+escapeHTML(labels[r[2]])+'</td><td>'+escapeHTML(r[3])+'</td></tr>').join('')+'</tbody></table><p>'+visible.length+' displayed sample results. Policy: '+escapeHTML($('access-revision').value)+'. Context: '+escapeHTML(accessContext())+'</p>';
  if(direction==='service')content='<h2>Potential access · illustrative results</h2><p>All filtered matches across pages. Result type: '+escapeHTML($('candidate-kind').selectedOptions[0].textContent)+' · search: '+escapeHTML($('candidate-search').value)+' · outcome: '+escapeHTML($('outcome').value)+' · membership: '+escapeHTML($('group-filter').value)+' · sort: '+escapeHTML($('candidate-sort').selectedOptions[0].textContent)+'</p><p>'+escapeHTML(accessContext())+'</p>'+categoryReport(visible);
 }else if(active==='assertions'){
  content='<h2>Assertion review · all filtered matches</h2>'+assertionReportContent(filteredAssertions());
 }else if(active==='policy'){
  content='<h2>Policy comparison</h2><p>'+escapeHTML($('policy-scope').textContent)+'</p>';
  if(supportedPolicyComparison()){
   const clone=$('policy-results').cloneNode(true);clone.querySelectorAll('button,a').forEach(b=>b.remove());clone.removeAttribute('id');clone.querySelectorAll('[id]').forEach(e=>e.removeAttribute('id'));content+=clone.innerHTML+'<p>Semantic summary, not executable ZPL. No compiler or effective-policy comparison has run. Membership and source inputs are held constant; historical attributes were not replayed.</p>';
  }
 }else{
  content='<h2>'+escapeHTML($('history-heading').textContent)+'</h2><p>Selected UI window: '+escapeHTML($('history-window').value)+'. Reporting dates do not change this sample timeline.</p>'+$('history-events').innerHTML+'<h3>Evidence coverage</h3><p>Incomplete sample evidence; missing acknowledgment is not proof of failure. Derived expiry is not an observed event.</p>';
 }
 reportHTML='<h1>'+escapeHTML(title)+'</h1><div class="report-warning"><strong>SAMPLE REPORT — NOT AUDIT EVIDENCE</strong><br>Fictional records only. No production data queried, policy evaluated or completeness verified.</div><dl><dt>Report reference</dt><dd>'+id+'</dd><dt>Organization</dt><dd>Northstar Labs · fictional</dd><dt>Prepared by</dt><dd>'+escapeHTML(author)+' · user-entered, not authenticated</dd><dt>Reporting period</dt><dd>'+start+' through '+end+' inclusive · UTC dates · requested scope only</dd><dt>Generated</dt><dd>'+generated+' · browser clock</dd><dt>View / scope</dt><dd>'+escapeHTML(active+' / '+scope)+'</dd><dt>Evidence snapshot</dt><dd>26 Oct 2026, 10:30 · illustrative fixture; not the report-generation time</dd></dl>'+content+'<h2>Limitations and interpretation</h2><ul><li>Partial fictional inventory; unknown memberships and unavailable sources are not negative evidence.</li><li>Group membership is not an access grant. Potential permission is not live reachability or an installed visa.</li><li>Publication and activation are distinct. Policy comparisons do not replay historical attributes.</li><li>Requested report dates are metadata only in this prototype; sample events are not re-queried or date-filtered.</li><li>This editable HTML has no signature, integrity verification, durable retention or chain-of-custody guarantee.</li></ul><footer>Control Room Audit prototype · Confidentiality/retention classification and signed evidence export: Not Implemented.</footer>';
 const holder=$('report-document');holder.innerHTML=reportHTML;holder.querySelectorAll('button').forEach(b=>b.remove());reportHTML=holder.innerHTML;
 reportReturnFocus='report-button';
 $('detail').close();document.body.classList.add('reporting');$('report-preview').classList.remove('hidden');$('report-back').focus();
}
function tab(name){
 const valid=['access','policy','assertions','history','reports','inventory'];
 if(!valid.includes(name))name='access';
 document.querySelectorAll('main > section').forEach(s=>s.classList.toggle('hidden',s.id!==name));
 document.querySelectorAll('[data-tab]').forEach(b=>{b.classList.toggle('selected',b.dataset.tab===name);if(b.dataset.tab===name)b.setAttribute('aria-current','page');else b.removeAttribute('aria-current')});
 if(location.hash!=='#'+name)history.replaceState(null,'','#'+name);
 document.querySelector('.prototype').classList.toggle('hidden',name==='inventory');
 document.querySelector('.title-row .pill').classList.toggle('hidden',name==='inventory');
 document.querySelector('header .env').textContent=name==='inventory'?'IMPORTED SNAPSHOT':'NORTHSTAR · SAMPLE';
 document.querySelector('aside').classList.toggle('hidden',name==='inventory');
 document.querySelector('.layout').classList.toggle('inventory-layout',name==='inventory');
 document.querySelector('main > footer').classList.toggle('hidden',name==='inventory');
}
function supportedPolicyComparison(){return $('before').value==='r41'&&$('after').selectedIndex===0}
function renderPolicy(){
 const supported=supportedPolicyComparison();
 $('policy-results').classList.toggle('hidden',!supported);
 $('policy-scope').textContent=supported?'Displayed comparison: r41 → r42 · fixed fictional inputs · partial activation evidence, not universal enforcement.':'No comparison available for '+$('before').value+' → '+$('after').value+'. Arbitrary revision analysis is Not Implemented. No impact or activation evidence is inferred for this selection.';
}
function accessContext(){
 const cohort=$('cohort').value;
 if(direction==='service')return '26 Oct 2026, 10:30 · users, groups and machines · per-row sample contexts. Group candidates are not member-wide grants.';
 if(groups.includes($('target').value))return '26 Oct 2026, 10:30 · group-related service candidates; remaining constraints must be evaluated per member/request.';
 return '26 Oct 2026, 10:30 · '+$('target').value+' · '+cohort+' · '+(cohort==='Device not selected'?'device/session unspecified; no unconditional permission inferred.':cohort==='Trusted source unavailable'?'trusted evidence unavailable; no negative conclusion inferred.':'per-service fixture contexts; no live session or source verification.');
}
function setDirection(value){direction=value;document.querySelectorAll('[data-direction]').forEach(b=>b.classList.toggle('selected',b.dataset.direction===value));$('target-caption').textContent=value==='service'?'Service':'Subject';if(value==='service')selectOptions('target',services);else subjectOptions();selectOptions('cohort',['Selected sample request','Device not selected','Trusted source unavailable']);$('group-filter-label').classList.toggle('hidden',value!=='service');$('candidate-kind-label').classList.toggle('hidden',value!=='service');$('candidate-kind').value='all';$('group-filter').value='all';$('outcome').value='all';$('candidate-search').value='';renderAccess()}
const categoryNames={user:'Users',group:'Groups',machine:'Machines'};
let categoryPages={},candidatePage=0,candidateScope='';
function subjectOptions(){
 const kind=$('subject-category').value;
 selectOptions('target',kind==='group'?groups:Object.keys(memberships).filter(name=>memberships[name].id.startsWith(kind+':')));
}
function selectSubject(name){
 $('subject-category').value=groups.includes(name)?'group':memberships[name].id.split(':')[0];
 subjectOptions();$('target').value=name;
}
function categoryReport(rows){
 return Object.entries(categoryNames).map(([kind,name])=>{
  const records=rows.filter(r=>r[1].startsWith(kind+':'));
  return '<section><h3>'+name+' · '+records.length+' records</h3>'+reportTable(['Entity / reference','Outcome / relationship','Reason / constraints'],records.map(r=>[r[0]+' / '+r[1],labels[r[2]],r[3]]))+'</section>';
 }).join('');
}
function renderCategoryResults(filtered,size){
 $('category-results').innerHTML=Object.entries(categoryNames).map(([kind,name])=>{
  const rows=filtered.filter(({r})=>r[1].startsWith(kind+':')),total=currentRows.filter(r=>r[1].startsWith(kind+':')).length;
  const pages=Math.ceil(rows.length/size),page=Math.max(0,Math.min(categoryPages[kind]||0,pages-1));categoryPages[kind]=page;
  const stats=Object.entries(labels).map(([key,label])=>label+': '+rows.filter(({r})=>r[2]===key).length).join(' · ');
  return '<section class="panel" data-category="'+kind+'"><h3>'+name+' · '+rows.length+' of '+total+' match</h3><p>'+escapeHTML(stats)+'</p><div class="table-wrap"><table><thead><tr><th>Entity / reference</th><th>Outcome / relationship</th><th>Reason / constraints</th><th>Inspect</th></tr></thead><tbody>'+
   (rows.length?rows.slice(page*size,(page+1)*size).map(({r,i})=>'<tr><td><strong>'+escapeHTML(r[0])+'</strong><small>'+escapeHTML(r[1])+'</small></td><td>'+escapeHTML(labels[r[2]])+'</td><td>'+escapeHTML(r[3])+'</td><td><button data-explain="'+i+'">Explain</button> <button data-membership="'+escapeHTML(r[0])+'">'+(kind==='group'?'Members':'Membership')+'</button></td></tr>').join(''):'<tr><td colspan="4">No matching '+name.toLowerCase()+'. No absence of access inferred.</td></tr>')+
   '</tbody></table></div><div class="row"><span>'+(pages?'Page '+(page+1)+' of '+pages:'No matches')+'</span><button data-category-page="'+kind+'" data-step="-1" '+(page===0?'disabled':'')+'>Previous '+name.toLowerCase()+'</button><button data-category-page="'+kind+'" data-step="1" '+(page>=pages-1?'disabled':'')+'>Next '+name.toLowerCase()+'</button></div></section>';
 }).join('');
}
function filteredCandidates(){
 const term=$('candidate-search').value.trim().toLowerCase();
 const rows=currentRows.map((r,i)=>({r,i})).filter(({r})=>(direction!=='service'||$('candidate-kind').value==='all'||r[1].startsWith($('candidate-kind').value+':'))&&($('outcome').value==='all'||r[2]===$('outcome').value)&&(!term||(r[0]+' '+r[1]).toLowerCase().includes(term)));
 const order=$('candidate-sort').value;
 return rows.sort((a,b)=>{
  if(order==='outcome'){const status=labels[a.r[2]].localeCompare(labels[b.r[2]]);if(status)return status}
  return (order==='reverse'?-1:1)*a.r[0].localeCompare(b.r[0]);
 });
}
function renderAccess(){
 $('subject-category-label').classList.toggle('hidden',direction==='service');
 const target=$('target').value,cohort=$('cohort').value,old=$('access-revision').selectedIndex===1;
 $('cohort-label').classList.toggle('hidden',direction==='service'||groups.includes(target));
 if(direction==='service'){
  currentRows=serviceCandidateRows(target,old);
  $('access-heading').textContent='Who could access '+target+'?';$('entity-column').textContent='User / group / machine';
 }else{
  currentRows=groups.includes(target)?groupServiceRows(target,old):identityServiceRows(target,cohort,old);
  $('access-heading').textContent='What could '+target+' access?';$('entity-column').textContent='Service';
 }
 const group=$('group-filter').value;
 $('access-context').textContent=accessContext();
 if(direction==='service'&&group!=='all')currentRows=currentRows.filter(r=>{const m=memberships[r[0]];return r[1].startsWith('group:')?r[0]===group:m&&(group==='unknown'?m.unavailable:!m.unavailable&&m.groups.some(g=>g[0]===group))});
 $('membership-summary').innerHTML=direction==='identity'?'<strong>Selected identity groups:</strong> '+(cohort==='Trusted source unavailable'?'Unavailable for this selected context':groupLinks(target))+' <button class="link" data-membership="'+escapeHTML(target)+'">Inspect sample membership</button>':'<strong>Membership scope:</strong> '+escapeHTML(group==='all'?'All sample groups':group==='unknown'?'Unavailable membership evidence':group)+' · direct and source-reported nested memberships. User and machine authorities are separate. Filtering changes the candidate cohort, not policy.';
 if(direction==='identity'&&groups.includes(target))$('membership-summary').innerHTML='<button class="link" data-group="'+escapeHTML(target)+'">Inspect '+escapeHTML(target)+' members</button> · Group-related candidates only; not a grant to all members.';
 $('coverage').textContent='SAMPLE · '+(direction==='service'?'Counts kept separate by category':'Services for selected subject')+' · '+$('access-revision').value+' · no live evaluation';
 const countRows=direction==='service'?currentRows.filter(r=>r[1].startsWith($('candidate-kind').value+':')):currentRows;
 $('access-stats').innerHTML=Object.entries(labels).map(([k,v])=>'<div class="stat"><strong>'+countRows.filter(r=>r[2]===k).length+'</strong><span>'+v+'</span></div>').join('');
 const filtered=filteredCandidates();
 const scope=JSON.stringify([direction,target,cohort,old,group,$('candidate-kind').value,$('outcome').value,$('candidate-search').value,$('candidate-sort').value,$('candidate-size').value]);
 if(scope!==candidateScope){candidatePage=0;categoryPages={};candidateScope=scope}
 const size=Number($('candidate-size').value),pages=Math.ceil(filtered.length/size);
 candidatePage=Math.max(0,Math.min(candidatePage,pages-1));
 const pageRows=filtered.slice(candidatePage*size,(candidatePage+1)*size);
 const separated=direction==='service'&&$('candidate-kind').value==='all';
 $('category-results').classList.toggle('hidden',!separated);
 $('access-stats').classList.toggle('hidden',separated);
 $('access-rows').closest('.panel').classList.toggle('hidden',separated);
 $('page-summary').parentElement.classList.toggle('hidden',separated);
 if(separated){
  $('access-rows').innerHTML='';
  $('search-count').textContent='Results and counts are independent for users, groups and machines.';
  renderCategoryResults(filtered,size);return;
 }
 $('search-count').textContent=filtered.length+' of '+countRows.length+' candidates match in this category. Counts below describe this category before search/outcome filters.';
 $('page-summary').textContent=filtered.length?'Page '+(candidatePage+1)+' of '+pages+' · '+(candidatePage*size+1)+'–'+Math.min((candidatePage+1)*size,filtered.length)+' of '+filtered.length+' matches':'No matching candidates · this does not prove absence of access.';
 $('page-previous').disabled=candidatePage===0;
 $('page-next').disabled=candidatePage>=pages-1;
 $('access-rows').innerHTML=filtered.length?pageRows.map(({r,i})=>'<tr><td><strong>'+escapeHTML(r[0])+'</strong><small>'+escapeHTML(r[1])+'</small></td><td>'+(direction==='service'?groupLinks(r[0])+'<br><button class="link" data-membership="'+escapeHTML(r[0])+'">Membership details</button>':'<small>Selected identity groups shown above</small>')+'</td><td><span class="status '+r[2]+'">'+labels[r[2]]+'</span></td><td>'+escapeHTML(r[3])+'</td><td><button class="link" data-explain="'+i+'">Explain</button><br><button class="link" data-related="'+i+'">History</button></td></tr>').join(''):'<tr><td colspan="5">No sample results match this filter.</td></tr>';
}
function showHistory(kind,ref){$('history-kind').value=kind;$('history-ref').value=ref;renderHistory();tab('history')}
function renderHistory(){
 const kind=$('history-kind').value,ref=$('history-ref').value.trim()||'No reference selected';
 $('history-heading').textContent=kind+' · '+ref;
 const visa=kind==='Visa';
 const events=visa?[['10:32:04','Decision permitted','Policy r42 · Alice on finance-laptop-17 → Finance Reports',false],['10:32:04','Visa issued','visa-demo-7f23 · 60-second validity',false],['10:32:05','Destination installation acknowledged','adapter:reports-01 · event arrived 2 seconds late',false],['10:32:05','Source installation not observed','Missing acknowledgment is not proof of installation failure.',true],['10:33:04','Validity ended · derived fact','Calculated from sample expiry; not an observed expiry event.',true]]:kind==='Policy'?[['08:45:00','Revision saved and validated','r42 · Morgan Lee · synthetic review evidence',false],['09:10:00','Revision published','Review CR-128 · Sam Rivera',false],['09:12:00','Activation partially acknowledged','2 of 3 acknowledgments observed; remaining scope unknown.',true]]:[['10:30:00','Selected entity snapshot','Sample inventory reference only; not evidence of a live session.',false],['10:32:04','Related decision observed','Example correlation: policy r42 · visa-demo-7f23',false],['10:32:05','Evidence gap','Only illustrative linked events are included in this preview.',true]];
 $('history-events').innerHTML=events.map(e=>'<div class="event '+(e[3]?'gap':'')+'"><time>26 Oct · '+e[0]+'</time><strong>'+e[1]+'</strong><p>'+e[2]+'</p><button class="link" data-ni="Retrieve authoritative event evidence">Inspect evidence · Not Implemented</button></div>').join('');
 $('history-context').innerHTML='<p><strong>Selection:</strong> '+escapeHTML(kind)+' / '+escapeHTML(ref)+'</p><p>Policy: <button class="link" id="context-policy">r42 · Corporate service access</button></p><p>Service: Finance Reports<br>Scenario identity: Alice Chen<br>Machine: finance-laptop-17</p><div class="note">Illustrative links, not resolved from the selected entity. Time filter: '+escapeHTML($('history-window').value)+'.</div>';
}
document.addEventListener('click',e=>{
 const contentsLink=e.target.closest('#report-document a[href^="#pack-"]');
 if(contentsLink){e.preventDefault();$(contentsLink.getAttribute('href').slice(1)).scrollIntoView();return}
 const b=e.target.closest('button');if(!b)return;
 if(b.dataset.tab)tab(b.dataset.tab);
 if(b.dataset.categoryPage){categoryPages[b.dataset.categoryPage]+=Number(b.dataset.step);renderAccess()}
 if(b.dataset.assertion){selectedAssertion=b.dataset.assertion;renderAssertions()}
 if(b.dataset.direction)setDirection(b.dataset.direction);
 if(b.dataset.ni)notImplemented(b.dataset.ni);
 if(b.dataset.template)chooseTemplate(b.dataset.template);
 if(b.dataset.packPreview!==undefined)previewTemplate(reportPack[Number(b.dataset.packPreview)]);
 if(b.dataset.packTemplate!==undefined){
  const item=reportPack[Number(b.dataset.packTemplate)];
  if(item.key==='identity')$('template-category').value=memberships[item.scope].id.split(':')[0];
  $('template-search').value='';chooseTemplate(item.key);$('template-scope').value=item.scope;$('template-scope').focus();
  $('pack-feedback').textContent='Selected the template and scope for report '+(Number(b.dataset.packTemplate)+1)+'. The existing report is unchanged.';
 }
 if(b.dataset.packRemove!==undefined){reportPack.splice(Number(b.dataset.packRemove),1);renderPack()}
 if(b.dataset.packDown!==undefined){const i=Number(b.dataset.packDown);if(i<reportPack.length-1){[reportPack[i+1],reportPack[i]]=[reportPack[i],reportPack[i+1]];renderPack()}}
 if(b.dataset.packUp!==undefined){const i=Number(b.dataset.packUp);if(i>0){[reportPack[i-1],reportPack[i]]=[reportPack[i],reportPack[i-1]];renderPack()}}
 if(b.dataset.membership){if(groups.includes(b.dataset.membership))showGroup(b.dataset.membership);else showMembership(b.dataset.membership)}
 if(b.dataset.group)showGroup(b.dataset.group);
 if(b.dataset.member){$('detail').close();setDirection('identity');selectSubject(b.dataset.member);renderAccess();tab('access')}
 if(b.dataset.groupService){const group=b.dataset.groupScope;$('detail').close();setDirection('service');$('target').value=b.dataset.groupService;$('group-filter').value=group;renderAccess();tab('access')}
 if(b.dataset.explain!==undefined){const r=currentRows[Number(b.dataset.explain)];dialog('Sample explanation · '+r[0],'<p><span class="status '+r[2]+'">'+labels[r[2]]+'</span></p><p>'+escapeHTML(r[3])+'</p><ul class="drawer-evidence"><li>Policy: '+escapeHTML($('access-revision').value)+'</li><li>Organization: Northstar Labs</li><li>Context: '+escapeHTML(accessContext())+'</li><li>Evidence: fictional fixture; no attributes fetched</li></ul><p class="note">Real rule traces and dependency freshness are Not Implemented. This is not an engine-generated decision.</p>')}
 if(b.dataset.related!==undefined){const r=currentRows[Number(b.dataset.related)];if(r[1].startsWith('group:'))showGroup(r[0]);else showHistory(direction==='identity'?'Service':r[1].startsWith('machine:')?'Machine':'User',r[1].split(' · ')[0])}
 if(b.id==='context-policy')tab('policy');
});
$('close-detail').onclick=()=>$('detail').close();
$('inventory-file').onchange=importInventory;
$('baseline-file').onchange=importBaseline;
$('baseline-clear').onclick=clearBaseline;
['baseline-system','baseline-reference','baseline-retrieved'].forEach(id=>$(id).addEventListener(id==='baseline-system'?'change':'input',updateBaselineProvenance));
$('comparison-search').oninput=updateComparisonReview;
$('comparison-kind').onchange=updateComparisonReview;
$('comparison-case').onchange=updateComparisonReview;
$('comparison-reset').onclick=resetComparisonReview;
$('comparison-page-previous').onclick=()=>selectComparisonPosition(comparisonMatches[(comparisonPage-1)*comparisonPageSize].line);
$('comparison-page-next').onclick=()=>selectComparisonPosition(comparisonMatches[(comparisonPage+1)*comparisonPageSize].line);
$('comparison-previous').onclick=()=>moveComparisonPosition(-1);
$('comparison-next').onclick=()=>moveComparisonPosition(1);
$('comparison-lines').onclick=event=>{
 const button=event.target.closest('[data-comparison-select]');
 if(button)selectComparisonPosition(Number(button.dataset.comparisonSelect));
};
$('comparison-go').onclick=()=>{
 try{
  const position=sourceLineNumber($('comparison-position').value,Math.max(sourceReviewLines(baselineInventory.data.settings.source).length,sourceReviewLines(importedInventory.data.settings.source).length));
  selectComparisonPosition(position);
 }catch(error){$('comparison-error').textContent=error.message}
};
$('comparison-note').onclick=addComparisonReference;
$('source-search').oninput=updateSourceSearch;
$('source-case').onchange=updateSourceSearch;
$('source-previous').onclick=()=>moveSourceMatch(-1);
$('source-next').onclick=()=>moveSourceMatch(1);
$('source-reset').onclick=()=>{$('source-search').value='';$('source-case').value='insensitive';updateSourceSearch()};
$('source-go').onclick=()=>{
 try{
  const line=sourceLineNumber($('source-line').value,sourceReviewLines(importedInventory.data.settings.source).length);
  selectSourceLine(line);$('source-error').textContent='';
 }catch(error){$('source-error').textContent=error.message}
};
$('source-note').onclick=()=>{
 const record=importedInventory;
 const reference='Source line '+selectedSourceLine+' · settings revision '+record.data.settings.revision+' · organization '+record.data.organization_id+' · file '+record.file+' (unverified reviewer reference)';
 const notes=$('inventory-notes'),value=notes.value+(notes.value?'\n':'')+reference;
 if(value.length>4000){$('source-error').textContent='The line reference would exceed the 4000-character notes limit.';return}
 notes.value=value;$('source-error').textContent='';notes.focus();
};
$('inventory-clear').onclick=()=>{clearInventory();$('inventory-file').value='';$('inventory-error').textContent=''};
$('help-button').onclick=openPageHelp;
document.querySelectorAll('[data-help-close]').forEach(button=>button.onclick=()=>$('audit-help').close());
$('audit-help').addEventListener('close',()=>$('help-button').focus());
$('assertion-search').oninput=renderAssertions;
$('assertion-role').onchange=renderAssertions;
$('assertion-reset').onclick=()=>{
 $('assertion-search').value='';$('assertion-role').value='all';renderAssertions();
};
$('assertion-add').onclick=()=>{
 const assertion=currentAssertion();
 const added=addPackScopes([assertion.name],'assertion');
 $('assertion-feedback').textContent=added?'Added '+assertion.name+' to the report pack. Open Reports to review it.':assertion.name+' is already in the report pack.';
};
$('assertion-policy').onclick=()=>{
 $('before').value='r41';$('after').selectedIndex=0;renderPolicy();tab('policy');
};
$('assertion-access').onclick=()=>{
 const assertion=currentAssertion();
 setDirection('service');$('target').value=assertion.service;$('access-revision').selectedIndex=0;renderAccess();tab('access');
};
$('report-button').onclick=reportSetup;
$('candidate-search').oninput=renderAccess;
$('candidate-kind').onchange=renderAccess;
$('subject-category').onchange=()=>{subjectOptions();renderAccess()};
$('candidate-sort').onchange=renderAccess;
$('candidate-size').onchange=renderAccess;
$('page-previous').onclick=()=>{candidatePage--;renderAccess()};
$('page-next').onclick=()=>{candidatePage++;renderAccess()};
$('browse-groups').onclick=browseGroups;
$('access-reset').onclick=resetAccessFilters;
$('template-search').oninput=renderCatalog;
$('template-preview').onclick=()=>previewTemplate({key:selectedTemplate,scope:$('template-scope').value});
$('template-category').onchange=renderTemplateScopes;
$('pack-add-all').onclick=()=>addPackScopes(availableTemplateScopes());
$('pack-remove-template').onclick=()=>{
 const key=selectedTemplate,count=reportPack.filter(r=>r.key===key).length;
 dialog('Remove template reports?','<p>Remove '+count+' reports for '+escapeHTML(reportTemplates[key].name)+'? Other templates and reporting details will be retained.</p><button id="pack-remove-template-confirm" data-remove-key="'+key+'">Remove these reports</button>');
};
document.addEventListener('click',e=>{
 const button=e.target.closest('#pack-remove-template-confirm');if(!button)return;
 const key=button.dataset.removeKey;
 reportPack=reportPack.filter(r=>r.key!==key);renderPack();$('detail').close();
 $('pack-feedback').textContent='Removed reports for '+reportTemplates[key].name+'.';
 $('pack-add').focus();
});
$('pack-clear').onclick=()=>dialog('Clear report pack?','<p>Remove all selected reports? Shared title, dates and preparer will be retained.</p><button id="pack-clear-confirm">Clear selected reports</button>');
document.addEventListener('click',e=>{if(e.target.closest('#pack-clear-confirm')){reportPack=[];renderPack();$('detail').close();$('pack-feedback').textContent='Report pack cleared.'}});
chooseTemplate('group');renderPack();
$('pack-add').onclick=()=>addPackScopes([$('template-scope').value]);
$('pack-preview').onclick=previewPack;
document.addEventListener('click',e=>{if(e.target.closest('#report-build'))buildReport()});
$('report-back').onclick=()=>{document.body.classList.remove('reporting');$('report-preview').classList.add('hidden');$(reportReturnFocus).focus()};
$('report-print').onclick=()=>window.print();
$('report-download').onclick=()=>{
 const styles=[...document.styleSheets].flatMap(sheet=>[...sheet.cssRules].map(rule=>rule.cssText)).join('\n');
 const imported=document.querySelector('[data-tab].selected').dataset.tab==='inventory';
 const html='<!doctype html><html lang="en"><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>'+ (imported?'Imported assertion inventory':'Sample Audit report')+'</title><style>'+styles+'</style><body class="reporting"><section id="report-preview"><article id="report-document">'+reportHTML+'</article></section></body></html>';
 const url=URL.createObjectURL(new Blob([html],{type:'text/html;charset=utf-8'}));const a=document.createElement('a');a.href=url;a.download='zpr-audit-'+(imported?'IMPORTED':'SAMPLE')+'-'+new Date().toISOString().slice(0,10)+'.html';a.click();setTimeout(()=>URL.revokeObjectURL(url),1000);
};
$('show-access').onclick=renderAccess;
['target','cohort','access-revision','outcome','group-filter'].forEach(id=>$(id).onchange=renderAccess);
$('show-history').onclick=renderHistory;
['history-kind','history-window'].forEach(id=>$(id).onchange=renderHistory);
$('context-button').onclick=()=>dialog('Sample analysis context','<p>'+escapeHTML(accessContext())+'</p><p>Organization: Northstar Labs · evaluator not connected. Sample identity links only.</p><p>Changing real context, selecting historical attributes and obtaining fresh source evidence are <strong>Not Implemented</strong>.</p>');
['before','after'].forEach(id=>$(id).onchange=renderPolicy);
$('compare').onclick=()=>{renderPolicy();if(supportedPolicyComparison())dialog('Sample comparison loaded','<p>The displayed r41 → r42 fixture illustrates a source change, partial activation and potential-access differences. No policy analysis ran.</p>');else notImplemented('Compare arbitrary policy revisions')};
$('impact-access').onclick=()=>{setDirection('service');$('target').value='Finance Reports';$('access-revision').selectedIndex=0;renderAccess();tab('access')};
$('policy-history').onclick=()=>showHistory('Policy','corporate-access:r42');
$('history-access').onclick=()=>{setDirection('service');tab('access')};
setDirection('service');renderHistory();renderAssertions();renderPolicy();
tab(location.hash.slice(1));
addEventListener('hashchange',()=>tab(location.hash.slice(1)));
