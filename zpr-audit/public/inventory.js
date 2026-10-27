'use strict';

const maxInventoryBytes=256*1024;
function compareAssertionSnapshots(baseline,current){
 if(baseline.organization_id!==current.organization_id)throw new Error('Organization IDs differ. Cross-organization comparison is not allowed.');
 const fields=['revision','enabled','interval_seconds'].map(key=>({
  field:key,baseline:baseline.settings[key],current:current.settings[key],
  changed:baseline.settings[key]!==current.settings[key]
 }));
 const before=sourceReviewLines(baseline.settings.source),after=sourceReviewLines(current.settings.source);
 const sourceChanged=baseline.settings.source!==current.settings.source;
 const contentChanged=sourceChanged||fields.some(field=>field.field!=='revision'&&field.changed);
 const revisionDelta=current.settings.revision-baseline.settings.revision;
 const status=revisionDelta<0?'Reversed revision order':revisionDelta===0?
  (contentChanged?'Same-revision conflict':'Same revision and settings'):
  (contentChanged?'Settings differ across revisions':'Revision-only settings change');
 const bytes={baseline:new TextEncoder().encode(baseline.settings.source).byteLength,current:new TextEncoder().encode(current.settings.source).byteLength};
 const lines=[],counts={changed:0,added:0,removed:0,unchanged:0};
 for(let i=0;i<Math.max(before.length,after.length);i++){
  const kind=before[i]===after[i]?'unchanged':i>=before.length?'added':i>=after.length?'removed':'changed';
  counts[kind]++;
  if(kind!=='unchanged')lines.push({line:i+1,baseline:before[i]??null,current:after[i]??null,kind});
 }
 const warning=baseline.settings.revision>current.settings.revision?
  'Baseline revision is newer than current. Selection order is not proof of chronological history.':
  baseline.settings.revision===current.settings.revision&&
   (fields.some(field=>field.changed)||baseline.settings.source!==current.settings.source)?
   'Same revision contains different settings or source. Resolve this inconsistency with the source owner; no trustworthy change history is inferred.':'Revision order alone does not establish publication, activation or historical validity.';
 return {fields,lines,counts,sourceChanged,contentChanged,revisionDelta,status,
  sourceSize:{bytes,lines:{baseline:before.length,current:after.length}},
  nameChanged:baseline.organization_name!==current.organization_name,warning};
}
function filterComparisonLines(lines,query,kind='all',matchCase=false){
 const needle=matchCase?query:query.toLowerCase();
 return lines.filter(line=>(kind==='all'||line.kind===kind)&&
  [line.baseline,line.current].some(text=>text!==null&&(matchCase?text:text.toLowerCase()).includes(needle)));
}
function sourceReviewLines(source){
 if(!source)return [];
 return source.split(/\r\n|\n|\r/);
}
function matchingSourceLines(source,query,matchCase=false){
 if(!query)return [];
 const needle=matchCase?query:query.toLowerCase();
 return sourceReviewLines(source).flatMap((line,index)=>(matchCase?line:line.toLowerCase()).includes(needle)?[index+1]:[]);
}
function sourceLineNumber(value,count){
 if(!/^[1-9]\d*$/.test(value)||!Number.isSafeInteger(Number(value))||Number(value)>count)throw new Error('Enter a line number from 1 through '+count+'.');
 return Number(value);
}
function validateInventoryProvenance(provenance,importedAt){
 if(!['unspecified','policy-service','control-room'].includes(provenance.system))throw new Error('Choose a supported source system.');
 if(provenance.reference.length>1000)throw new Error('Source reference exceeds the 1000-character limit.');
 const retrieved=provenance.retrieved.trim();
 if(retrieved){
  if(!/^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}Z$/.test(retrieved)||!Number.isFinite(Date.parse(retrieved))||new Date(retrieved).toISOString().slice(0,19)+'Z'!==retrieved)throw new Error('Retrieval time must be a valid UTC timestamp: YYYY-MM-DDTHH:mm:ssZ.');
  if(Date.parse(retrieved)>Date.parse(importedAt))throw new Error('Retrieval time cannot be after this snapshot was imported.');
 }
 return {system:provenance.system,reference:provenance.reference.trim(),retrieved};
}
function validateInventoryReview(review,importedAt){
 const provenance=validateInventoryProvenance(review,importedAt);
 if(!review.preparer.trim()||!review.purpose.trim())throw new Error('Enter a preparer and report purpose.');
 if(review.preparer.length>200||review.purpose.length>1000||review.notes.length>4000)throw new Error('Review fields exceed their allowed lengths.');
 return {...review,...provenance,preparer:review.preparer.trim(),purpose:review.purpose.trim(),notes:review.notes.trim()};
}
function parseAssertionInventory(text){
 const data=JSON.parse(text);
 if(!data||typeof data!=='object'||Array.isArray(data))throw new Error('Expected an assertion-settings response object.');
 if(Object.hasOwn(data,'error'))throw new Error('The file is a service-error response, not an assertion-settings export. Obtain a successful authorized response after the source is available.');
 if(Object.hasOwn(data,'settings_error')&&(typeof data.settings_error!=='string'||data.settings_error.trim()))throw new Error('The source reports an assertion-settings load failure. Cached or unavailable settings cannot be imported as a successful snapshot.');
 if(Object.hasOwn(data,'scope')&&data.scope!=='organization')throw new Error('Only organization-scoped assertion settings are supported.');
 const settings=data.settings;
 if(typeof data.organization_id!=='string'||!data.organization_id.trim())throw new Error('organization_id is required.');
 if(typeof data.organization_name!=='string'||!data.organization_name.trim())throw new Error('organization_name is required; Audit does not infer it from a directory or Simulator.');
 if(!settings||typeof settings!=='object'||Array.isArray(settings))throw new Error('settings is required.');
 if(!Number.isSafeInteger(settings.revision)||settings.revision<0)throw new Error('settings.revision must be a non-negative integer.');
 if(typeof settings.source!=='string'||new TextEncoder().encode(settings.source).byteLength>65536)throw new Error('settings.source must be a string of at most 64 KiB UTF-8.');
 if(typeof settings.enabled!=='boolean')throw new Error('settings.enabled must be boolean.');
 if(!Number.isSafeInteger(settings.interval_seconds)||settings.interval_seconds<0)throw new Error('settings.interval_seconds must be a non-negative integer.');
 return {
  organization_id:data.organization_id,organization_name:data.organization_name,
  settings:{revision:settings.revision,source:settings.source,enabled:settings.enabled,interval_seconds:settings.interval_seconds}
 };
}
