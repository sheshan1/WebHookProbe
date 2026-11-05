import {test,expect} from '@playwright/test';
test.describe('optional parcel demo',()=>{
 test.skip(process.env.TEST_DEMO==='false','Optional demo is not part of the core stack.');
 for(const mode of ['corrected','faulty'])for(const [pattern,copies] of [['normal',1],['sequential',5],['concurrent',10]] as const){
  test(`${mode}: ${pattern}`,async({page})=>{
   await page.goto('/');await page.getByText('Try the included parcel example',{exact:true}).click();await page.getByLabel('Example receiver',{exact:true}).selectOption(mode);await page.getByLabel('Example delivery',{exact:true}).selectOption(pattern);
   if(!process.env.TEST_BASE_URL){await page.getByText('Example connection settings',{exact:true}).click();await page.getByLabel('Example receiver URL',{exact:true}).fill('http://127.0.0.1:8080');}
   await page.getByRole('button',{name:'Prepare example',exact:true}).click();await expect(page.getByRole('status')).toContainText('Example loaded');
   const exporting=page.waitForEvent('download');await page.getByRole('button',{name:'Export settings',exact:true}).click();await page.locator('input[type=file]').setInputFiles((await (await exporting).path())!);await expect(page.getByRole('status')).toContainText('Settings imported');
   await page.getByRole('button',{name:'Run test',exact:true}).click();if(mode==='corrected'&&pattern==='concurrent')await page.reload();
   await expect(page.getByText('Expected responses',{exact:true})).toBeVisible({timeout:15000});await expect(page.locator('.metrics strong').last()).toHaveText(mode==='corrected'||pattern==='normal'?'Passed':'Failed',{timeout:15000});await expect(page.getByRole('table',{name:'Delivery attempts'}).locator('tbody tr')).toHaveCount(copies);
   await page.getByRole('button',{name:'#1',exact:true}).click();await expect(page.getByText('Request #1 response',{exact:false})).toBeVisible();const downloading=page.waitForEvent('download');await page.getByRole('button',{name:'Download report',exact:true}).click();await downloading;
   if(!await page.getByRole('button',{name:'Close example',exact:true}).isVisible())await page.getByText('Try the included parcel example',{exact:true}).click();await page.getByRole('button',{name:'Close example',exact:true}).click();await expect(page.getByRole('status')).toContainText('Example closed');await page.screenshot({path:`test-results/${mode}-${pattern}.png`,fullPage:true});
  });
 }
});
