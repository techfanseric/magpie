// Optional macOS quota rings: five languages in both browser engines,
// preserving the selected cards and numeric-logo preference across saves
// and reloads. All API responses are local fixtures.
const assert = require("node:assert/strict");
const fs = require("node:fs/promises");
const path = require("node:path");
const { test } = require("node:test");
const { chromium, webkit } = require("playwright");
const { openPart } = require("./reader.cjs");

const assets = process.env.MAGPIE_RING_ASSETS || path.resolve(__dirname, "../assets");

const quotas = [
  { provider: "claude", name: "Claude Code", icon: "claude-color", user: "a@b.c", plan: "Max",
    windows: [{ name: "5-hour", used: 42 }, { name: "Weekly", used: 18 }] },
];

function settingsPayload(over) {
  return {
    theme: "light", lang: "en", tray: "panel", quotaLeft: false, currency: "usd",
    dock: false, dockWindow: false, proxy: "", redact: false, redactPersonal: false, redactWords: [],
    codexWarmup: "", claudeWarmup: "", codexWarmAt: "", claudeWarmAt: "", workbuddyCheckin: false, noStats: false,
    trayUsage: "claude|a@b.c", trayUsages: ["claude|a@b.c"], trayUsageEvery: 3, vision: "", imageGen: "",
    version: "0.1.400", dir: "~/.config/magpie", gateway: "http://127.0.0.1:3425",
    proxyNow: "none", proxySource: "none", login: false,
    visionModels: [], imageGenModels: [], workbuddyCheckins: [], lanURLs: [],
    fx: { rate: 7.2, at: new Date().toISOString(), stale: false },
    ...over,
  };
}

function server(lang, posts, over) {
  let cur = settingsPayload({ lang, ...over });
  return async (route) => {
    const req = route.request(), url = new URL(req.url());
    const json = (data) => route.fulfill({ json: data });
    if (url.pathname === "/boot.js") return route.fulfill({ contentType: "text/javascript", body: `window.bootPrefs = {lang:"${lang}",theme:"light",web:false};` });
    if (url.pathname === "/wails/runtime.js") return route.fulfill({ contentType: "text/javascript", body: "export const Window = {};" });
    if (url.pathname === "/api/state") return json({ agents: [], profiles: [], settings: { lang, theme: "light" }, fx: cur.fx });
    if (url.pathname === "/api/settings") {
      if (req.method() === "POST") {
        const body = req.postDataJSON();
        posts.push(body);
        cur = { ...cur, ...body, trayUsage: (body.trayUsages || [])[0] || "" };
      }
      return json(cur);
    }
    if (url.pathname === "/api/usage/quotas") return json(quotas);
    if (url.pathname === "/api/plugins") return json({ plugins: [] });
    if (url.pathname === "/api/groups") return json({ groups: [], models: [] });
    if (url.pathname.startsWith("/api/")) return json({});
    const file = path.join(assets, url.pathname === "/" ? "index.html" : url.pathname);
    const contentType = { ".html": "text/html", ".js": "text/javascript", ".css": "text/css", ".svg": "image/svg+xml", ".png": "image/png" }[path.extname(file)];
    await route.fulfill({ body: await fs.readFile(file), contentType });
  };
}

const view = (page) => page.locator("#view-settings").evaluate((v) => v.scrollTop);

async function open(browser, lang, posts, platform, over) {
  const context = await browser.newContext({ viewport: { width: 900, height: 420 }, reducedMotion: "reduce" });
  await context.addInitScript((p) => Object.defineProperty(Navigator.prototype, "platform", { get: () => p }), platform);
  const page = await context.newPage();
  page.setDefaultTimeout(5000);
  page.setDefaultNavigationTimeout(60000);
  await page.route("**/*", server(lang, posts, over));
  await page.goto("http://magpie.test/?view=usage");
  await page.locator("#prefs").click();
  await openPart(page, "usage");
  await page.locator("#trayUsagePick button").waitFor();
  return { context, page };
}


for (const engine of (process.env.BROWSER ? [process.env.BROWSER] : ["chromium", "webkit"])) {
  test(engine + ": optional compact rings persist without changing other preferences", async (t) => {
    const browser = await (engine === "webkit" ? webkit.launch() : chromium.launch({ channel: "chromium" }));
    t.after(() => browser.close());
    for (const [lang, name] of [["en", "Compact quota rings"], ["zh", "紧凑额度环"], ["zh-TW", "精簡額度環"], ["ja", "コンパクトな利用枠リング"], ["de", "Kompakte Kontingentringe"]]) {
      await t.test(lang, async () => {
        const posts = [], errors = [];
        const {context,page} = await open(browser,lang,posts,"MacIntel", {trayNoLogos:true});
        page.on("pageerror", e=>errors.push(e.message));
        const row=page.locator("#trayRingsRow"), opts=row.locator(".opt");
        await row.waitFor();
        assert.equal((await row.locator(".name").textContent()).trim(),name);
        assert.equal(await opts.nth(0).evaluate(b=>b.classList.contains("on")),true,"existing installations keep numbers");
        await row.scrollIntoViewIfNeeded();
        await Promise.all([page.waitForResponse(r=>new URL(r.url()).pathname==="/api/settings" && r.request().method()==="POST"),opts.nth(1).click()]);
        await page.waitForFunction(()=>document.querySelector("#trayRingsSegs [data-seg=on]").classList.contains("on"));
        assert.equal(posts.at(-1).trayRings,true);
        assert.equal(posts.at(-1).trayNoLogos,true,"numeric-logo preference retained");
        assert.equal(await page.locator("#trayLogosRow").isVisible(),false,"logo control is inapplicable to rings");
        // Another preference save and a reload retain the ring style.
        let sent=posts.length;
        await page.locator("#currencySegs .opt").nth(1).evaluate(b=>b.click());
        await page.waitForFunction(n=>document.querySelector("#currencySegs [data-seg=cny]").classList.contains("on"),sent);
        assert.equal(posts.at(-1).trayRings,true);
        await page.reload();
        await page.locator("#prefs").click();
        await openPart(page,"usage");
        await page.locator("#trayRingsRow").waitFor();
        assert.equal(await page.locator("#trayRingsSegs .opt").nth(1).evaluate(b=>b.classList.contains("on")),true);
        await page.setViewportSize({width:420,height:600});
        await page.waitForTimeout(300);
        const overflow=await row.evaluate(r=>({right:r.getBoundingClientRect().right,width:innerWidth,document:document.documentElement.scrollWidth,scroll:r.scrollWidth,client:r.clientWidth,parts:[...r.children].map(e=>({class:e.className,right:e.getBoundingClientRect().right,width:e.getBoundingClientRect().width}))}));
        assert(overflow.right<=overflow.width+1 && overflow.scroll<=overflow.client+1 && overflow.parts.every(p=>p.right<=overflow.width+1),"narrow ring row overflows: "+JSON.stringify(overflow));
        await page.locator("#trayRingsRow").scrollIntoViewIfNeeded();
        await Promise.all([page.waitForResponse(r=>new URL(r.url()).pathname==="/api/settings" && r.request().method()==="POST"),page.locator("#trayRingsSegs .opt").nth(0).click()]);
        await page.waitForFunction(()=>document.querySelector("#trayLogosRow").hidden===false);
        assert.equal(posts.at(-1).trayRings,false);
        assert.equal(posts.at(-1).trayNoLogos,true);
        assert.deepEqual(errors,[]);
        if(process.env.ARTIFACT_DIR && lang==="en") {
          await fs.mkdir(process.env.ARTIFACT_DIR,{recursive:true});
          await page.screenshot({path:path.join(process.env.ARTIFACT_DIR,engine+"-ring-settings.png")});
        }
        await context.close();
      });
    }
    for(const [platform,over] of [["MacIntel",{trayUsage:"",trayUsages:[]}],["Win32",{}],["Linux x86_64",{}]]) {
      const {context,page}=await open(browser,"en",[],platform,over);
      await page.waitForTimeout(200);
      assert.equal(await page.locator("#trayRingsRow").isVisible(),false,platform+JSON.stringify(over));
      await context.close();
    }
  });
}
