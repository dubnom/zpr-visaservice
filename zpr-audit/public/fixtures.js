'use strict';

const services=['Finance Reports','Payroll API','Engineering Git'];
const identities=['Alice Chen','Bob Evans','Priya Shah','Jordan Lee','finance-laptop-17','personal-mac-04','finance-kiosk-02','finance-laptop-22','payroll-worker-02'];
const memberships={
 'Alice Chen':{source:'Company directory',id:'user:alice',groups:[['Finance','Direct'],['Employees','Via Finance']]},
 'Bob Evans':{source:'Company directory',id:'user:bob',groups:[['Engineering','Direct'],['Employees','Via Engineering']]},
 'Priya Shah':{source:'Company directory',id:'user:priya',groups:[['Finance','Direct'],['Employees','Via Finance']]},
 'Jordan Lee':{source:'Company directory',id:'user:jordan',groups:[],unavailable:true},
 'finance-laptop-17':{source:'Device registry',id:'machine:fin17',groups:[['Managed devices','Direct']]},
 'personal-mac-04':{source:'Device registry',id:'machine:personal04',groups:[]},
 'finance-kiosk-02':{source:'Device registry',id:'machine:kiosk02',groups:[['Managed devices','Direct']]},
 'finance-laptop-22':{source:'Device registry',id:'machine:fin22',groups:[],unavailable:true},
 'payroll-worker-02':{source:'Device registry',id:'machine:pay02',groups:[['Managed devices','Direct']]},
 'backup-worker-03':{source:'Device registry',id:'machine:backup03',groups:[],unavailable:true},
 'build-runner-08':{source:'Device registry',id:'machine:build08',groups:[['Managed devices','Direct']]}
};
function membershipText(name){const m=memberships[name];return !m?(groups.includes(name)?'Group · member-specific constraints apply':'Not an identity'):m.unavailable?'Unavailable — not an empty membership':m.groups.length?m.groups.map(g=>g[0]+(g[1]==='Direct'?' · direct':' · nested')).join('; '):'No groups in sample snapshot'}
const labels={permitted:'Permitted',denied:'Denied',conditional:'Context required',unknown:'Indeterminate'};
const assertionFixtures=[
 {name:'Finance membership',id:'assertion:finance-member@a3',roleKey:'enforcing',role:'Enforcing · sample intent',definition:'Selected user must be a member of Finance.',policy:'Corporate service access r42 · Finance Reports',service:'Finance Reports',outcomes:'Alice: satisfied; Bob: not satisfied; Jordan: indeterminate.',source:'Company directory · Finance group · sample snapshot 2026-10-26 10:30 UTC',limit:'Jordan membership unavailable. Nested membership follows the source-reported path; no independent traversal or live lookup performed.'},
 {name:'Managed device',id:'assertion:managed-device@a2',roleKey:'enforcing',role:'Enforcing · sample intent',definition:'Selected request must originate from a managed device.',policy:'Corporate service access r42 · Finance Reports',service:'Finance Reports',outcomes:'finance-laptop-17: satisfied; personal-mac-04: not satisfied; Priya without a selected device: context required.',source:'Device registry · managed-device evidence · sample snapshot 2026-10-26 10:30 UTC',limit:'finance-laptop-22 evidence unavailable. Membership in a machine group does not establish user authorization or a live session.'},
 {name:'Directory freshness diagnostic',id:'assertion:directory-freshness@a1',roleKey:'diagnostic',role:'Diagnostic only · sample intent',definition:'Report whether directory evidence freshness can be established.',policy:'Illustrative diagnostic reference alongside r42; does not independently grant or deny access.',service:'Finance Reports',outcomes:'Alice snapshot: observed in fixture; Jordan: indeterminate because the source is unavailable.',source:'Company directory · observation time and source availability; no approved freshness threshold selected',limit:'No freshness verification ran. A diagnostic result must not be presented as an enforcing denial or successful access decision.'}
];
function matchingAssertions(term='',role='all'){
 if(!['all','enforcing','diagnostic'].includes(role))throw new Error('Unknown assertion role: '+role);
 const query=term.trim().toLowerCase();
 return assertionFixtures.filter(a=>(role==='all'||a.roleKey===role)&&
  [a.name,a.id,a.definition,a.policy,a.source].some(value=>value.toLowerCase().includes(query)));
}
const userRows=[
 ['Alice Chen','user:alice · Finance','permitted','Finance member; managed device verified.'],
 ['Bob Evans','user:bob · Engineering','denied','Finance membership requirement not satisfied.'],
 ['Priya Shah','user:priya · Finance','conditional','User membership matches; device context not selected.'],
 ['Jordan Lee','user:jordan · source unavailable','unknown','Directory freshness cannot be established.']
];
const machineRows=[
 ['finance-laptop-17','machine:fin17 · Alice session','permitted','Selected Alice-on-device context satisfies requirements.'],
 ['personal-mac-04','machine:personal04 · Bob session','denied','Device is unmanaged; Finance membership also required.'],
 ['finance-kiosk-02','machine:kiosk02 · no selected user','conditional','Managed machine; authenticated user context required.'],
 ['finance-laptop-22','machine:fin22 · stale device evidence','unknown','Device authority is unavailable.']
];
function serviceRequestRows(service,old=false){
 if(service==='Finance Reports')return [...userRows,...machineRows].map(r=>{
  const row=[...r];
  if(old&&row[0]==='Bob Evans'){row[2]='permitted';row[3]='Employee membership matched under r41 in the same managed-device context.'}
  if(old&&row[0]==='Priya Shah'){row[2]='permitted';row[3]='Employee membership matched under r41; this rule did not require device evidence.'}
  return row;
 });
 if(service==='Payroll API')return [['payroll-worker-02','machine:pay02 · service identity','permitted','Selected payroll workload principal is authorized; not all users on this machine.'],['Alice Chen','user:alice · Finance','denied','Interactive users are not permitted to call this API.'],['backup-worker-03','machine:backup03 · source unavailable','unknown','Machine identity evidence unavailable.']];
 if(service==='Engineering Git')return [['Bob Evans','user:bob · Engineering','permitted','Engineering membership matches in the selected managed-device/VPN context.'],['Alice Chen','user:alice · Finance','denied','Engineering membership required.'],['build-runner-08','machine:build08','conditional','Select the workload principal for this machine.']];
 throw new Error('Unknown sample service: '+service);
}
function identityServiceRows(identity,context='Selected sample request',old=false){
 return services.map((service,i)=>{
  const sample=serviceRequestRows(service,old).find(r=>r[0]===identity);
  let status=sample?sample[2]:'unknown',reason=sample?sample[3]:'No request fixture for this identity/service. Not evaluated; no denial inferred.';
  if(context==='Device not selected'){status='conditional';reason='Device/session context must be selected; no unconditional access conclusion.'}
  if(context==='Trusted source unavailable'){status='unknown';reason='Trusted source evidence is unavailable in this selected context.'}
  return [service,'service:'+['reports','payroll','git'][i],status,reason];
 });
}
const groups=['Finance','Engineering','Employees','Managed devices'];
const accessSubjects=[...Object.keys(memberships),...groups];
function groupServiceRows(group,old=false){
 if(!groups.includes(group))throw new Error('Unknown sample group: '+group);
 return services.map((service,i)=>{
  let status='unknown',reason='No group-policy relationship in this fixture. Not evaluated; no denial inferred.';
  if(service==='Finance Reports'){
   status='conditional';
   reason=group==='Finance'?(old?'Source-reported Employees membership contributes to r41; request and source constraints remain.':'Finance membership contributes to r42; device and session constraints remain.'):
    group==='Employees'?(old?'Employee membership contributes to r41; request and source constraints remain.':'Employee membership alone does not meet r42; Finance and device requirements remain.'):
    group==='Managed devices'?'Device condition only; authorized user or workload context still required.':'Engineering membership alone does not meet Finance requirements; evaluate each request.';
  }else if(service==='Engineering Git'&&(group==='Engineering'||group==='Employees'||group==='Managed devices')){
   status='conditional';reason='Engineering membership and applicable user/workload context must be established per request.';
  }else if(service==='Payroll API'&&group==='Managed devices'){
   status='conditional';reason='Managed device alone is insufficient; separate payroll workload authorization required.';
  }
  return [service,'service:'+['reports','payroll','git'][i],status,reason];
 });
}
function serviceCandidateRows(service,old=false){
 return [...serviceRequestRows(service,old),...groups.map(group=>{
  const row=groupServiceRows(group,old).find(r=>r[0]===service);
  return [group,'group:'+group,row[2],row[3]];
 })];
}
