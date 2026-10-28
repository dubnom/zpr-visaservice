import {test,expect} from '@playwright/test';
import http from 'node:http';
import {configuration,createAuditServer} from '../server.mjs';
const data={organization_id:'real-org',organization_name:'Actual <organization>',settings:{revision:12,source:'<img src=x onerror=alert(1)>',enabled:true,interval_seconds:60}};
const envelope={snapshot:data,source_system:'policy-service',source_endpoint:'https://policy.example/api/assertions/settings',retrieved_at:'2026-10-27T13:00:00.000Z',response_sha256:'a'.repeat(64)};
test('browser reads an actual independent operator service through the connected adapter',async({page})=>{
 const upstream=http.createServer((req,res)=>{
  if(req.method!=='GET'||req.url!=='/api/assertions/settings'){res.writeHead(404);res.end();return}
  res.setHeader('Content-Type','application/json');res.end(JSON.stringify(data));
 });
 await new Promise(resolve=>upstream.listen(0,'127.0.0.1',resolve));
 const adapter=createAuditServer({config:configuration({
  ZPR_AUDIT_POLICY_SERVICE_URL:'http://127.0.0.1:'+upstream.address().port,
  ZPR_AUDIT_ORGANIZATION_ID:data.organization_id,
  ZPR_SIMULATOR_URL:'http://invalid.invalid',
 })});
 await new Promise(resolve=>adapter.listen(0,'127.0.0.1',resolve));
 try{
  await page.goto('http://127.0.0.1:'+adapter.address().port+'/#inventory');
  await page.locator('#source-check').click();
  await page.locator('#source-fetch').click();
  await expect(page.locator('#inventory-status')).toContainText('Read settings revision 12');
  await expect(page.locator('#inventory-metadata')).toContainText('Read from configured Policy-Service');
  await page.locator('#inventory-preparer').fill('Integration reviewer');
  await page.locator('#inventory-purpose').fill('Operator service review');
  await page.locator('#report-button').click();
  await expect(page.locator('#report-document')).toContainText('SERVICE SETTINGS SNAPSHOT');
 }finally{
  await page.goto('about:blank');
  for(const server of [adapter,upstream]){
   server.closeAllConnections();await new Promise(resolve=>server.close(resolve));
  }
 }
});
test('explicit connected read uses real contract and existing review/report without fixtures',async({page})=>{
 const requests=[];
 await page.route('**/api/audit/**',async route=>{
  requests.push(new URL(route.request().url()).pathname);
  await route.fulfill({json:route.request().url().endsWith('/source')?{configured:true,organization_id:'real-org',source_system:'policy-service'}:envelope});
 });
 await page.goto('/#inventory');
 expect(requests).toEqual([]);
 await page.locator('#source-check').click();
 await expect(page.locator('#source-connection')).toContainText('real-org');
 await page.locator('#source-fetch').click();
 await expect(page.locator('#inventory-status')).toContainText('Read settings revision 12');
 await expect(page.locator('#inventory-metadata')).toContainText('Actual <organization>');
 await expect(page.locator('#inventory-metadata')).toContainText(envelope.source_endpoint);
 await expect(page.locator('#inventory-metadata')).toContainText(envelope.response_sha256);
 await expect(page.locator('#inventory-source img')).toHaveCount(0);
 await page.locator('#inventory-preparer').fill('Reviewer');
 await page.locator('#inventory-purpose').fill('Production settings review');
 await page.locator('#report-button').click();
 await expect(page.locator('#report-document')).toContainText('SERVICE SETTINGS SNAPSHOT');
 await expect(page.locator('#report-document')).toContainText(envelope.source_endpoint);
 await expect(page.locator('#report-document')).not.toContainText('No live sources queried');
 await expect(page.locator('#report-document')).not.toContainText('Northstar');
 await page.locator('#report-back').click();
 await page.route('**/api/audit/assertion-settings',route=>route.fulfill({status:502,json:{error:'unavailable'}}));
 await page.locator('#source-fetch').click();
 await expect(page.locator('#inventory-error')).toContainText('No previous evidence');
 await expect(page.locator('#inventory-record')).toBeHidden();
 await page.locator('#report-button').click();
 await expect(page.locator('#report-preview')).toBeHidden();
});
test('static hosting remains explicit and stale reads cannot replace file imports',async({page})=>{
 await page.goto('/#inventory');
 await page.locator('#source-check').click();
 await expect(page.locator('#source-connection')).toContainText('Static hosting');
 await expect(page.locator('#source-fetch')).toBeDisabled();
 await page.route('**/api/audit/source',route=>route.fulfill({json:{configured:true,organization_id:'real-org',source_system:'policy-service'}}));
 let release;
 const paused=new Promise(resolve=>release=resolve);
 await page.route('**/api/audit/assertion-settings',async route=>{await paused;await route.fulfill({json:envelope})});
 await page.locator('#source-check').click();
 await page.locator('#source-fetch').click();
 await page.locator('#inventory-file').setInputFiles({name:'operator.json',mimeType:'application/json',buffer:Buffer.from(JSON.stringify({...data,settings:{...data.settings,revision:99}}))});
 await expect(page.locator('#inventory-status')).toContainText('revision 99');
 release();
 await expect(page.locator('#inventory-status')).toContainText('revision 99');
 await expect(page.locator('#inventory-metadata')).not.toContainText(envelope.source_endpoint);
});
