// 격리된 API 응답으로 Precision 화면의 스타일·반응형·접근성 계약을 브라우저에서 검증한다.
import assert from 'node:assert/strict';
import { mkdir } from 'node:fs/promises';
import { fileURLToPath, pathToFileURL } from 'node:url';
const { chromium } = await import(process.argv[2] ? pathToFileURL(process.argv[2]).href : 'playwright');
const browser = await chromium.launch({ channel: 'chrome', headless: true });
const output = new URL('../.tools/precision-screens/', import.meta.url);
await mkdir(output, { recursive: true });
const page = await browser.newPage({ viewport: { width: 1440, height: 1000 }, reducedMotion: 'reduce' });
page.setDefaultTimeout(10000);
const errors = [];
page.on('pageerror', error => errors.push(error.message));
let loggedIn = true;
let totpEnabled = false;
const user = { id: 'ui-test', username: 'precision-admin', role: 'admin', is_active: true, created_at: '2026-10-25T00:00:00Z' };
const schema = { type: 'object', properties: { title: { type: 'string', title: '제목', default: '자동화 테스트' }, enabled: { type: 'boolean', title: '사용', default: true }, mode: { type: 'string', title: '모드', enum: ['daily','weekly'] }, details: { type: 'object', title: '세부 정보' } } };
const plugin = { id: 'demo', name: '데모 플러그인', version: '1.0.0', state: 'active', repository_url: 'https://example.com/demo.git', active_release_id: 'release', manifest: { id: 'demo', name: '데모 플러그인', version: '1.0.0', data_schema_version: 1, tasks: [{ id: 'sync', title: '동기화 실행', input_schema: schema }], settings_schema: schema, menus: [{ id: 'home', title: '데모 작업', icon: 'puzzle', resource: 'records' }], resources: [{ id: 'records', methods: ['GET'] }] } };
const menu = { id: 'home', plugin_id: 'demo', plugin_name: plugin.name, title: '데모 작업', icon: 'puzzle', position: 0, enabled: true };
const items = value => ({ items: value, total: value.length });
await page.route('**/api/v1/**', async route => {
  const path = new URL(route.request().url()).pathname.replace('/api/v1','');
  let data = items([]), status = 200;
  if (path === '/events') return route.fulfill({ status: 200, contentType: 'text/event-stream', body: ': fixture\n\n' });
  if (path === '/auth/me') { data = loggedIn ? { user, csrf_token: 'fixture' } : { error: { code: 'unauthorized', message: '로그인 필요' } }; status = loggedIn ? 200 : 401; }
  else if (path === '/auth/options') data = { passkey_available: false, passkey_reason: '로컬 UI 테스트' };
  else if (path === '/auth/login') { data = { mfa_required: true, expires_at: '2099-01-01T00:00:00Z' }; status = 202; }
  else if (path === '/auth/mfa') { loggedIn = true; data = { user, csrf_token: 'fixture' }; }
  else if (path === '/auth/logout') { loggedIn = false; data = {}; }
  else if (path === '/auth/security') data = { totp_enabled: totpEnabled, recovery_codes_remaining: totpEnabled ? 10 : 0, passkey_available: false, passkey_reason: '로컬 UI 테스트', passkeys: [{ id: 'key', name: '개인 노트북', created_at: '2026-10-25', last_used_at: '2026-10-25' }] };
  else if (path === '/auth/reauth') data = { proof: 'fixture', expires_at: '2099-01-01T00:00:00Z' };
  else if (path === '/auth/totp/setup') data = { secret: 'JBSWY3DPEHPK3PXP', qr_png: 'data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+aqqsAAAAASUVORK5CYII=', expires_at: '2099-01-01T00:00:00Z' };
  else if (path === '/auth/totp/confirm') { totpEnabled = true; data = { user, csrf_token: 'rotated-fixture', recovery_codes: Array.from({length:10}, (_, i) => `ABCDE-FGHIJ-KLMNO-PQRST-UVWX${i}`) }; }
  else if (path === '/status') data = { version: 'precision-preview', started_at: '2026-10-25T00:00:00Z', plugins_active: 1, runs_queued: 2, runs_running: 1, runs_succeeded: 128, runs_failed: 0 };
  else if (path === '/menus') data = items([menu]);
  else if (path === '/users') data = items([user]);
  else if (path === '/plugins') data = items([plugin]);
  else if (path === '/plugins/demo') data = plugin;
  else if (path === '/plugins/demo/settings') data = { title: '테스트 설정', enabled: true, mode: 'daily' };
  else if (path === '/plugins/demo/resources/records') data = items([{ name: '긴 항목 '.repeat(15), state: 'active', value: 123 }]);
  else if (path === '/runs') data = items(['queued','running','succeeded','failed'].map((state,i) => ({ id: `run-${i}`, plugin_id:'demo', release_id:'release', task_id:'sync', state, progress:i*25, created_at:'2026-10-25T00:00:00Z', started_at:null, finished_at:null })));
  else if (path === '/logs') data = items(['INFO','WARN','ERROR'].map((level,i) => ({ id:i, timestamp:'2026-10-25T00:00:00Z', level, component:'modudock', plugin_id:null, message:'UI 확인용 로그 '.repeat(15) })));
  return route.fulfill({ status, json: data });
});
async function noOverflow(label) {
  assert.ok(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth), `${label}: horizontal overflow`);
}
async function capture(name) { await noOverflow(name); await page.screenshot({ path: fileURLToPath(new URL(`${name}.png`, output)), fullPage: true }); }
async function navigate(name, mobile) {
  if (mobile) await page.getByRole('button', { name: '메뉴 열기', exact: true }).click();
  const nav = page.getByRole('navigation', { name: mobile ? '모바일 내비게이션' : '주 내비게이션' });
  await nav.getByRole('button', { name, exact: true }).click();
  await page.getByRole('heading', {name, exact:true, level:1}).waitFor();
  if (mobile) await page.getByRole('dialog').waitFor({ state:'hidden' });
}
try {
  await page.goto('http://127.0.0.1:17441');
  await page.getByRole('heading', { name:'개요', exact:true }).waitFor();
  assert.equal(await page.evaluate(() => getComputedStyle(document.documentElement).getPropertyValue('--primary').trim()), 'oklch(0.575 0.176 268)', 'Precision primary token must reach the rendered app');
  assert.equal(await page.locator('.stat-card').first().evaluate(e => getComputedStyle(e).borderRadius), '6px', 'Precision card radius');
  for (const mobile of [false,true]) {
    await page.setViewportSize(mobile ? {width:390,height:844} : {width:1440,height:1000});
    for (const name of ['개요','작업 이력','플러그인','스케줄','사용자 계정','API 토큰','메뉴 편집','전체 로그','계정 보안','데모 작업']) {
      await navigate(name, mobile);
      await capture(`${mobile?'mobile':'desktop'}-${name}`);
      if (name === '스케줄' || name === '사용자 계정') {
        await page.getByRole('button', {name:name==='스케줄'?'스케줄 추가':'계정 추가',exact:true}).click();
        await page.getByLabel(name==='스케줄'?'이름':'사용자 이름',{exact:true}).fill('Precision 확인');
        await capture(`${mobile?'mobile':'desktop'}-${name}-editor`);
      }
    }
    await page.getByRole('tab', {name:'작업', exact:true}).click();
    await page.getByLabel('제목', {exact:true}).fill('한글 입력 유지');
    await page.getByRole('tab', {name:'설정', exact:true}).focus();
    await page.keyboard.press('Enter');
    await page.getByRole('heading', {name:'플러그인 설정',exact:true}).waitFor();
    await capture(`${mobile?'mobile':'desktop'}-plugin-form`);
    await navigate('계정 보안',mobile);
    await page.getByRole('button',{name:'2FA 설정',exact:true}).click();
    await page.getByLabel('재인증 비밀번호').fill('fixture-password');
    await page.getByRole('button',{name:'인증 후 계속'}).click();
    await page.getByLabel('인증 코드',{exact:true}).waitFor();
    await capture(`${mobile?'mobile':'desktop'}-totp`);
    if (!mobile) { await navigate('개요',false); await page.getByRole('button',{name:'사이드바 접기',exact:true}).click(); await capture('desktop-collapsed'); await page.getByRole('button',{name:'사이드바 펼치기',exact:true}).click(); }
  }
  await page.getByLabel('인증 코드',{exact:true}).fill('012345');
  await page.getByRole('button',{name:'인증 및 활성화'}).click();
  await page.getByText('ABCDE-FGHIJ-KLMNO-PQRST-UVWX0').waitFor();
  await capture('mobile-recovery');
  await page.getByRole('heading',{name:'계정 보안',exact:true,level:1}).waitFor();
  await page.getByText('복구 코드 10개 남음').waitFor();
  await page.setViewportSize({width:1440,height:1000});
  await capture('desktop-recovery-inline');
  await page.setViewportSize({width:390,height:844});
  assert.equal(await page.getByRole('button',{name:'저장 확인 후 닫기'}).isDisabled(),true);
  await page.getByLabel('복구 코드를 안전한 곳에 저장했습니다.').check();
  await page.getByRole('button',{name:'저장 확인 후 닫기'}).click();
  assert.equal(await page.getByText('ABCDE-FGHIJ-KLMNO-PQRST-UVWX0').count(),0);
  await page.setViewportSize({width:1440,height:1000});
  await page.getByRole('button',{name:'로그아웃',exact:true}).click();
  await page.setViewportSize({width:390,height:844});
  await page.getByLabel('사용자 이름').fill('precision-admin');
  await page.getByLabel('비밀번호',{exact:true}).fill('fixture-password');
  await capture('mobile-login');
  const input = page.getByLabel('비밀번호',{exact:true});
  await input.focus();
  assert.notEqual(await input.evaluate(e=>getComputedStyle(e).boxShadow),'none','Focused input must remain visible');
  await page.getByRole('button',{name:'로그인',exact:true}).click();
  await page.getByLabel('인증 코드',{exact:true}).waitFor();
  await capture('mobile-mfa');
  await page.getByLabel('인증 코드',{exact:true}).fill('012345');
  await page.getByRole('button',{name:'인증 후 로그인',exact:true}).click();
  await page.getByRole('heading',{name:'개요',exact:true}).waitFor();
  for (const width of [320,768,1024]) {
    await page.setViewportSize({width,height:900});
    await navigate('개요',width<768);
    await capture(`overview-${width}`);
    await navigate('계정 보안',width<768);
    await capture(`security-${width}`);
  }
  assert.deepEqual(errors,[]);
  console.log('PASS Precision tokens/cards, 20 menu views, account/schedule editors, tabs/forms, sidebar, 320/390/768/1024/1440px, TOTP/recovery/MFA fixtures, focus, no page errors.');
} finally { await browser.close(); }
