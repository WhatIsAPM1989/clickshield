import assert from 'node:assert/strict';
import {mkdir} from 'node:fs/promises';
import {fileURLToPath} from 'node:url';
import {chromium, webkit} from 'playwright';

const engine = process.env.CLICKSHIELD_INVITE_BROWSER;
assert.ok(['chromium', 'webkit'].includes(engine));
const origin = process.env.CLICKSHIELD_INVITE_BASE;
const browser = await ({chromium, webkit})[engine].launch({headless: true});
try {
  const page = await browser.newPage();
  await page.goto(`${origin}/invite/${process.env.CLICKSHIELD_INVITE_TOKEN}`);
  await page.getByLabel('Email',{exact:true}).fill(process.env.CLICKSHIELD_INVITE_EMAIL);
  await page.getByLabel('Пароль (от 12 символов)',{exact:true}).fill('Browser-test-password-123');
  await page.getByLabel('Повторите пароль',{exact:true}).fill('Browser-test-password-123');
  const [response] = await Promise.all([
   page.waitForResponse(r=>r.request().method()==='POST'),
   page.getByRole('button',{name:'Создать аккаунт',exact:true}).click(),
  ]);
  const headers = await response.request().allHeaders();
  console.log(`${engine}: POST ${response.status()}, Origin=${headers.origin}`);
  assert.equal(headers.origin,origin,'same-origin form must not send Origin:null');
  assert.equal(headers.referer,`${origin}/`,'Referer must not include the invitation token');
  assert.equal(response.status(),303);
  await page.waitForURL(`${origin}/`);
  await page.getByRole('button',{name:'Выйти',exact:true}).waitFor();
  const output = new URL('../output/playwright/',import.meta.url);
  await mkdir(output,{recursive:true});
  await page.screenshot({path:fileURLToPath(new URL(`invite-${engine}.png`,output))});
  console.log(`${engine}: account created and signed in`);
} finally { await browser.close(); }
