import {test} from 'node:test';
import assert from 'node:assert/strict';
import http from 'node:http';
import {createHash} from 'node:crypto';
import {configuration,createAuditServer} from '../server.mjs';

const data={organization_id:'real-org',organization_name:'Real organization',settings:{revision:7,source:'group "Finance";',enabled:true,interval_seconds:60}};
async function listen(server){
 await new Promise(resolve=>server.listen(0,'127.0.0.1',resolve));
 return 'http://127.0.0.1:'+server.address().port;
}
async function close(server){server.closeAllConnections();await new Promise(resolve=>server.close(resolve))}

test('operator configuration is explicit, organization-pinned and secure by default',()=>{
 assert.equal(configuration({ZPR_SIMULATOR_URL:'http://invalid.invalid'}),null);
 for(const url of ['http://remote.example','file:///etc/passwd','https://user:secret@example.com','https://example.com/path','https://example.com/?token=secret']){
  assert.throws(()=>configuration({ZPR_AUDIT_POLICY_SERVICE_URL:url,ZPR_AUDIT_ORGANIZATION_ID:'real-org'}));
 }
 assert.throws(()=>configuration({ZPR_AUDIT_POLICY_SERVICE_URL:'https://example.com'}));
 assert.throws(()=>configuration({ZPR_AUDIT_POLICY_SERVICE_URL:'https://example.com',ZPR_AUDIT_ORGANIZATION_ID:'real-org',ZPR_AUDIT_TLS_CERT:'unused'}));
 assert.equal(configuration({ZPR_AUDIT_POLICY_SERVICE_URL:'https://example.com',ZPR_AUDIT_ORGANIZATION_ID:'real-org'}).url.href,'https://example.com/api/assertions/settings');
});

test('adapter serves static UI without a source, never substitutes fixtures',async()=>{
 const server=createAuditServer(),base=await listen(server);
 try{
  assert.equal((await fetch(base+'/')).status,200);
  assert.deepEqual(await (await fetch(base+'/api/audit/source')).json(),{configured:false});
  assert.equal((await fetch(base+'/api/audit/assertion-settings')).status,503);
  assert.equal((await fetch(base+'/api/audit/source',{method:'POST'})).status,405);
  assert.equal((await fetch(base+'/server.mjs')).status,404);
  assert.equal((await fetch(base+'/api/audit/source',{headers:{Origin:'https://evil.example'}})).status,403);
  const rebound=await new Promise((resolve,reject)=>{
   http.get(base+'/api/audit/source',{headers:{Host:'evil.example:123'}},response=>{
    response.resume();resolve(response.statusCode);
   }).on('error',reject);
  });
  assert.equal(rebound,403);
 }finally{await close(server)}
});

test('read-only production contract, provenance, redaction and fail-closed responses',async()=>{
 let status=200,body=JSON.stringify({...data,last_run:{secret:'not settings evidence'}});
 let requests=0;
 const upstream=http.createServer((req,res)=>{
  requests++;
  assert.equal(req.method,'GET');assert.equal(req.url,'/api/assertions/settings');
  assert.equal(req.headers.authorization,undefined);assert.equal(req.headers.cookie,undefined);
  res.writeHead(status,{'Content-Type':'application/json',Location:'/redirect-secret'});
  res.end(body);
 });
 const url=await listen(upstream);
 const server=createAuditServer({config:configuration({ZPR_AUDIT_POLICY_SERVICE_URL:url,ZPR_AUDIT_ORGANIZATION_ID:'real-org',ZPR_SIMULATOR_URL:'http://invalid.invalid'})});
 const base=await listen(server);
 try{
  assert.equal(requests,0);
  const response=await fetch(base+'/api/audit/assertion-settings',{headers:{Cookie:'browser-secret'}});
  assert.equal(response.status,200);assert.equal(response.headers.get('cache-control'),'no-store');
  const evidence=await response.json();
  assert.deepEqual(evidence.snapshot,data);
  assert.equal(evidence.response_sha256,createHash('sha256').update(body).digest('hex'));
  assert.equal(evidence.source_endpoint,url+'/api/assertions/settings');
  assert.ok(Number.isFinite(Date.parse(evidence.retrieved_at)));
  for(const invalid of [
   {...data,organization_id:'another-org'},
   {...data,error:'sensitive error'},
   {...data,settings_error:'failed to load'},
   {...data,scope:'machine'},
   {...data,settings:{...data.settings,revision:-1}},
  ]){
   body=JSON.stringify(invalid);
   const failed=await fetch(base+'/api/audit/assertion-settings');
   assert.equal(failed.status,502);assert.ok(!(await failed.text()).includes('sensitive'));
  }
  for(const malformed of ['{','a'.repeat(256*1024+1)]){
   body=malformed;assert.equal((await fetch(base+'/api/audit/assertion-settings')).status,502);
  }
  body=JSON.stringify(data);
  for(const code of [302,401,403,503]){
   status=code;
   const count=requests;
   assert.equal((await fetch(base+'/api/audit/assertion-settings')).status,502);
   assert.equal(requests,count+1,'redirects are not followed');
  }
  await close(upstream);
  assert.equal((await fetch(base+'/api/audit/assertion-settings')).status,502);
 }finally{await close(server);if(upstream.listening)await close(upstream)}
});
