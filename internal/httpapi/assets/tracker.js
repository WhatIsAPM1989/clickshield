(() => {
  'use strict';
  try {
    const script = document.currentScript || Array.from(document.scripts).find((s) => /\/tracker\.js(?:\?|$)/.test(s.src));
    if (!script || !script.src) return;
    const scriptURL = new URL(script.src, document.baseURI);
    const key = scriptURL.searchParams.get('key') || script.dataset.projectKey || '';
    if (!key) return;

    const apiBase = scriptURL.origin;
    const eventsURL = `${apiBase}/v1/events?key=${encodeURIComponent(key)}`;
    const finishURL = `${apiBase}/v1/session/finish?key=${encodeURIComponent(key)}`;
    const flushEvery = Math.max(2000, Number(script.dataset.flushMs || 5000));
    const mouseEvery = Math.max(50, Number(script.dataset.mouseSampleMs || 90));
    const yandexCounter = script.dataset.yandexCounter || '';
    const googleMeasurement = script.dataset.googleMeasurement || '';
    const now = Date.now();
    const safeURL = (value) => {
      try {
        if (!value) return "";
        const u = new URL(value, location.href);
        if (!["http:","https:"].includes(u.protocol)) return "";
        const clean = new URLSearchParams();
        ['utm_source','utm_medium','utm_campaign','utm_term','utm_content','yclid','gclid','rb_clickid'].forEach((name) => {
          const v = u.searchParams.get(name); if (v) clean.set(name, v.slice(0, 256));
        });
        u.username = ''; u.password = ''; u.hash = ''; u.search = clean.toString();
        return u.toString().slice(0, 2048);
      } catch (_) { return ''; }
    };
    const safeStorage = (storage) => ({
      get(k) { try { return storage.getItem(k); } catch (_) { return null; } },
      set(k, v) { try { storage.setItem(k, v); } catch (_) {} }
    });
    const local = safeStorage(window.localStorage);
    const session = safeStorage(window.sessionStorage);
    const uuid = () => {
      const cryptoAPI = window.crypto;
      if (cryptoAPI?.randomUUID) return cryptoAPI.randomUUID();
      const b = new Uint8Array(16);
      if (cryptoAPI?.getRandomValues) cryptoAPI.getRandomValues(b); else for (let i = 0; i < b.length; i++) b[i] = Math.floor(Math.random() * 256);
      b[6] = (b[6] & 15) | 64; b[8] = (b[8] & 63) | 128;
      const h = [...b].map((x) => x.toString(16).padStart(2, '0'));
      return `${h.slice(0,4).join('')}-${h.slice(4,6).join('')}-${h.slice(6,8).join('')}-${h.slice(8,10).join('')}-${h.slice(10).join('')}`;
    };

    const visitorKey = `clickshield:visitor:${key}`;
    let visitorID = local.get(visitorKey);
    if (!visitorID || !/^[0-9a-f-]{36}$/i.test(visitorID)) { visitorID = uuid(); local.set(visitorKey, visitorID); }

    const stateKey = `clickshield:session:${key}`;
    let state = null;
    try { state = JSON.parse(session.get(stateKey) || 'null'); } catch (_) {}
    if (!state || !state.id || now - Number(state.last || 0) > 30 * 60 * 1000) {
      state = { id: uuid(), created: now, last: now, pageIndex: 0, landing: safeURL(location.href), referrer: safeURL(document.referrer), attribution: Object.fromEntries(new URL(safeURL(location.href)).searchParams) };
    }
    state.last = now; state.pageIndex = Number(state.pageIndex || 0) + 1;
    session.set(stateKey, JSON.stringify(state));

    const params = new URLSearchParams(state.attribution || new URL(state.landing || location.href).search);
    const utm = {
      source: (params.get('utm_source') || '').slice(0, 256),
      medium: (params.get('utm_medium') || '').slice(0, 256),
      campaign: (params.get('utm_campaign') || '').slice(0, 256),
      term: (params.get('utm_term') || '').slice(0, 256),
      content: (params.get('utm_content') || '').slice(0, 256)
    };
    const yclid = (params.get('yclid') || '').slice(0, 256);
    const gclid = (params.get('gclid') || '').slice(0, 256);
    session.set(`clickshield:yclid:${key}`, '');
    session.set(`clickshield:gclid:${key}`, '');

    const rb_clickid = (params.get('rb_clickid') || state.rbClickID || '').slice(0, 256);
    state.rbClickID = rb_clickid;
    session.set(stateKey, JSON.stringify(state));

    const cookie = (name) => document.cookie.split(';').map((x) => x.trim()).find((x) => x.startsWith(`${name}=`))?.slice(name.length + 1) || '';
    let yandexClientID = cookie('_ym_uid').replace(/\D/g, '').slice(0, 32);
    let googleClientID = (() => { const p = decodeURIComponent(cookie('_ga')).split('.'); return p.length >= 4 ? `${p[p.length-2]}.${p[p.length-1]}` : ''; })();
    try {
      if (yandexCounter && typeof window.ym === 'function') window.ym(Number(yandexCounter), 'getClientID', (id) => { if (id) yandexClientID = String(id).replace(/\D/g, '').slice(0, 32); });
      if (googleMeasurement && typeof window.gtag === 'function') window.gtag('get', googleMeasurement, 'client_id', (id) => { if (id) googleClientID = String(id).slice(0, 65); });
    } catch (_) {}

    const automation = {
      webdriver: navigator.webdriver === true,
      selenium: Boolean(window.document.documentElement.getAttribute('webdriver') || window._Selenium_IDE_Recorder || window.__selenium_unwrapped),
      phantom: Boolean(window.callPhantom || window._phantom),
      playwright: Boolean(window.__playwright__binding__ || window.__pwInitScripts),
      puppeteer: Object.keys(window).some((k) => /^cdc_[a-z0-9]+_/i.test(k))
    };
    const deviceType = /ipad|tablet/i.test(navigator.userAgent) ? 'tablet' : (/mobi|android|iphone|ipod/i.test(navigator.userAgent) || navigator.maxTouchPoints > 1) ? 'mobile' : 'desktop';
    const device = {
      type: deviceType,
      platform: String(navigator.userAgentData?.platform || navigator.platform || '').slice(0, 80),
      language: String(navigator.language || '').slice(0, 32),
      timezone: (() => { try { return Intl.DateTimeFormat().resolvedOptions().timeZone || ''; } catch (_) { return ''; } })(),
      screen_width: screen.width || 0, screen_height: screen.height || 0, color_depth: screen.colorDepth || 0,
      hardware_concurrency: navigator.hardwareConcurrency || 0, device_memory: navigator.deviceMemory || 0,
      max_touch_points: navigator.maxTouchPoints || 0
    };

    let fingerprint = '';
    const fpParts = [navigator.userAgent, device.platform, device.language, device.timezone, `${screen.width}x${screen.height}x${screen.colorDepth}`, navigator.hardwareConcurrency, navigator.deviceMemory, navigator.maxTouchPoints].join('|');
    (async () => {
      try {
        const digest = await window.crypto.subtle.digest('SHA-256', new TextEncoder().encode(fpParts));
        fingerprint = Array.from(new Uint8Array(digest)).map((b) => b.toString(16).padStart(2, '0')).join('');
      } catch (_) {
        let h = 2166136261; for (let i = 0; i < fpParts.length; i++) h = Math.imul(h ^ fpParts.charCodeAt(i), 16777619);
        fingerprint = `fnv-${(h >>> 0).toString(16)}`;
      }
    })();

    let queue = [];
    let lastMouse = 0, lastScroll = 0, lastActivity = now, lastTick = now, activeDelta = 0, idleDelta = 0;
    let flushing = false, pendingBatch = null, decision = 'MONITOR';
    const event = (type, data = {}) => {
      queue.push({ type, ts: Date.now(), page_url: safeURL(location.href), page_index: state.pageIndex, ...data });
      if (queue.length >= 100) flush(false);
    };
    const activity = () => { lastActivity = Date.now(); };
    const heartbeat = () => {
      const t = Date.now(); const delta = Math.min(5000, Math.max(0, t - lastTick));
      if (!document.hidden && t - lastActivity < 5000) activeDelta += delta; else idleDelta += delta;
      lastTick = t;
    };
    setInterval(heartbeat, 1000);

    const emitScroll=()=>{const d=Math.max(document.documentElement.scrollHeight,document.body?.scrollHeight||0);event('scroll',{scroll_y:scrollY,depth:d>0?Math.min(1,(scrollY+innerHeight)/d):0,vw:innerWidth,vh:innerHeight,document_height:d})};
    event('pageview', { page_index: state.pageIndex, extra: {page_url:safeURL(location.href)} });
    emitScroll();
    addEventListener('resize',emitScroll,{passive:true});
    event('automation', { extra: automation });
    let lastPageURL=location.href;
    addEventListener('clickshield:pageview', () => {
      if(lastPageURL===location.href)return;lastPageURL=location.href;
      state.pageIndex += 1;
      state.last = Date.now();
      session.set(stateKey, JSON.stringify(state));
      event('pageview', { page_index: state.pageIndex, extra: {page_url:safeURL(location.href)} });
      emitScroll();
      flush(false);
    });
    addEventListener('mousemove', (e) => { const t = Date.now(); if (t - lastMouse < mouseEvery) return; lastMouse = t; activity(); event('mousemove', { x: e.clientX, y: e.clientY, vw: innerWidth, vh: innerHeight, pointer: 'mouse' }); }, { passive: true });
    addEventListener('pointerdown', (e) => { activity(); event('click', { x: e.clientX, y: e.clientY, vw: innerWidth, vh: innerHeight, pointer: e.pointerType || '', button: e.button, scroll_y: scrollY, document_height: Math.max(document.documentElement.scrollHeight,document.body?.scrollHeight||0) }); }, { passive: true, capture: true });
    addEventListener('scroll', () => { const t = Date.now(); if (t - lastScroll < 150) return; lastScroll = t; activity(); const d = Math.max(document.documentElement.scrollHeight, document.body?.scrollHeight || 0); const depth = d > 0 ? Math.min(1, (scrollY + innerHeight) / d) : 0; event('scroll', { scroll_y: scrollY, depth, vw: innerWidth, vh: innerHeight, document_height: d }); }, { passive: true });
    addEventListener('touchstart', (e) => { activity(); event('touch', { extra: { touches: Math.min(10, e.touches.length) } }); }, { passive: true });
    document.addEventListener('visibilitychange', () => { heartbeat(); event('visibility', { visible: !document.hidden }); if (document.hidden) flush(false); }, { passive: true });
    ['keydown', 'focus'].forEach((name) => addEventListener(name, activity, { passive: true }));

    const payload = (events, batchID = uuid(), finish = false) => ({
      session_id: state.id, visitor_id: visitorID, batch_id: batchID, sent_at: Date.now(), finish,
      page_url: safeURL(location.href), landing_url: safeURL(state.landing || location.href),
      referrer: state.referrer || "", yandex_client_id: yandexClientID, google_client_id: googleClientID,
      yclid, gclid, rb_clickid, utm, fingerprint, device, automation, page_index: state.pageIndex, events
    });
    const addHeartbeat = () => { heartbeat(); if (activeDelta || idleDelta) { queue.push({ type: 'heartbeat', ts: Date.now(), active_ms: Math.round(activeDelta), idle_ms: Math.round(idleDelta) }); activeDelta = 0; idleDelta = 0; } };
    const handleDecision = (value) => {
      if (!value || value === decision) return; decision = value;
      document.documentElement.dataset.antifraudDecision = value.toLowerCase();
      dispatchEvent(new CustomEvent('antifraud:decision', { detail: { decision: value, sessionId: state.id } }));
    };
    async function flush(beacon) {
      addHeartbeat();
      // Background heartbeats must not keep the visit alive: it lasts until 30 minutes after the last interaction.
      state.last = Math.max(Number(state.last || 0), lastActivity); session.set(stateKey, JSON.stringify(state));
      if (beacon) {
        if (!navigator.sendBeacon) return;
        if (pendingBatch) {
          const finalBody = JSON.stringify(payload(pendingBatch.events, pendingBatch.batchID, true));
          navigator.sendBeacon(eventsURL, new Blob([finalBody], { type: 'text/plain;charset=UTF-8' }));
        }
        if (queue.length) {
          const events = queue.splice(0, 200); const body = JSON.stringify(payload(events, uuid(), true));
          navigator.sendBeacon(eventsURL, new Blob([body], { type: 'text/plain;charset=UTF-8' }));
        }
        return;
      }
      if (flushing) return;
      if (!pendingBatch) {
        if (!queue.length) return;
        const events = queue.splice(0, 200); const batchID = uuid();
        pendingBatch = { events, batchID, body: JSON.stringify(payload(events, batchID)) };
      }
      flushing = true;
      try {
        const response = await fetch(eventsURL, { method: 'POST', mode: 'cors', credentials: 'omit', keepalive: true, headers: { 'Content-Type': 'text/plain;charset=UTF-8' }, body: pendingBatch.body });
        if (!response.ok) throw new Error(`HTTP ${response.status}`);
        const result = await response.json().catch(() => null); handleDecision(result?.decision);
        pendingBatch = null;
      } catch (_) {
        // Keep the exact batch and batch_id for an idempotent retry.
      } finally {
        flushing = false;
        if (!pendingBatch && queue.length >= 100) setTimeout(() => flush(false), 0);
      }
    }
    setInterval(() => flush(false), flushEvery);
    addEventListener('pagehide', () => {
      flush(true);
      const finish = JSON.stringify({ session_id: state.id, visitor_id: visitorID, batch_id: uuid(), reason: 'pagehide', sent_at: Date.now() });
      if (navigator.sendBeacon) navigator.sendBeacon(finishURL, new Blob([finish], { type: 'text/plain;charset=UTF-8' }));
    }, { capture: true });
    const track = (name,properties={}) => {
      if(typeof name!=='string'||!name.trim()||[...name.trim()].length>80)throw new TypeError('Event name must contain 1–80 characters');
      if(!properties||typeof properties!=='object'||Array.isArray(properties)||Object.keys(properties).length>32)throw new TypeError('Expected up to 32 scalar properties');
      const clean={};for(const [k,v] of Object.entries(properties)){if(!k.trim()||new TextEncoder().encode(k).length>80||!(v===null||['string','number','boolean'].includes(typeof v))||(typeof v==='number'&&!Number.isFinite(v))||(typeof v==='string'&&new TextEncoder().encode(v).length>1024))throw new TypeError('Invalid event property');Object.defineProperty(clean,k,{value:v,enumerable:true});}
      if(new TextEncoder().encode(JSON.stringify(clean)).length>8192)throw new TypeError('Event properties exceed 8 KiB');
      event('business',{name:name.trim(),properties:clean});return true;
    };
    document.addEventListener('click',e=>{const el=e.target?.closest?.('[data-cs-event]');if(!el)return;try{const p={};for(const [k,v] of Object.entries(el.dataset))if(k.startsWith('csProp'))p[k.slice(6).replace(/^./,c=>c.toLowerCase())]=v;track(el.dataset.csEvent,p);}catch(_){}},{capture:true});
    window.ClickShield = Object.freeze({ track, sessionId: state.id, visitorId: visitorID, pageIndex: () => state.pageIndex, flush: () => flush(false), getDecision: () => decision });
    for(const method of ['pushState','replaceState']){const original=history[method];history[method]=function(...args){const result=original.apply(this,args);dispatchEvent(new Event('clickshield:pageview'));return result}}
    addEventListener('popstate',()=>dispatchEvent(new Event('clickshield:pageview')));
    flush(false);
    fetch(`${apiBase}/v1/tracker/config?key=${encodeURIComponent(key)}`,{mode:'cors',credentials:'omit'}).then(r=>r.ok?r.json():null).then(cfg=>{
      if(!cfg?.replay_enabled)return;
      let sample=state.replaySample;if(typeof sample!=='number'){sample=Math.random()*100;state.replaySample=sample;session.set(stateKey,JSON.stringify(state))}
      if(sample>=cfg.replay_sample_rate)return;
      const loader=document.createElement('script');loader.src=`${apiBase}/assets/replay-recorder.js`;loader.async=true;
      loader.onload=()=>{try{window.ClickShieldReplay?.start({apiBase,key,sessionId:state.id,visitorId:visitorID,pageIndex:()=>state.pageIndex,safeURL})}catch(_){}};
      document.head.append(loader);
    }).catch(()=>{});
  } catch (_) {
    // Tracker failures are intentionally isolated from the host website.
  }
})();
