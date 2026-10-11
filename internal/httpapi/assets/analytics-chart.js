import {$, esc, SEQUENTIAL, fmtInt, pad, parseBucket, ruDate, MAX_SERIES} from "./analytics-common.js";
import {METRICS, weekdays, DIMS, REPORT_DIMS, dimLabel} from "./analytics-model.js";
import {state,colorFor,isTimeReport} from "./analytics-state.js";
import {drillURL} from "./analytics-filters.js";


let chartScale=null;
// ---------- Chart ----------
// Four gridline steps of a round size, just above the largest value.
function niceMax(v){if(v<=0)return 1;const raw=v/4,e=Math.pow(10,Math.floor(Math.log10(raw))),f=raw/e,step=[1,2,2.5,3,4,5,6,8,10].find(x=>f<=x)*e;return step*4}
function bucketLabel(b,interval,long=false){
  const d=parseBucket(b);
  if(interval==='hour')return long?`${ruDate(d,{weekday:'short',day:'numeric',month:'short'})}, ${pad(d.getHours())}:00–${pad(d.getHours())}:59`:(d.getHours()===0?ruDate(d,{day:'numeric',month:'short'}):`${pad(d.getHours())}:00`);
  if(interval==='day')return long?ruDate(d,{weekday:'short',day:'numeric',month:'long',year:'numeric'}):ruDate(d,{day:'numeric',month:'short'});
  if(interval==='week'){const e=new Date(d);e.setDate(e.getDate()+6);return long?`${ruDate(d,{day:'numeric',month:'short'})} — ${ruDate(e,{day:'numeric',month:'short',year:'numeric'})}`:ruDate(d,{day:'numeric',month:'short'})}
  return ruDate(d,{month:long?'long':'short',year:'numeric'});
}
function chartSeries(){
  const tl=state.timeline,m=METRICS[state.chartMetric];if(!tl)return [];
  const out=tl.series.map(s=>({id:s.key,name:dimLabel(REPORT_DIMS[state.report][0],s.key),color:colorFor(s.key),values:s.points.map(m.value)}));
  out.push({id:'__total',name:isTimeReport()?m.label:'Итого',color:'#18202f',values:tl.total.map(m.value),total:true});
  if(state.compare&&state.prevTimeline)out.push({id:'__prev',name:'Прошлый период',color:'#9aa1b4',values:tl.buckets.map((_,i)=>state.prevTimeline.total[i]?m.value(state.prevTimeline.total[i]):null),dashed:true,prev:true,labels:state.prevTimeline.buckets});
  return out;
}
function renderLegend(series,stacked){
  $('#chart-legend').innerHTML=series.length>1?series.map(s=>`<button type="button" data-series="${esc(s.id)}" class="${state.hidden.has(s.id)?'off':''}" aria-pressed="${!state.hidden.has(s.id)}" data-color="${s.color}"><i class="rp-key ${s.dashed?'dashed':stacked&&!s.total?'box':''}"></i><span>${esc(s.name)}</span></button>`).join(''):'';
  for(const b of document.querySelectorAll('[data-series]')){b.querySelector('i').style.color=b.dataset.color;b.onclick=()=>{const id=b.dataset.series;state.hidden.has(id)?state.hidden.delete(id):state.hidden.add(id);renderChart()}}
}
function renderChart(){
  chartScale=null;
  if(!REPORT_DIMS[state.report])return;
 setTimeout(()=>document.dispatchEvent(new Event("clickshield:chart")),0);
  const el=$('#report-chart'),m=METRICS[state.chartMetric];
  for(const b of document.querySelectorAll('[data-type]')){b.classList.toggle('active',b.dataset.type===state.chartType);b.disabled=b.dataset.type==='area'&&(!m.additive||isTimeReport())}
  $('#chart-type').classList.toggle('hidden',isTimeReport());
  if(state.report==='hours')return drawHourBars(el);
  if(state.report==='activity')return drawHeatmap(el);
  const tl=state.timeline;
  if(!tl||!tl.buckets.length){el.innerHTML='<div class="rp-empty-chart">Нет данных для графика</div>';$('#chart-legend').innerHTML='';return}
  const stacked=state.chartType==='area'&&m.additive&&!isTimeReport();
  const all=chartSeries(),series=all.filter(s=>!state.hidden.has(s.id));
  renderLegend(all,stacked);
  $('#chart-hint').textContent=isTimeReport()?'':`На графике — отмеченные в таблице строки (до ${MAX_SERIES}).`;
  const W=Math.max(el.clientWidth,320),H=290,ml=52,mr=14,mt=12,mb=30,iw=W-ml-mr,ih=H-mt-mb,n=tl.buckets.length;
  const x=i=>ml+(n===1?iw/2:i*iw/(n-1));
  const parts=series.filter(s=>!s.total&&!s.prev);
  const cum=stacked?parts.reduce((acc,s,j)=>{acc.push(s.values.map((v,i)=>(j?acc[j-1][i]:0)+(v||0)));return acc},[]):[];
  let max=0;for(const s of series)for(const v of s.values)if(v!=null)max=Math.max(max,v);for(const c of cum)for(const v of c)max=Math.max(max,v);
  if(m.pct)max=Math.min(1,niceMax(max*1.02));else max=niceMax(max*1.02);
  const y=v=>mt+ih-(v/max)*ih,ticks=[0,.25,.5,.75,1].map(t=>t*max);
  const axisFmt=m.axis||(v=>m.pct?Math.round(v*100)+' %':Math.abs(v)>=1e4?(v/1e3).toLocaleString('ru-RU',{maximumFractionDigits:1})+' тыс.':v.toLocaleString('ru-RU',{maximumFractionDigits:max<10?1:0}));
  const step=Math.max(1,Math.ceil(n/Math.max(2,Math.floor(iw/86))));
  let svg=`<svg viewBox="0 0 ${W} ${H}" height="${H}" role="img" aria-label="${esc(m.label)} по времени"><g class="grid">${ticks.map(t=>`<line x1="${ml}" x2="${W-mr}" y1="${y(t)}" y2="${y(t)}"/>`).join('')}</g>`;
  svg+=`<g class="axis">${ticks.map(t=>`<text x="${ml-8}" y="${y(t)+4}" text-anchor="end">${esc(axisFmt(t))}</text>`).join('')}${tl.buckets.map((b,i)=>i%step?'':`<text x="${x(i)}" y="${H-8}" text-anchor="${n===1?'middle':i===0?'start':i>=n-step?'end':'middle'}">${esc(bucketLabel(b,tl.interval))}</text>`).join('')}</g>`;
  const path=vals=>{let d='',pen=false;vals.forEach((v,i)=>{if(v==null){pen=false;return}d+=(pen?'L':'M')+x(i).toFixed(1)+','+y(v).toFixed(1);pen=true});return d};
  if(stacked)parts.forEach((s,j)=>{const top=cum[j],bottom=j?cum[j-1]:top.map(()=>0);const d=top.map((v,i)=>`${i?'L':'M'}${x(i).toFixed(1)},${y(v).toFixed(1)}`).join('')+bottom.map((v,i)=>[i,v]).reverse().map(([i,v])=>`L${x(i).toFixed(1)},${y(v).toFixed(1)}`).join('')+'Z';svg+=`<path d="${d}" fill="${s.color}" fill-opacity=".82" stroke="#fff" stroke-width="1.5"/>`});
  for(const s of series){if(stacked&&!s.total&&!s.prev)continue;svg+=`<path d="${path(s.values)}" fill="none" stroke="${s.color}" stroke-width="${s.total?2.4:2}" ${s.dashed?'stroke-dasharray="5 4"':''} stroke-linejoin="round" stroke-linecap="round"/>`;if(n===1)svg+=`<circle cx="${x(0)}" cy="${y(s.values[0]||0)}" r="4" fill="${s.color}"/>`}
  svg+=`<line class="crosshair hidden" x1="0" x2="0" y1="${mt}" y2="${mt+ih}"/><g class="dots"></g><rect class="hit" x="${ml}" y="${mt}" width="${iw}" height="${ih}"/></svg>`;
  el.innerHTML=svg;
  // Annotations place markers on this scale: bucket starts, interpolated within a bucket.
  const starts=tl.buckets.map(b=>parseBucket(b).getTime());
  chartScale={top:mt,bottom:mt+ih,toX(ms){
    if(n===1)return x(0);
    let i=starts.findLastIndex(t=>t<=ms);if(i<0)return null;if(i===n-1)i=n-2;
    const px=x(i)+(x(i+1)-x(i))*(ms-starts[i])/(starts[i+1]-starts[i]);return px<ml-1||px>W-mr+1?null:px;
  }};
  const root=el.querySelector('svg'),cross=root.querySelector('.crosshair'),dots=root.querySelector('.dots'),hit=root.querySelector('.hit');
  const rank=s=>s.total?0:s.prev?2:1;
  const show=(i,ev)=>{
    cross.classList.remove('hidden');cross.setAttribute('x1',x(i));cross.setAttribute('x2',x(i));
    dots.innerHTML=series.map((s,j)=>{const v=stacked&&!s.total&&!s.prev?cum[parts.indexOf(s)][i]:s.values[i];return v==null?'':`<circle cx="${x(i)}" cy="${y(v)}" r="4" fill="${s.color}" stroke="#fff" stroke-width="2"/>`}).join('');
    const rows=series.filter(s=>s.values[i]!=null).map(s=>({s,v:s.values[i]})).sort((a,b)=>rank(a.s)-rank(b.s)||b.v-a.v);
    tooltip(`<h4>${esc(bucketLabel(tl.buckets[i],tl.interval,true))}</h4>${rows.map(({s,v})=>`<div><i class="rp-key ${s.dashed?'dashed':''}" data-c="${s.color}"></i><strong>${esc(m.fmt(v))}</strong><span>${esc(s.prev&&s.labels?.[i]?'Прошлый период · '+bucketLabel(s.labels[i],tl.interval):s.name)}</span></div>`).join('')}`,ev);
  };
  const index=ev=>{const r=root.getBoundingClientRect(),px=(ev.clientX-r.left)*W/r.width;return Math.max(0,Math.min(n-1,Math.round(n===1?0:(px-ml)/iw*(n-1))))};
  hit.onpointermove=ev=>show(index(ev),ev);
  hit.onpointerleave=()=>{cross.classList.add('hidden');dots.innerHTML='';hideTooltip()};
}
function tooltip(html,ev){
  const t=$('#chart-tooltip');t.innerHTML=html;for(const k of t.querySelectorAll('[data-c]'))k.style.color=k.dataset.c;t.classList.remove('hidden');
  const r=t.getBoundingClientRect();let left=ev.clientX+16,top=ev.clientY+14;
  if(left+r.width>innerWidth-8)left=ev.clientX-r.width-16;if(top+r.height>innerHeight-8)top=ev.clientY-r.height-14;
  t.style.left=Math.max(8,left)+'px';t.style.top=Math.max(8,top)+'px';
}
function hideTooltip(){$('#chart-tooltip').classList.add('hidden')}
function drawHourBars(el){
  const m=METRICS[state.chartMetric],byHour=new Map(state.data.rows.filter(r=>r.level===1).map(r=>[r.dimensions[0],r]));
  $('#chart-legend').innerHTML='';$('#chart-hint').textContent='Сумма за выбранный период по часу начала визита.';
  const hours=[...Array(24)].map((_,h)=>pad(h)),vals=hours.map(h=>byHour.has(h)?m.value(byHour.get(h)):0);
  const W=Math.max(el.clientWidth,320),H=290,ml=52,mr=8,mt=12,mb=30,iw=W-ml-mr,ih=H-mt-mb,max=m.pct?Math.min(1,niceMax(Math.max(...vals)*1.05)):niceMax(Math.max(...vals)*1.05),bw=iw/24;
  const y=v=>mt+ih-v/max*ih,ticks=[0,.25,.5,.75,1].map(t=>t*max);
  el.innerHTML=`<svg viewBox="0 0 ${W} ${H}" height="${H}" role="img" aria-label="${esc(m.label)} по часам суток"><g class="grid">${ticks.map(t=>`<line x1="${ml}" x2="${W-mr}" y1="${y(t)}" y2="${y(t)}"/>`).join('')}</g><g class="axis">${ticks.map(t=>`<text x="${ml-8}" y="${y(t)+4}" text-anchor="end">${esc(m.pct?Math.round(t*100)+' %':m.axis?m.axis(t):fmtInt(t))}</text>`).join('')}${hours.map((h,i)=>i%(bw<22?3:1)?'':`<text x="${ml+bw*i+bw/2}" y="${H-8}" text-anchor="middle">${h}</text>`).join('')}</g>${vals.map((v,i)=>`<rect class="bar" data-i="${i}" x="${ml+bw*i+1}" y="${Math.min(y(v),mt+ih-1)}" width="${Math.max(1,bw-2)}" height="${Math.max(1,mt+ih-y(v))}" rx="3" fill="#2a78d6"/>`).join('')}</svg>`;
  for(const r of el.querySelectorAll('.bar')){const i=Number(r.dataset.i),row=byHour.get(hours[i]);r.onpointermove=ev=>tooltip(`<h4>${esc(DIMS.hour.fmt(hours[i]))}</h4><div><i class="rp-key box" data-c="#2a78d6"></i><strong>${esc(m.fmt(vals[i]))}</strong><span>${esc(m.label)}</span></div>${row&&state.chartMetric!=='visits'?`<div><i></i><strong>${fmtInt(row.visits)}</strong><span>визитов</span></div>`:''}`,ev);r.onpointerleave=hideTooltip;if(row)r.onclick=()=>location.href=drillURL(row.dimensions)}
}
function drawHeatmap(el){
  const m=METRICS[state.chartMetric],cells=new Map(state.data.rows.filter(r=>r.level===2).map(r=>[r.dimensions.join(':'),r]));
  $('#chart-legend').innerHTML='';$('#chart-hint').textContent='Чем темнее ячейка, тем выше значение. Нажмите ячейку, чтобы открыть сессии.';
  let max=0;for(const r of cells.values())max=Math.max(max,m.value(r));
  const color=v=>!v||!max?'':SEQUENTIAL[Math.min(SEQUENTIAL.length-1,Math.floor(v/max*(SEQUENTIAL.length-1)))];
  let html='<div class="rp-heatmap"><span></span>'+[...Array(24)].map((_,h)=>`<span>${h%3?'':pad(h)}</span>`).join('');
  for(let d=1;d<=7;d++){html+=`<span>${['','Пн','Вт','Ср','Чт','Пт','Сб','Вс'][d]}</span>`;for(let h=0;h<24;h++){const k=`${d}:${pad(h)}`,r=cells.get(k);html+=`<span class="cell" data-k="${k}" data-bg="${color(r?m.value(r):0)}" role="img" aria-label="${esc(weekdays[d]+', '+pad(h)+':00 — '+m.fmt(r?m.value(r):0))}"></span>`}}
  el.innerHTML=html+`</div><div class="rp-heatmap-scale"><span>${esc(m.fmt(0))}</span><i></i><span>${esc(m.fmt(max))}</span></div>`;
  el.querySelector('.rp-heatmap-scale i').style.background=`linear-gradient(90deg,${SEQUENTIAL[0]},${SEQUENTIAL[SEQUENTIAL.length-1]})`;
  for(const c of el.querySelectorAll('.cell')){
    if(c.dataset.bg)c.style.background=c.dataset.bg;
    const r=cells.get(c.dataset.k),[d,h]=c.dataset.k.split(':');
    c.onpointermove=ev=>tooltip(`<h4>${esc(weekdays[d]+', '+h+':00–'+h+':59')}</h4><div><i class="rp-key box" data-c="${c.dataset.bg||'#d9dde8'}"></i><strong>${esc(m.fmt(r?m.value(r):0))}</strong><span>${esc(m.label)}</span></div>`,ev);
    c.onpointerleave=hideTooltip;if(r)c.onclick=()=>location.href=drillURL(r.dimensions);
  }
}


export {chartScale, niceMax, bucketLabel, chartSeries, renderLegend, renderChart, tooltip, hideTooltip, drawHourBars, drawHeatmap};
