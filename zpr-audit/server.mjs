import http from 'node:http';
import https from 'node:https';
import {readFileSync} from 'node:fs';
import {readFile} from 'node:fs/promises';
import {fileURLToPath} from 'node:url';
import {createHash} from 'node:crypto';
import {runInNewContext} from 'node:vm';

const publicRoot=new URL('./public/',import.meta.url);
const {parseAssertionInventory,maxInventoryBytes}=runInNewContext(
 await readFile(new URL('inventory.js',publicRoot),'utf8')+'\n({parseAssertionInventory,maxInventoryBytes})',{TextEncoder});
const assets=new Map([
 ['/', ['index.html','text/html']],['/index.html',['index.html','text/html']],
 ['/help.html',['help.html','text/html']],['/app.css',['app.css','text/css']],
 ...['app','inventory','fixtures','ui'].map(name=>['/'+name+'.js',[name+'.js','text/javascript']]),
 ['/shared/palette.css',['shared/palette.css','text/css']],
]);

export function configuration(env=process.env){
 const endpoint=env.ZPR_AUDIT_POLICY_SERVICE_URL;
 if(!endpoint)return null;
 const url=new URL(endpoint);
 if(!['http:','https:'].includes(url.protocol)||url.username||url.password||url.search||url.hash||url.pathname!=='/'){
  throw new Error('ZPR_AUDIT_POLICY_SERVICE_URL must be an HTTP(S) origin without credentials or a path.');
 }
 if(url.protocol==='http:'&&!['127.0.0.1','[::1]','localhost'].includes(url.hostname)){
  throw new Error('Remote Policy-Service requires HTTPS; HTTP is supported only on loopback.');
 }
 const organization=env.ZPR_AUDIT_ORGANIZATION_ID?.trim();
 if(!organization)throw new Error('ZPR_AUDIT_ORGANIZATION_ID is required when a Policy-Service is configured.');
 if(Boolean(env.ZPR_AUDIT_TLS_CERT)!==Boolean(env.ZPR_AUDIT_TLS_KEY))throw new Error('Configure both ZPR_AUDIT_TLS_CERT and ZPR_AUDIT_TLS_KEY.');
 const tls={};
 for(const [key,name] of [['ca','CA'],['cert','CERT'],['key','KEY']]){
  if(env['ZPR_AUDIT_TLS_'+name])tls[key]=readFileSync(env['ZPR_AUDIT_TLS_'+name]);
 }
 return {url:new URL('/api/assertions/settings',url),organization,tls};
}

function retrieve(config){
 return new Promise((resolve,reject)=>{
  const timer=setTimeout(()=>request.destroy(new Error('timeout')),12000);
  const request=(config.url.protocol==='https:'?https:http).get(config.url,{
   ...config.tls,headers:{Accept:'application/json','Cache-Control':'no-cache'},
  },response=>{
   if(response.statusCode!==200){
    response.resume();reject(new Error('upstream status'));return;
   }
   const chunks=[];let size=0;
   response.on('data',chunk=>{
    size+=chunk.length;
    if(size>maxInventoryBytes)request.destroy(new Error('response too large'));
    else chunks.push(chunk);
   });
   response.on('error',reject);
   response.on('end',()=>{
    try{
     const bytes=Buffer.concat(chunks),text=new TextDecoder('utf-8',{fatal:true}).decode(bytes);
     const snapshot=parseAssertionInventory(text);
     if(snapshot.organization_id!==config.organization)throw new Error('organization mismatch');
     resolve({snapshot,retrieved_at:new Date().toISOString(),
      source_system:'policy-service',source_endpoint:config.url.href,
      response_sha256:createHash('sha256').update(bytes).digest('hex')});
    }catch(error){reject(error)}
   });
  });
  request.on('error',reject);
  request.on('close',()=>clearTimeout(timer));
 });
}

export function createAuditServer({config=null}={}){
 return http.createServer(async(req,res)=>{
  res.setHeader('Cache-Control','no-store');
  res.setHeader('X-Content-Type-Options','nosniff');
  res.setHeader('Referrer-Policy','no-referrer');
  const json=(status,data)=>{
   res.writeHead(status,{'Content-Type':'application/json'});res.end(JSON.stringify(data));
  };
  // Loopback is not an authentication boundary: reject cross-origin and DNS-rebinding reads.
  const host=req.headers.host||'';
  if(!/^(127\.0\.0\.1|localhost|\[::1\]):\d+$/.test(host)||
   (req.headers.origin&&req.headers.origin!=='http://'+host)||
   (req.headers['sec-fetch-site']&&!['same-origin','none'].includes(req.headers['sec-fetch-site']))){
   json(403,{error:'Same-origin loopback access required.'});return;
  }
  if(req.method!=='GET'){json(405,{error:'Read-only service.'});return}
  if(req.url==='/api/audit/source'){
   json(200,config?{configured:true,organization_id:config.organization,source_system:'policy-service'}:{configured:false});return;
  }
  if(req.url==='/api/audit/assertion-settings'){
   if(!config){json(503,{error:'No Policy-Service configured. Use file import or configure the connected adapter.'});return}
   try{json(200,await retrieve(config))}
   catch{json(502,{error:'Policy-Service read failed or returned invalid settings or an unexpected organization. No cached evidence retained. Check operator configuration and service readiness.'})}
   return;
  }
  const asset=assets.get(req.url);
  if(!asset){json(404,{error:'Not found.'});return}
  try{
   const body=await readFile(new URL(asset[0],publicRoot));
   res.writeHead(200,{'Content-Type':asset[1]+'; charset=utf-8'});res.end(body);
  }catch{json(500,{error:'Asset unavailable.'})}
 });
}

if(process.argv[1]===fileURLToPath(import.meta.url)){
 const port=Number(process.env.ZPR_AUDIT_PORT||49282);
 if(!Number.isInteger(port)||port<1||port>65535)throw new Error('Invalid ZPR_AUDIT_PORT.');
 const server=createAuditServer({config:configuration()});
 server.listen(port,'127.0.0.1',()=>console.log('Audit listening on http://127.0.0.1:'+port));
}
