import {test,expect} from '@playwright/test';
import {readFile} from 'node:fs/promises';

test('import comparison and exported report work with external services blocked',async({context,page,baseURL})=>{
 const external=[],errors=[];
 const origin=new URL(baseURL).origin;
 context.on('request',request=>{
  if(new URL(request.url()).origin!==origin)external.push(request.url());
 });
 context.on('page',opened=>opened.on('pageerror',error=>errors.push(error.message)));
 page.on('pageerror',error=>errors.push(error.message));
 await context.route('**/*',route=>{
  if(new URL(route.request().url()).origin===origin)return route.continue();
  return route.abort('blockedbyclient');
 });
 const snapshot={organization_id:'boundary-test',organization_name:'Synthetic boundary test',
  base_dn:'dc=ignored,dc=test',settings:{revision:2,source:'current synthetic source',enabled:false,interval_seconds:60}};
 const upload=async(id,value)=>page.locator('#'+id).setInputFiles({
  name:id+'.json',mimeType:'application/json',buffer:Buffer.from(JSON.stringify(value))
 });
 await page.goto('/#inventory');
 await upload('inventory-file',{...snapshot,scope:'organization',settings_error:'',
  last_run:{status:'must not appear in report'},sources:[{name:'discarded runtime source'}]});
 await upload('baseline-file',{...snapshot,settings:{...snapshot.settings,revision:1,source:'baseline synthetic source'}});
 await expect(page.locator('#baseline-results')).toContainText('Settings differ across revisions');
 await page.locator('#inventory-preparer').fill('Boundary regression');
 await page.locator('#inventory-purpose').fill('Synthetic offline integration check');
 await page.locator('#report-button').click();
 await expect(page.locator('#report-document')).toContainText('current synthetic source');
 await expect(page.locator('#report-document')).toContainText('baseline synthetic source');
 await expect(page.locator('#report-document')).not.toContainText('must not appear in report');
 await expect(page.locator('#report-document')).not.toContainText('discarded runtime source');
 const downloading=page.waitForEvent('download');
 await page.locator('#report-download').click();
 const html=await readFile(await (await downloading).path(),'utf8');
 const report=await context.newPage();
 await report.setContent(html);
 await report.emulateMedia({media:'print'});
 await expect(report.locator('#report-document')).toBeVisible();
 await expect(report.locator('#report-document')).toContainText('baseline synthetic source');
 expect(errors).toEqual([]);
 expect(external).toEqual([]);
});
