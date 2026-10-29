import { test, expect } from '@playwright/test';
import { readFile } from 'node:fs/promises';

test('identity report assembly and group directory retain reference-category boundaries',async({page})=>{
 await page.goto('/#reports');
 await page.locator('[data-template="identity"]').click();
 await expect(page.locator('#template-scope option')).toHaveCount(4);
 await expect(page.locator('#template-scope')).not.toContainText('finance-laptop');
 await page.locator('#pack-add-all').click();
 await expect(page.locator('#pack-list li')).toHaveCount(4);
 await page.locator('#template-category').selectOption('machine');
 await expect(page.locator('#template-scope option')).toHaveCount(7);
 await expect(page.locator('#template-scope')).not.toContainText('Alice');
 await page.locator('#template-scope').selectOption('backup-worker-03');
 await page.locator('#pack-add').click();
 await expect(page.locator('#pack-list li')).toHaveCount(5);
 await page.locator('#template-category').selectOption('user');
 await page.locator('[data-pack-template="4"]').click();
 await expect(page.locator('#template-category')).toHaveValue('machine');
 await expect(page.locator('#template-scope')).toHaveValue('backup-worker-03');
 await page.locator('[data-tab="access"]').click();
 await page.locator('#browse-groups').click();
 await expect(page.locator('[data-group-category="user"]')).toContainText('Finance');
 await expect(page.locator('[data-group-category="user"]')).not.toContainText('Managed devices');
 await expect(page.locator('[data-group-category="machine"]')).toContainText('Managed devices');
 await expect(page.locator('[data-group-category="machine"]')).not.toContainText('Finance');
});

test('reference categories have independent counts, pages, reverse selectors and report sections',async({page})=>{
 await page.goto('/');
 for(const kind of ['user','group','machine']){
  await expect(page.locator('[data-category="'+kind+'"]')).toContainText('4 of 4 match');
  const ids=await page.locator('[data-category="'+kind+'"] tbody td:first-child small').allTextContents();
  expect(ids.every(id=>id.startsWith(kind+':'))).toBe(true);
 }
 await page.locator('#candidate-size').selectOption('2');
 await page.locator('[data-category-page="user"][data-step="1"]').click();
 await expect(page.locator('[data-category="user"]')).toContainText('Page 2 of 2');
 await expect(page.locator('[data-category="machine"]')).toContainText('Page 1 of 2');
 await page.locator('#report-button').click();
 await page.locator('#report-author').fill('Category reviewer');
 await page.locator('#report-build').click();
 for(const heading of ['Users · 4 records','Groups · 4 records','Machines · 4 records']){
  await expect(page.locator('#report-document')).toContainText(heading);
 }
 await page.locator('#report-back').click();
 await page.locator('[data-direction="identity"]').click();
 await page.locator('#subject-category').selectOption('group');
 await expect(page.locator('#target option')).toHaveCount(4);
 await expect(page.locator('#target')).not.toContainText('Alice');
 await page.locator('#subject-category').selectOption('machine');
 await expect(page.locator('#target')).not.toContainText('Finance');
});

test('service exploration lists every subject type without a cohort and reverses groups to services', async ({page})=>{
  await page.goto('/');
  await expect(page.locator('#cohort-label')).toBeHidden();
  await expect(page.locator('#category-results tbody tr')).toHaveCount(12);
  for(const name of ['Alice Chen','Finance','finance-laptop-17']){
    await expect(page.locator('#category-results')).toContainText(name);
  }
  await page.locator('#candidate-kind').selectOption('group');
  await expect(page.locator('#access-rows tr')).toHaveCount(4);
  await expect(page.locator('#access-rows .permitted')).toHaveCount(0);
  await page.locator('#report-button').click();
  await page.locator('#report-author').fill('Group reviewer');
  await page.locator('#report-build').click();
  await expect(page.locator('#report-document tbody tr')).toHaveCount(4);
  await expect(page.locator('#report-document')).toContainText('result type: Groups');
  await page.locator('#report-back').click();
  await page.locator('[data-direction="identity"]').click();
  await page.locator('#subject-category').selectOption('group');
  await page.locator('#target').selectOption('Finance');
  await expect(page.locator('#cohort-label')).toBeHidden();
  await expect(page.locator('#access-heading')).toHaveText('What could Finance access?');
  await expect(page.locator('#access-rows tr')).toHaveCount(3);
  await expect(page.locator('#access-rows')).toContainText('device and session constraints remain');
  await expect(page.locator('#access-rows .permitted')).toHaveCount(0);
  await page.locator('#subject-category').selectOption('machine');
  await page.locator('#target').selectOption('backup-worker-03');
  await expect(page.locator('#cohort-label')).toBeVisible();
  await expect(page.locator('#access-rows')).toContainText('Machine identity evidence unavailable');
});

test('topic help preserves workspace state and reports retain moved policy limitations', async ({ page }) => {
  await page.goto('/');
  await page.locator('#candidate-search').fill('Bob');
  await page.getByRole('button',{name:'Help for this page'}).click();
  await expect(page.locator('#audit-help-title')).toHaveText('Access explorer');
  const opening=page.waitForEvent('popup');
  await page.getByRole('link',{name:'Read full help'}).click();
  const help=await opening;
  await expect(help).toHaveURL(/help.html#access$/);
  await expect(help.locator('#access')).toContainText('not proof of connectivity');
  await expect(help.locator('#policy')).toContainText('not executable');
  for(const topic of ['access','policy','assertions','history','reports']){
    await expect(help.locator('#'+topic)).toHaveCount(1);
  }
  await help.close();
  await page.keyboard.press('Escape');
  await expect(page.locator('#help-button')).toBeFocused();
  await expect(page.locator('#candidate-search')).toHaveValue('Bob');
  await page.locator('[data-tab="policy"]').click();
  await page.locator('#report-button').click();
  await page.locator('#report-author').fill('Help reviewer');
  await page.locator('#report-build').click();
  await expect(page.locator('#report-document')).toContainText('No compiler or effective-policy comparison has run');
  await expect(page.locator('#report-document a[href*="help.html"]')).toHaveCount(0);
  await expect(page.locator('#report-document')).toContainText('SAMPLE REPORT');
});

test('toolbar help follows the active workspace and closes without losing selections', async ({page})=>{
  await page.goto('/');
  for(const [topic,title] of [['policy','Policy changes'],['assertions','Assertion review'],['history','Entity history'],['reports','Report packs']]){
    await page.locator('[data-tab="'+topic+'"]').click();
    await page.locator('#help-button').click();
    await expect(page.locator('#audit-help')).toBeVisible();
    await expect(page.locator('#audit-help-title')).toHaveText(title);
    await expect(page.locator('#audit-help-steps li')).toHaveCount(3);
    await expect(page.locator('#audit-help-docs')).toHaveAttribute('href','help.html#'+topic);
    await page.locator('#audit-help [aria-label="Close help"]').click();
    await expect(page.locator('#help-button')).toBeFocused();
    await expect(page).toHaveURL(new RegExp('#'+topic+'$'));
  }
});

test('compact workspace fits the full user cohort in a laptop window without shrinking text', async ({ page }) => {
  await page.setViewportSize({width:1366,height:768});
  await page.goto('/');
  await page.locator('#candidate-kind').selectOption('user');
  await expect(page.locator('#access-rows tr')).toHaveCount(4);
  const layout=await page.evaluate(()=>{
    const rows=[...document.querySelectorAll('#access-rows tr')];
    return {
      tableTop:document.querySelector('#access-rows').getBoundingClientRect().top,
      lastBottom:rows.at(-1).getBoundingClientRect().bottom,
      font:getComputedStyle(document.body).fontSize,
      overflow:document.documentElement.scrollWidth>innerWidth
    };
  });
  expect(layout.tableTop).toBeLessThan(430);
  expect(layout.lastBottom).toBeLessThanOrEqual(720);
  expect(layout.font).toBe('14px');
  expect(layout.overflow).toBe(false);
  await expect(page.locator('#membership-context')).not.toHaveAttribute('open','');
  await page.locator('#membership-context summary').click();
  await expect(page.locator('#membership-summary')).toBeVisible();
  await expect(page.locator('#membership-summary')).toContainText('User and machine authorities are separate');
  await page.locator('[data-tab="assertions"]').click();
  await expect(page.locator('#assertion-details')).toBeVisible();
  await page.setViewportSize({width:390,height:844});
  expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth)).toBe(true);
});

test('checkpoint: user membership to unavailable context to dated report', async ({ page }) => {
  await page.goto('/');
  await page.locator('#browse-groups').click();
  await page.locator('#detail [data-group="Finance"]').click();
  await page.locator('#detail [data-member="Priya Shah"]').click();
  await expect(page.locator('#target')).toHaveValue('Priya Shah');
  await expect(page.locator('#membership-summary')).toContainText('Finance');
  await page.locator('#cohort').selectOption('Trusted source unavailable');
  await expect(page.locator('#access-context')).toContainText('trusted evidence unavailable');
  await expect(page.locator('#access-rows .status.unknown')).toHaveCount(3);
  await page.locator('#access-rows [data-explain]').first().click();
  await expect(page.locator('#detail-body')).toContainText('Trusted source unavailable');
  await expect(page.locator('#detail-body')).not.toContainText('selected sample device/session and VPN');
  await page.getByRole('button',{name:'Close dialog'}).click();
  await page.locator('#context-button').click();
  await expect(page.locator('#detail-body')).toContainText('trusted evidence unavailable');
  await page.getByRole('button',{name:'Close dialog'}).click();
  await page.locator('#report-button').click();
  await page.locator('#report-author').fill('Checkpoint reviewer');
  await page.locator('#report-start').fill('2026-10-01');
  await page.locator('#report-build').click();
  await expect(page.locator('#report-document')).toContainText('2026-10-01 through 2026-10-26');
  await expect(page.locator('#report-document')).toContainText('Priya Shah');
  await expect(page.locator('#report-document')).toContainText('Trusted source unavailable');
  await expect(page.locator('#report-document')).toContainText('not re-queried or date-filtered');
});

test('checkpoint: policy revisions cannot retain unrelated comparison or access scope', async ({ page }) => {
  await page.goto('/');
  await page.locator('#target').selectOption('Payroll API');
  await page.locator('#access-revision').selectOption({index:1});
  await page.locator('[data-tab="policy"]').click();
  await expect(page.locator('#policy-results')).toBeVisible();
  await page.locator('#before').selectOption('r40');
  await expect(page.locator('#policy-results')).toBeHidden();
  await expect(page.locator('#policy-scope')).toContainText('No comparison available for r40');
  await page.locator('#report-button').click();
  await page.locator('#report-author').fill('Checkpoint reviewer');
  await page.locator('#report-build').click();
  await expect(page.locator('#report-document')).toContainText('No comparison available for r40');
  await expect(page.locator('#report-document')).not.toContainText('Newly denied');
  await expect(page.locator('#report-document')).not.toContainText('2 of 3 acknowledgments');
  await page.locator('#report-back').click();
  await page.locator('#before').selectOption('r41');
  await expect(page.locator('#policy-results')).toBeVisible();
  await page.locator('#impact-access').click();
  await expect(page.locator('#target')).toHaveValue('Finance Reports');
  await expect(page.locator('#access-revision')).toHaveValue('r42 · Active');
  await page.locator('#candidate-search').fill('Priya');
  await page.locator('#candidate-kind').selectOption('user');
  await expect(page.locator('#access-rows')).toContainText('Context required');
  await page.locator('#access-revision').selectOption({index:1});
  await expect(page.locator('#access-rows')).toContainText('Permitted');
});

test('checkpoint: diagnostic assertion to scoped dated pack and printable export', async ({ page }) => {
  await page.goto('/#assertions');
  await page.locator('#assertion-role').selectOption('diagnostic');
  await expect(page.locator('#assertion-details')).toContainText('no approved freshness threshold');
  await page.locator('#assertion-add').click();
  await page.locator('[data-tab="reports"]').click();
  await page.locator('#pack-author').fill('Checkpoint reviewer');
  await page.locator('#pack-start').fill('2026-10-01');
  await page.locator('#pack-preview').click();
  await expect(page.locator('#report-document')).toContainText('Diagnostic only');
  await expect(page.locator('#report-document')).toContainText('2026-10-01 through 2026-10-26');
  await expect(page.locator('#report-document')).not.toContainText('assertion:managed-device@a2');
  const downloading=page.waitForEvent('download');
  await page.locator('#report-download').click();
  const html=await readFile(await (await downloading).path(),'utf8');
  const exported=await page.context().newPage();
  await exported.setContent(html);
  await exported.emulateMedia({media:'print'});
  await expect(exported.locator('#report-document')).toBeVisible();
  await expect(exported.locator('#report-document')).toContainText('must not be presented as an enforcing denial');
  await exported.close();
});

test('assertion catalog filters, clears stale details and survives workspace refresh', async ({ page }) => {
  const errors=[],external=[];
  page.on('pageerror',error=>errors.push(error.message));
  page.on('request',request=>{
    if(new URL(request.url()).origin!=='http://127.0.0.1:49283')external.push(request.url());
  });
  await page.goto('/#assertions');
  await expect(page.locator('#assertions')).toBeVisible();
  await expect(page.locator('#assertion-list [data-assertion]')).toHaveCount(3);
  await expect(page.locator('#assertion-details')).toContainText('assertion:finance-member@a3');
  await page.locator('[data-assertion="assertion:managed-device@a2"]').click();
  await expect(page.locator('#assertion-details')).toContainText('Device registry');
  await expect(page.locator('#assertion-details')).not.toContainText('assertion:finance-member');
  await page.locator('#assertion-search').fill('  ASSERTION:MANAGED-DEVICE@A2 ');
  await expect(page.locator('#assertion-count')).toContainText('1 of 3');
  await expect(page.locator('#assertion-heading')).toHaveText('Managed device');
  await page.locator('#assertion-role').selectOption('diagnostic');
  await expect(page.locator('#assertion-list')).toContainText('No matching sample assertions');
  await expect(page.locator('#assertion-details')).not.toContainText('Device registry');
  await expect(page.locator('#assertion-heading')).toHaveText('No assertion selected');
  for(const id of ['assertion-add','assertion-policy','assertion-access']){
    await expect(page.locator('#'+id)).toBeDisabled();
  }
  await page.locator('#assertion-search').fill('');
  await expect(page.locator('#assertion-details')).toContainText('Diagnostic only');
  await expect(page.locator('#assertion-details')).toContainText('no approved freshness threshold');
  await expect(page.locator('#assertion-details')).toContainText('must not be presented as an enforcing denial');
  await page.locator('#assertion-reset').click();
  await expect(page.locator('#assertion-role')).toHaveValue('all');
  await expect(page.locator('#assertion-list [aria-pressed="true"]')).toContainText('Directory freshness');
  await expect(page.locator('#assertion-list [data-assertion]')).toHaveCount(3);
  for(const width of [768,390]){
    await page.setViewportSize({width,height:1024});
    expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth)).toBe(true);
  }
  await page.reload();
  await expect(page.locator('#assertions')).toBeVisible();
  await expect(page.locator('[data-tab="assertions"]')).toHaveAttribute('aria-current','page');
  expect(errors).toEqual([]);
  expect(external).toEqual([]);
});

test('assertion policy and service shortcuts pin their referenced fixture', async ({ page }) => {
  await page.goto('/');
  await page.locator('#target').selectOption('Payroll API');
  await page.locator('#access-revision').selectOption({index:1});
  await page.locator('[data-tab="policy"]').click();
  await page.locator('#before').selectOption('r40');
  await page.locator('#after').selectOption({index:1});
  await page.locator('[data-tab="assertions"]').click();
  await page.locator('[data-assertion="assertion:managed-device@a2"]').click();
  await page.locator('#assertion-policy').click();
  await expect(page).toHaveURL(/#policy$/);
  await expect(page.locator('#before')).toHaveValue('r41');
  await expect(page.locator('#after')).toHaveValue('r42 · Active');
  await page.locator('[data-tab="assertions"]').click();
  await expect(page.locator('#assertion-heading')).toHaveText('Managed device');
  await page.locator('#assertion-access').click();
  await expect(page).toHaveURL(/#access$/);
  await expect(page.locator('#target')).toHaveValue('Finance Reports');
  await expect(page.locator('#cohort-label')).toBeHidden();
  await expect(page.locator('#candidate-kind')).toHaveValue('all');
  await expect(page.locator('#access-revision')).toHaveValue('r42 · Active');
  await expect(page.locator('#category-results')).toContainText('Finance membership requirement not satisfied');
});

test('adding an assertion reuses pack deduplication without changing report template selection', async ({ page }) => {
  await page.goto('/#reports');
  await page.locator('#template-scope').selectOption('Engineering');
  await page.locator('[data-tab="assertions"]').click();
  await page.locator('#assertion-role').selectOption('diagnostic');
  await page.locator('#assertion-add').click();
  await expect(page.locator('#assertion-feedback')).toContainText('Added Directory freshness diagnostic');
  await page.locator('#assertion-add').click();
  await expect(page.locator('#assertion-feedback')).toContainText('already in the report pack');
  await page.locator('[data-tab="reports"]').click();
  await expect(page.locator('#template-name')).toHaveText('Group membership & access');
  await expect(page.locator('#template-scope')).toHaveValue('Engineering');
  await expect(page.locator('#pack-list li')).toHaveCount(1);
  await expect(page.locator('#pack-list')).toContainText('Directory freshness diagnostic');
  await page.locator('[data-pack-preview="0"]').click();
  await expect(page.locator('#detail-body')).toContainText('assertion:directory-freshness@a1');
  await expect(page.locator('#detail-body')).not.toContainText('assertion:finance-member@a3');
});

test('assertion current-view export uses all filtered matches and distinguishes empty scope', async ({ page }) => {
  await page.goto('/#assertions');
  await page.locator('#assertion-role').selectOption('enforcing');
  await page.locator('#report-button').click();
  await page.locator('#report-author').fill('Assertion reviewer');
  await page.locator('#report-build').click();
  await expect(page.locator('#report-document')).toContainText('role: Enforcing');
  await expect(page.locator('#report-document')).toContainText('assertion:finance-member@a3');
  await expect(page.locator('#report-document')).toContainText('assertion:managed-device@a2');
  await expect(page.locator('#report-document')).not.toContainText('assertion:directory-freshness@a1');
  const downloading=page.waitForEvent('download');
  await page.locator('#report-download').click();
  const exported=await readFile(await (await downloading).path(),'utf8');
  expect(exported).toContain('assertion:managed-device@a2');
  expect(exported).toContain('not compiler/evaluator results');
  expect(exported).toContain('SAMPLE REPORT');
  await page.locator('#report-back').click();
  await expect(page.locator('#report-button')).toBeFocused();
  await expect(page.locator('#assertions')).toBeVisible();
  await page.locator('#assertion-search').fill('<no matches>');
  await page.locator('#report-button').click();
  await page.locator('#report-author').fill('Assertion reviewer');
  await page.locator('#report-build').click();
  await expect(page.locator('#report-document')).toContainText('<no matches>');
  await expect(page.locator('#report-document')).toContainText('does not establish absence');
  await expect(page.locator('#report-document')).not.toContainText('assertion:finance-member@a3');
});

test('template search, bulk addition and confirmed removal retain unrelated reports', async ({ page }) => {
  await page.goto('/#reports');
  await page.locator('#pack-author').fill('Retained reviewer');
  await page.locator('#pack-notes').fill('Retained notes');
  await page.locator('#pack-add').click();
  await page.locator('#template-search').fill('assertion');
  await expect(page.locator('#template-count')).toContainText('1 of 6');
  await expect(page.locator('#template-catalog [data-template]')).toHaveCount(1);
  await page.locator('[data-template="assertion"]').click();
  await page.locator('#pack-add-all').click();
  await expect(page.locator('#pack-list li')).toHaveCount(5);
  await expect(page.locator('#pack-feedback')).toContainText('4 reports added; 0 already');
  await expect(page.locator('[data-template="assertion"]')).toContainText('(4 in pack)');
  await expect(page.locator('#pack-coverage')).toContainText('Assertion review: 4 of 4');
  await expect(page.locator('#pack-coverage')).toContainText('Service access review: Not included');
  await page.locator('#pack-add-all').click();
  await expect(page.locator('#pack-list li')).toHaveCount(5);
  await expect(page.locator('#pack-feedback')).toContainText('0 reports added; 4 already');
  await page.locator('#template-search').fill('No such template');
  await expect(page.locator('#template-catalog')).toContainText('No matching templates');
  await expect(page.locator('#template-name')).toHaveText('Assertion review');
  await page.locator('#pack-remove-template').click();
  await page.getByRole('button', { name: 'Close dialog' }).click();
  await expect(page.locator('#pack-list li')).toHaveCount(5);
  await page.locator('#pack-remove-template').click();
  await page.locator('#pack-remove-template-confirm').click();
  await expect(page.locator('#pack-list li')).toHaveCount(1);
  await expect(page.locator('#pack-list')).toContainText('Finance');
  await expect(page.locator('#pack-remove-template')).toBeDisabled();
  await expect(page.locator('#pack-author')).toHaveValue('Retained reviewer');
  await expect(page.locator('#pack-notes')).toHaveValue('Retained notes');
  await page.locator('#template-search').fill('');
  await expect(page.locator('#template-count')).toContainText('6 of 6');
  await expect(page.locator('[data-template="assertion"]')).toContainText('(0 in pack)');
});

test('pack item previews and template shortcuts use the item scope without mutation', async ({ page }) => {
  await page.goto('/#reports');
  await page.locator('#template-scope').selectOption('Engineering');
  await page.locator('#pack-add').click();
  await page.locator('[data-template="coverage"]').click();
  await page.locator('#pack-add').click();
  await page.locator('[data-pack-preview="0"]').click();
  await expect(page.locator('#detail-title')).toContainText('Group membership & access');
  await expect(page.locator('#detail-body')).toContainText('Scope: Engineering');
  await page.getByRole('button', { name: 'Close dialog' }).click();
  await expect(page.locator('#pack-list li')).toHaveCount(2);
  await page.locator('#template-search').fill('coverage');
  await page.locator('[data-pack-template="0"]').click();
  await expect(page.locator('#template-search')).toHaveValue('');
  await expect(page.locator('#template-scope')).toHaveValue('Engineering');
  await expect(page.locator('#template-scope')).toBeFocused();
  await page.locator('#pack-add').click();
  await expect(page.locator('#pack-feedback')).toContainText('0 reports added; 1 already');
  await expect(page.locator('#pack-list li')).toHaveCount(2);
});

test('pack contents navigate without changing workspace and exports retain valid links', async ({ page }) => {
  await page.goto('/#reports');
  await page.locator('#pack-add').click();
  await page.locator('[data-template="coverage"]').click();
  await page.locator('#pack-add').click();
  await page.locator('[data-pack-up="1"]').click();
  await page.locator('#pack-author').fill('Reviewer');
  await page.locator('#pack-preview').click();
  await expect(page.locator('#pack-section-1 h2')).toHaveText('1. Evidence coverage');
  await page.locator('a[href="#pack-section-2"]').click();
  await expect(page).toHaveURL(/#reports$/);
  await expect(page.locator('#pack-section-2')).toBeInViewport();
  await page.locator('#pack-section-2 a[href="#pack-contents"]').click();
  await expect(page.locator('#pack-contents')).toBeInViewport();
  const downloading=page.waitForEvent('download');
  await page.locator('#report-download').click();
  const exported=await readFile(await (await downloading).path(),'utf8');
  expect(exported).toContain('href="#pack-section-2"');
  expect(exported).toContain('id="pack-section-2"');
  expect(exported).toContain('href="#pack-contents"');
  await page.locator('#report-back').click();
  await expect(page.locator('#pack-preview')).toBeFocused();
  await expect(page).toHaveURL(/#reports$/);
  await page.reload();
  await expect(page.locator('#reports')).toBeVisible();
});

test('sorting and pagination preserve explanations and full report scope', async ({ page }) => {
  await page.goto('/');
  await page.locator('#candidate-kind').selectOption('user');
  await page.locator('#candidate-size').selectOption('2');
  await expect(page.locator('#access-rows tr')).toHaveCount(2);
  await expect(page.locator('#page-summary')).toContainText('Page 1 of 2');
  await expect(page.locator('#page-previous')).toBeDisabled();
  await page.locator('#page-next').click();
  await expect(page.locator('#access-rows')).toContainText('Jordan');
  await page.locator('#access-rows [data-explain]').first().click();
  await expect(page.locator('#detail-title')).toContainText('Jordan Lee');
  await page.getByRole('button', { name: 'Close dialog' }).click();
  await expect(page.locator('#page-next')).toBeDisabled();
  await page.locator('#candidate-sort').selectOption('reverse');
  await expect(page.locator('#page-summary')).toContainText('Page 1');
  await expect(page.locator('#access-rows tr').first()).toContainText('Priya');
  await page.locator('#report-button').click();
  await page.locator('#report-author').fill('Reviewer');
  await page.locator('#report-build').click();
  await expect(page.locator('#report-document')).toContainText('All filtered matches across pages');
  await expect(page.locator('#report-document tbody tr')).toHaveCount(4);
  await page.locator('#report-back').click();
  await page.locator('#candidate-search').fill('No such candidate');
  await expect(page.locator('#page-summary')).toContainText('does not prove absence');
  await expect(page.locator('#page-next')).toBeDisabled();
  await expect(page.locator('#page-previous')).toBeDisabled();
});

test('pagination resets with scope and works in either access direction', async ({ page }) => {
  await page.goto('/');
  await page.locator('#candidate-kind').selectOption('user');
  await page.locator('#candidate-size').selectOption('2');
  await page.locator('#page-next').click();
  await page.locator('#page-previous').click();
  await expect(page.locator('#access-rows tr').first()).toContainText('Alice');
  await page.locator('#candidate-sort').selectOption('outcome');
  await expect(page.locator('#access-rows tr').first()).toContainText('Priya');
  await page.locator('#page-next').click();
  await page.locator('#outcome').selectOption('denied');
  await expect(page.locator('#page-summary')).toContainText('Page 1 of 1');
  await expect(page.locator('#access-rows tr')).toHaveCount(1);
  await expect(page.locator('#access-rows')).toContainText('Bob');
  await page.locator('#access-reset').click();
  await expect(page.locator('#candidate-kind')).toHaveValue('all');
  await expect(page.locator('[data-category="user"]')).toContainText('Page 1 of 2');
  await expect(page.locator('[data-category="machine"]')).toContainText('Page 1 of 2');
  await page.locator('[data-direction="identity"]').click();
  await page.locator('#candidate-sort').selectOption('name');
  await expect(page.locator('#access-rows tr')).toHaveCount(2);
  await expect(page.locator('#access-rows tr').first()).toContainText('Engineering Git');
  await page.locator('#page-next').click();
  await expect(page.locator('#access-rows tr')).toHaveCount(1);
  await expect(page.locator('#access-rows')).toContainText('Payroll API');
  await page.locator('#candidate-size').selectOption('5');
  await expect(page.locator('#page-summary')).toContainText('Page 1 of 1');
  await expect(page.locator('#access-rows tr')).toHaveCount(3);
});

test('group directory and filter reset preserve selected request', async ({ page }) => {
  await page.goto('/');
  await page.locator('#browse-groups').click();
  await expect(page.getByRole('heading', { name: 'Browse sample groups' })).toBeVisible();
  await page.locator('#detail [data-group="Finance"]').click();
  await expect(page.locator('#detail [data-member]')).toHaveCount(2);
  await page.getByRole('button', { name: 'Close dialog' }).click();
  await page.locator('#target').selectOption('Engineering Git');
  await page.locator('#group-filter').selectOption('Finance');
  await page.locator('#outcome').selectOption('permitted');
  await page.locator('#candidate-search').fill('Nobody');
  await page.locator('#access-reset').click();
  await expect(page.locator('#target')).toHaveValue('Engineering Git');
  await expect(page.locator('#group-filter')).toHaveValue('all');
  await expect(page.locator('#outcome')).toHaveValue('all');
  await expect(page.locator('#candidate-search')).toHaveValue('');
  await expect(page.locator('#category-results')).toContainText('Bob Evans');
});

test('template preview does not add reports and notes remain escaped', async ({ page }) => {
  await page.goto('/#reports');
  await page.locator('[data-template="assertion"]').click();
  await page.locator('#template-scope').selectOption('Managed device');
  await page.locator('#template-preview').click();
  await expect(page.locator('#detail')).toContainText('assertion:managed-device@a2');
  await expect(page.locator('#pack-list li')).toHaveCount(0);
  await page.getByRole('button', { name: 'Close dialog' }).click();
  await page.locator('#pack-add').click();
  await expect(page.locator('#pack-summary')).toContainText('1 reports · 1 template types · 1 distinct scopes');
  await page.locator('[data-template="coverage"]').click();
  await page.locator('#pack-add').click();
  await expect(page.locator('#pack-summary')).toContainText('2 reports · 2 template types · 2 distinct scopes');
  await page.locator('#pack-author').fill('Sample reviewer');
  await page.locator('#pack-notes').fill('Follow up <img src=x onerror=alert(1)>\nMissing source acknowledgment');
  await page.locator('#pack-preview').click();
  await expect(page.locator('.review-notes')).toContainText('<img src=x onerror=alert(1)>');
  await expect(page.locator('.review-notes img')).toHaveCount(0);
  const downloading = page.waitForEvent('download');
  await page.locator('#report-download').click();
  const download = await downloading;
  const exported = await readFile(await download.path(), 'utf8');
  expect(exported).toContain('Reviewer notes · unverified commentary');
  expect(exported).toContain('&lt;img');
});

test('candidate search and exported view share scope', async ({ page }) => {
  await page.goto('/');
  await page.locator('#candidate-kind').selectOption('user');
  await page.locator('#candidate-search').fill('USER:BOB');
  await expect(page.locator('#access-rows tr')).toHaveCount(1);
  await expect(page.locator('#access-rows')).toContainText('Bob Evans');
  await page.locator('#report-button').click();
  await page.locator('#report-author').fill('Reviewer');
  await page.locator('#report-build').click();
  await expect(page.locator('#report-document')).toContainText('USER:BOB');
  await expect(page.locator('#report-document')).toContainText('Bob Evans');
  await expect(page.locator('#report-document')).not.toContainText('Alice Chen');
});

test('pack down and confirmed clearing preserve reporting details', async ({ page }) => {
  await page.goto('/#reports');
  await expect(page.locator('#pack-clear')).toBeDisabled();
  await page.locator('#pack-add').click();
  await page.locator('[data-template="coverage"]').click();
  await page.locator('#pack-add').click();
  await page.locator('[data-pack-down="0"]').click();
  await expect(page.locator('#pack-list li').first()).toContainText('Evidence coverage');
  await page.locator('#pack-author').fill('Retained reviewer');
  await page.locator('#pack-clear').click();
  await page.getByRole('button', { name: 'Close dialog' }).click();
  await expect(page.locator('#pack-list li')).toHaveCount(2);
  await page.locator('#pack-clear').click();
  await page.locator('#pack-clear-confirm').click();
  await expect(page.locator('#pack-list li')).toHaveCount(0);
  await expect(page.locator('#pack-author')).toHaveValue('Retained reviewer');
});

test('printing without a preview explicitly reports missing report', async ({ page }) => {
  await page.goto('/');
  await page.emulateMedia({ media: 'print' });
  await expect(page.locator('#print-empty')).toBeVisible();
  await expect(page.locator('#report-preview')).toBeHidden();
});

test('standalone Audit groups, reports and shared palette', async ({ page }) => {
  const errors = [];
  const external = [];
  page.on('pageerror', error => errors.push(error.message));
  page.on('request', request => {
    if (new URL(request.url()).origin !== 'http://127.0.0.1:49283') external.push(request.url());
  });

  await page.goto('/');
  await expect(page.getByText('Standalone workspace', { exact: true })).toBeVisible();
  await page.locator('#candidate-kind').selectOption('user');
  expect(await page.evaluate(() => getComputedStyle(document.documentElement).getPropertyValue('--accent-primary').trim())).toBe('#174d3d');
  await page.locator('#access-rows [data-group="Finance"]').first().click();
  await expect(page.locator('#detail [data-member]')).toHaveCount(2);
  await page.locator('#detail [data-group-service="Finance Reports"]').click();
  await expect(page.locator('#group-filter')).toHaveValue('Finance');
  await page.locator('[data-tab="reports"]').click();
  await page.locator('[data-template="assertion"]').click();
  await page.locator('#pack-add').click();
  await page.locator('#pack-author').fill('Sample reviewer');
  await page.locator('#pack-preview').click();
  await expect(page.locator('#report-document')).toContainText('Diagnostic only');
  await expect(page.locator('#report-document')).toContainText('SAMPLE REPORT PACK');
  const downloading = page.waitForEvent('download');
  await page.locator('#report-download').click();
  const download = await downloading;
  expect(download.suggestedFilename()).toContain('SAMPLE');
  const downloaded = await download.path();
  const exported = await readFile(downloaded, 'utf8');
  expect(exported).toContain('Diagnostic only');
  expect(exported).toContain('--accent-primary');
  const reportPage = await page.context().newPage();
  await reportPage.setContent(exported);
  await expect(reportPage.locator('#report-document')).toContainText('Diagnostic only');
  await reportPage.emulateMedia({ media: 'print' });
  await expect(reportPage.locator('#report-document')).toBeVisible();
  await reportPage.close();
  await page.emulateMedia({ media: 'print' });
  await expect(page.locator('.report-controls')).toBeHidden();
  await page.emulateMedia({ media: 'screen' });
  await page.locator('#report-back').click();
  await page.setViewportSize({ width: 768, height: 1024 });
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
  expect(errors).toEqual([]);
  expect(external).toEqual([]);
});

test('view navigation survives refresh and rejects unknown views', async ({ page }) => {
  await page.goto('/#reports');
  await expect(page.locator('#reports')).toBeVisible();
  await page.reload();
  await expect(page.locator('#reports')).toBeVisible();
  await page.locator('[data-tab="history"]').click();
  await expect(page).toHaveURL(/#history$/);
  await page.goto('/#unknown');
  await expect(page.locator('#access')).toBeVisible();
  await expect(page).toHaveURL(/#access$/);
});

test('live audit feed renders escaped events from the authenticated endpoint', async ({page}) => {
  let requestedURL;
  await page.route('**/api/audit/events?*', async route => {
    requestedURL=route.request().url();
    await route.fulfill({json:{events:[{
      id:'event-1"><img src=x onerror=alert(1)>',
      occurred_at:'2026-10-28T12:00:00Z',
      actor_issuer:'https://idp.example',
      actor_subject:'operator <img src=x>',
      action:'audit.history.read',
      target:'/api/audit/events',
      outcome:'requested',
      source:'control-room'
    }]}})
  });
  await page.goto('/#history');
  await page.locator('#load-live-history').click();
  await expect(page.locator('#live-history-status')).toContainText('Loaded 1 event.');
  await expect(page.locator('#live-history-events')).toContainText('audit.history.read');
  await expect(page.locator('#live-history-events')).toContainText('operator <img src=x>');
  await expect(page.locator('#live-history-events img')).toHaveCount(0);
  expect(new URL(requestedURL).searchParams.get('limit')).toBe('50');
});

test('audit history defaults to a seven-day lookback and offers day, week, month, and custom windows', async ({page}) => {
  let requestURL;
  await page.route('**/api/audit/events?*', async route => {
    requestURL=new URL(route.request().url());
    await route.fulfill({json:{events:[]}});
  });
  await page.goto('/#history');
  await expect(page.locator('#live-history-window')).toHaveValue('week');
  const defaultRange=await page.evaluate(()=>[
    new Date(document.querySelector('#live-history-from').value).getTime(),
    new Date(document.querySelector('#live-history-until').value).getTime()
  ]);
  expect(defaultRange[1]-defaultRange[0]).toBe(7*24*60*60*1000);
  await page.locator('#load-live-history').click();
  await expect.poll(()=>requestURL?.searchParams.get('from')).toBeTruthy();
  expect(requestURL.searchParams.get('until')).toBeTruthy();

  for(const [windowName,duration] of [['day',24],['week',168],['month',720]]){
    await page.locator('#live-history-window').selectOption(windowName);
    const range=await page.evaluate(()=>[
      new Date(document.querySelector('#live-history-from').value).getTime(),
      new Date(document.querySelector('#live-history-until').value).getTime()
    ]);
    expect(range[1]-range[0]).toBe(duration*60*60*1000);
  }
  await page.locator('#live-history-from').fill('2026-10-01T10:00');
  await page.locator('#live-history-from').dispatchEvent('change');
  await expect(page.locator('#live-history-window')).toHaveValue('custom');
});

test('live audit feed distinguishes an empty store from authorization and service errors', async ({page}) => {
  let status=200;
  await page.route('**/api/audit/events?*', route => {
    if(status===200)return route.fulfill({json:{events:[]}});
    return route.fulfill({status,json:{error:'unavailable'}});
  });
  await page.goto('/#history');
  await page.locator('#load-live-history').click();
  await expect(page.locator('#live-history-status')).toContainText('does not prove that no activity occurred');
  status=403;
  await page.locator('#load-live-history').click();
  await expect(page.locator('#live-history-status')).toContainText('audit.read permission');
  status=503;
  await page.locator('#load-live-history').click();
  await expect(page.locator('#live-history-status')).toContainText('unavailable or not configured');
  await expect(page.locator('#live-history-events')).toBeEmpty();
});

test('live audit filters are exact and pagination retains the original query', async ({page}) => {
  const requests=[];
  await page.route('**/api/audit/events?*', async route => {
    requests.push(new URL(route.request().url()));
    await route.fulfill({json:{events:[],next_cursor:requests.length===1?'next-page':''}});
  });
  await page.goto('/#history');
  await page.locator('#live-history-action').fill('policy.read');
  await page.locator('#live-history-source').fill('control-room');
  await page.locator('#live-history-outcome').fill('success');
  await page.locator('#live-history-from').fill('2026-10-28T08:00');
  await page.locator('#live-history-until').fill('2026-10-28T09:00');
  const expectedTimes=await page.evaluate(()=>[
    new Date(document.querySelector('#live-history-from').value).toISOString(),
    new Date(document.querySelector('#live-history-until').value).toISOString()
  ]);
  await page.locator('#load-live-history').click();
  await expect(page.locator('#live-history-more')).toBeVisible();
  await page.locator('#live-history-more').click();
  expect(requests).toHaveLength(2);
  for(const request of requests){
    expect(request.searchParams.get('action')).toBe('policy.read');
    expect(request.searchParams.get('source')).toBe('control-room');
    expect(request.searchParams.get('outcome')).toBe('success');
    expect(request.searchParams.get('from')).toBe(expectedTimes[0]);
    expect(request.searchParams.get('until')).toBe(expectedTimes[1]);
  }
  expect(requests[0].searchParams.has('cursor')).toBe(false);
  expect(requests[1].searchParams.get('cursor')).toBe('next-page');
});

test('live audit query rejects reversed time bounds without calling the endpoint', async ({page}) => {
  let requests=0;
  await page.route('**/api/audit/events?*', route => {
    requests++;
    return route.fulfill({json:{events:[]}});
  });
  await page.goto('/#history');
  await page.locator('#live-history-from').fill('2026-10-28T09:00');
  await page.locator('#live-history-until').fill('2026-10-28T08:00');
  await page.locator('#load-live-history').click();
  await expect(page.locator('#live-history-status')).toHaveText('The from time must be earlier than the until time.');
  expect(requests).toBe(0);
});
