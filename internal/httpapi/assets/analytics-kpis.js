import {$, esc} from "./analytics-common.js";
import {METRICS, KPIS} from "./analytics-model.js";
import {state} from "./analytics-state.js";
import {syncURL} from "./analytics-filters.js";
import {renderChart} from "./analytics-chart.js";

// ---------- KPIs ----------
function delta(metric,cur,prev){
  const m=METRICS[metric];if(prev==null)return '';
  const a=m.value(cur),b=m.value(prev);
  let text,diff;
  if(m.pct){diff=a-b;text=(diff>0?'+':diff<0?'−':'±')+Math.abs(diff*100).toLocaleString('ru-RU',{maximumFractionDigits:1})+' п.п.'}
  else{if(!b)return a?'<span class="rp-delta flat">новое</span>':'';diff=(a-b)/b;text=(diff>0?'+':diff<0?'−':'±')+Math.abs(diff*100).toLocaleString('ru-RU',{maximumFractionDigits:diff&&Math.abs(diff)<.1?1:0})+' %'}
  const small=m.pct?Math.abs(diff)<.0005:Math.abs(diff)<.005;
  const cls=small||!m.better?'flat':(diff>0)===(m.better==='up')?'good':'bad';
  return `<span class="rp-delta ${cls}" title="Прошлый период: ${esc(m.fmt(b))}">${diff>0?'▲':diff<0?'▼':''}${esc(text)}</span>`;
}
function sparkline(values,color){
  if(!values||values.length<2)return '<svg viewBox="0 0 100 26" aria-hidden="true"></svg>';
  const max=Math.max(...values),min=Math.min(...values,0),span=max-min||1;
  const pts=values.map((v,i)=>`${(i/(values.length-1)*100).toFixed(2)},${(24-(v-min)/span*22).toFixed(2)}`);
  return `<svg viewBox="0 0 100 26" preserveAspectRatio="none" aria-hidden="true"><polyline points="${pts.join(' ')}" fill="none" stroke="${color}" stroke-width="1.6" vector-effect="non-scaling-stroke" stroke-linejoin="round"/></svg>`;
}
function renderKpis(){
  const d=state.data;if(!d)return;
  const tot=state.timeline?.total;
  $('#kpis').innerHTML=KPIS.map(k=>{const m=METRICS[k];return `<button type="button" class="rp-kpi ${state.chartMetric===k?'active':''}" data-kpi="${k}" aria-pressed="${state.chartMetric===k}" title="Показать на графике"><span class="rp-kpi-label" title="${esc(m.hint||m.label)}">${esc(m.kpi||m.label)}</span><span class="rp-kpi-value">${esc(m.fmt(m.value(d.totals)))}</span>${state.compare?delta(k,d.totals,state.prev?.totals)||'<span class="rp-delta flat">—</span>':''}${sparkline(tot?.map(m.value),state.chartMetric===k?'#6457ed':'#9aa1b4')}</button>`}).join('');
  for(const b of document.querySelectorAll('[data-kpi]'))b.onclick=()=>{state.chartMetric=b.dataset.kpi;$('#chart-metric').value=state.chartMetric;syncURL();renderKpis();renderChart()};
}


export {delta, sparkline, renderKpis};
