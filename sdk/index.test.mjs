import test from 'node:test';
import assert from 'node:assert/strict';
import {ClickShieldAnalytics} from './index.js';
test('SDK authenticates scoped reads and preserves typed dates/filters',async()=>{
 const calls=[];const api=new ClickShieldAnalytics({baseURL:'https://analytics.test',apiKey:'csak_test',fetch:async(url,opts)=>{calls.push({url,opts});return {ok:true,json:async()=>({data:[{name:'call',events:1}]})}}});
 const r=await api.behavior('events',{from:new Date('2026-01-01T00:00:00Z'),classification:'NO_BOTS'});
 assert.equal(r[0].name,'call');assert.equal(calls[0].opts.headers.Authorization,'Bearer csak_test');assert.equal(calls[0].url.searchParams.get('from'),'2026-01-01T00:00:00.000Z');assert.equal(calls[0].opts.redirect,'error');
 await assert.rejects(()=>api.behavior('../settings'),/Unknown/);
});
test('SDK exposes revoked-key status to caller',async()=>{const api=new ClickShieldAnalytics({baseURL:'https://analytics.test',apiKey:'csak_test',fetch:async()=>({ok:false,status:401,json:async()=>({error:'revoked'})})});await assert.rejects(()=>api.report('sources'),e=>e.status===401&&e.message==='revoked')});
