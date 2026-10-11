import {$, esc, fmtInt, fmtPct, plural} from "./analytics-common.js";
import {METRICS, dimLabel} from "./analytics-model.js";
import {state,colorFor,isTimeReport} from "./analytics-state.js";
import {drillURL, canFilter, rowKey, addFilters} from "./analytics-filters.js";


// ---------- Insights ----------
function renderInsights(){
  const el=$('#insights'),d=state.data;el.innerHTML='';
  if(!d||!d.totals.visits||isTimeReport())return;
  const dimKey=d.dimensions[0],rows=d.rows.filter(r=>r.level===1),T=d.totals,out=[];
  const minVisits=Math.max(10,T.visits*.03),name=r=>`«${dimLabel(dimKey,r.dimensions[0])}»`;
  const avgBot=METRICS.bot_share.value(T),avgConv=METRICS.conversion.value(T);
  if(!['quality','bots'].includes(state.report)){
    const bad=rows.filter(r=>r.visits>=minVisits&&METRICS.bot_share.value(r)>=Math.max(.25,avgBot*1.4)).sort((a,b)=>(b.bots+b.known_bots)-(a.bots+a.known_bots))[0];
    if(bad)out.push({cls:'bad',tag:'Боты',text:`${name(bad)}: ${fmtPct(METRICS.bot_share.value(bad))} визитов — боты (в среднем по отчёту ${fmtPct(avgBot)}). Это ${fmtInt(bad.bots+bad.known_bots)} ${plural(bad.bots+bad.known_bots,'визит','визита','визитов')}.`,href:drillURL(bad.dimensions,'BOT'),action:'Посмотреть ботов'});
  }
  const conv=rows.filter(r=>r.visits>=Math.max(20,T.visits*.03)&&r.converted_visits>0).sort((a,b)=>METRICS.conversion.value(b)-METRICS.conversion.value(a))[0];
  if(conv&&METRICS.conversion.value(conv)>=avgConv*1.25)out.push({cls:'good',tag:'Лучшая конверсия',text:`${name(conv)} конвертирует в заявку ${fmtPct(METRICS.conversion.value(conv))} визитов — в ${(METRICS.conversion.value(conv)/avgConv).toLocaleString('ru-RU',{maximumFractionDigits:1})} раза выше среднего.`,filter:conv.dimensions,action:'Оставить только этот сегмент'});
  if(T.leads>0){const waste=rows.filter(r=>r.visits>=Math.max(30,T.visits*.05)&&!r.leads).sort((a,b)=>b.visits-a.visits)[0];if(waste)out.push({cls:'warn',tag:'Без заявок',text:`${name(waste)}: ${fmtInt(waste.visits)} ${plural(waste.visits,'визит','визита','визитов')} и ни одной заявки за период.`,href:drillURL(waste.dimensions),action:'Открыть сессии'})}
  if(state.compare&&state.prev){
    const ch=rows.map(r=>{const p=state.prevIndex.get(rowKey(r.dimensions));return {r,diff:r.visits-(p?.visits||0),p}}).filter(c=>Math.abs(c.diff)>=Math.max(10,T.visits*.05)).sort((a,b)=>Math.abs(b.diff)-Math.abs(a.diff))[0];
    if(ch)out.push({cls:ch.diff>0?'good':'bad',tag:ch.diff>0?'Рост':'Падение',text:`${name(ch.r)}: ${ch.diff>0?'+':'−'}${fmtInt(Math.abs(ch.diff))} ${plural(Math.abs(ch.diff),'визит','визита','визитов')}${ch.p?.visits?` (${ch.diff>0?'+':'−'}${Math.round(Math.abs(ch.diff)/ch.p.visits*100)} %)`:''} к прошлому периоду — самое большое изменение в отчёте.`,filter:ch.r.dimensions,action:'Оставить только этот сегмент'});
  }
  el.innerHTML=out.slice(0,4).map((o,i)=>`<article class="rp-insight ${o.cls}"><small>${esc(o.tag)}</small><p>${esc(o.text)}</p>${o.href?`<a href="${esc(o.href)}">${esc(o.action)} →</a>`:canFilter(o.filter)?`<button type="button" data-insight="${i}">${esc(o.action)}</button>`:''}</article>`).join('');
  for(const b of el.querySelectorAll('[data-insight]'))b.onclick=()=>addFilters(out[Number(b.dataset.insight)].filter);
}


export {renderInsights};
