import {projectPicker} from "./analytics-project-picker.js";
import {$, esc, zone, PALETTE, storage, fmtInt, plural, fmtRange, MAX_SERIES} from "./analytics-common.js";
import {METRICS, DIMS, REPORTS, REPORT_DIMS, FILTER_PARAMS} from "./analytics-model.js";
import {state,colorFor,isTimeReport} from "./analytics-state.js";
import {toast, api} from "./analytics-api.js";
import {defaultSort,effectiveInterval, previousDates, setPeriod, markPeriod, baseQuery, syncURL, rowKey, renderCatalog, switchReport, renderChips} from "./analytics-filters.js";
import {renderKpis} from "./analytics-kpis.js";
import {renderChart} from "./analytics-chart.js";
import {renderInsights} from "./analytics-insights.js";
import {renderColumnGroups, renderTable} from "./analytics-table.js";
import {exportReport} from "./analytics-export.js";
import {clearMetrika, loadMetrikaReport} from "./analytics-metrika.js";
import "./product-analytics.js";
import "./analytics-tools.js";

// ---------- Loading ----------
// Behaviour reports, tools and the Metrica comparison live in their own panels under the same filters.
// Buttons that choose a project in place, for panels that work with one site only.
function showPanel(meta){
  const panel=meta.panel||'';
  // With a single site there is nothing to choose, so panels that need one project take it.
  const options=[...$('#project-filter').options].filter(o=>o.value);
  if(panel&&!$('#project-filter').value&&options.length===1&&(panel!=='product'||['goals','funnels'].includes(state.report))){$('#project-filter').value=options[0].value;syncURL()}
  $('#report-body').classList.toggle('hidden',!!panel);
  $('#product-panel').classList.toggle('hidden',panel!=='product');
  $('#tools-panel').classList.toggle('hidden',panel!=='tools');
  $('#metrika-panel').classList.toggle('hidden',panel!=='metrika');
  for(const id of ['export-report','interval-control','compare-control'])$('#'+id).classList.toggle('hidden',!!panel);
  $('#report-status').textContent=panel?`${fmtRange($('#date-from').value,$('#date-to').value)} · часовой пояс ${zone}`:'';
  if(panel==='product'){$('#product-kind').value=state.report;$('#new-definition').classList.toggle('hidden',!['goals','funnels'].includes(state.report));$('#product-kind').dispatchEvent(new Event('change'))}
  if(panel==='tools'){$('#tool-kind').value=state.report;$('#new-tool').textContent=({links:'Создать ссылку',pixels:'Создать пиксель',annotations:'Добавить отметку',keys:'Выпустить ключ'})[state.report];$('#tool-kind').dispatchEvent(new Event('change'))}
  return !!panel;
}
async function loadReport({keepSelection=false}={}){
 document.dispatchEvent(new Event("clickshield:filters"));
  if(!$('#report-filters').reportValidity())return;
  if($('#date-from').value>$('#date-to').value){toast('Начало периода должно быть раньше окончания',true);return}
  const request=++state.request,meta=REPORTS[state.report];
  ++state.timelineRequest;
  if(!keepSelection){state.selected=[];state.hidden.clear()}
  state.page=0;
  $('#report-title').textContent=meta.title;$('#report-description').textContent=meta.description;document.title=meta.title+' — ClickShield';
  renderCatalog();renderChips();markPeriod();syncURL();
  if(showPanel(meta))return;
  $('#report-body').classList.add('loading');$('#report-status').classList.remove('error');$('#report-status').textContent='Загружаем отчёт…';
  const prevDates=previousDates();
  try{
    const [data,prev]=await Promise.all([
      api('/v1/reports?'+baseQuery()),
      state.compare?api('/v1/reports?'+baseQuery(...prevDates)):Promise.resolve(null),
    ]);
    if(request!==state.request)return;
    state.data=data;state.prev=prev;state.prevIndex=new Map((prev?.rows||[]).map(r=>[rowKey(r.dimensions),r]));
    if(!keepSelection&&!isTimeReport())state.selected=data.rows.filter(r=>r.level===1).sort((a,b)=>b.visits-a.visits).slice(0,MAX_SERIES).map(r=>r.dimensions[0]);
    await loadTimeline();
    if(request!==state.request)return;
    renderAll();
    const t=data.totals.visits,rows=data.rows.filter(r=>r.level===1).length;
    $('#report-status').textContent=t?`${fmtInt(rows)} ${plural(rows,'строка','строки','строк')} · ${fmtRange($('#date-from').value,$('#date-to').value)}${state.compare?` · сравнение с ${fmtRange(...prevDates)}`:''} · часовой пояс ${zone}`:'За выбранный период визитов нет.';
    $('#export-report').disabled=!data.rows.length;
  }catch(e){
    if(request!==state.request)return;
    $('#report-status').textContent='Отчёт не загружен: '+e.message;$('#report-status').classList.add('error');
  }finally{if(request===state.request)$('#report-body').classList.remove('loading')}
}
async function loadTimeline(){
  const request=++state.timelineRequest,interval=effectiveInterval();
  const q=baseQuery();q.set('interval',interval);
  if(isTimeReport()||!state.selected.length)q.set('total_only','true');else for(const k of state.selected)q.append('series',k);
  const prevQ=state.compare?(()=>{const p=baseQuery(...previousDates());p.set('interval',interval);p.set('total_only','true');return p})():null;
  try{
    const [tl,prev]=await Promise.all([api('/v1/reports/timeline?'+q),prevQ?api('/v1/reports/timeline?'+prevQ):Promise.resolve(null)]);
    if(request!==state.timelineRequest)return;
    state.timeline=tl;state.prevTimeline=prev;
    for(const s of tl.series)colorFor(s.key);
  }catch(e){if(request===state.timelineRequest){state.timeline=null;state.prevTimeline=null}}
}
async function reloadTimeline(){
  const request=state.request;
  $('#report-chart').classList.add('loading');
  await loadTimeline();
  if(request!==state.request)return;
  $('#report-chart').classList.remove('loading');renderChart();renderKpis();renderTable();
}
function renderAll(){renderKpis();renderChart();renderInsights();renderColumnGroups();renderTable()}

// ---------- Wiring ----------
function bind(){
  const reload=()=>{clearMetrika();loadReport()};
  $('#report-filters').onsubmit=e=>{e.preventDefault();reload()};
  for(const id of ['project-filter','date-from','date-to'])$('#'+id).onchange=reload;
  for(const b of document.querySelectorAll('[data-period]'))b.onclick=()=>{setPeriod(b.dataset.period);reload()};
  for(const b of document.querySelectorAll('[data-class]'))b.onclick=()=>{state.segment=b.dataset.class;loadReport({keepSelection:true})};
  $('#interval').onchange=()=>{state.interval=$('#interval').value;markPeriod();syncURL();reloadTimeline()};
  $('#compare').onchange=()=>{state.compare=$('#compare').checked;loadReport({keepSelection:true})};
  $('#chart-metric').innerHTML=Object.entries(METRICS).map(([k,m])=>`<option value="${k}">${esc(m.label)}</option>`).join('');
  $('#chart-metric').onchange=()=>{state.chartMetric=$('#chart-metric').value;syncURL();renderKpis();renderChart()};
  for(const b of document.querySelectorAll('[data-type]'))b.onclick=()=>{state.chartType=b.dataset.type;storage.set('chart-type',state.chartType);renderChart()};
  $('#report-select').onchange=()=>switchReport($('#report-select').value);
  let searchTimer;$('#table-search').oninput=()=>{clearTimeout(searchTimer);searchTimer=setTimeout(()=>{state.search=$('#table-search').value.trim().toLowerCase();state.page=0;renderTable()},150)};
  $('#toggle-expand').onclick=()=>{state.expandAll=!state.expandAll;if(!state.expandAll)state.expanded.clear();renderColumnGroups();renderTable()};
  $('#report-prev').onclick=()=>{state.page--;renderTable()};$('#report-next').onclick=()=>{state.page++;renderTable()};
  $('#export-report').onclick=exportReport;
  $('#copy-link').onclick=async()=>{try{await navigator.clipboard.writeText(location.href);toast('Ссылка на отчёт скопирована')}catch{toast('Не удалось скопировать ссылку',true)}};
  $('#logout').onclick=async()=>{try{await api('/logout',{method:'POST',headers:{'X-CSRF-Token':state.csrf}});location.href='/login'}catch(e){toast(e.message,true)}};
  $('#metrika-report-kind').onchange=clearMetrika;$('#load-metrika-report').onclick=()=>loadMetrikaReport();
  $('#metrika-prev').onclick=()=>{state.metrikaOffset=Math.max(1,state.metrikaOffset-500);loadMetrikaReport(false)};$('#metrika-next').onclick=()=>{state.metrikaOffset+=500;loadMetrikaReport(false)};
  let width=0;new ResizeObserver(([e])=>{const w=Math.round(e.contentRect.width);if(w!==width&&w>0&&state.data){width=w;renderChart()}}).observe($('#report-chart'));
}
async function init(){
  bind();
  const me=await api('/v1/me');state.csrf=me.csrf_token;$('#username').textContent=me.username;
  const p=await api('/v1/projects');for(const item of p.projects||[]){const o=document.createElement('option');o.value=item.id;o.textContent=item.name;$('#project-filter').append(o)}
  setPeriod('30');
  const q=new URLSearchParams(location.search);
  for(const [k,id] of [['project_id','project-filter'],['from_date','date-from'],['to_date','date-to']])if(q.get(k))$('#'+id).value=q.get(k);
  if(REPORTS[q.get('report')])state.report=q.get('report');
  defaultSort();
  if(['','NO_BOTS','HUMAN','SUSPICIOUS','BOT','KNOWN_BOT','UNCLASSIFIED'].includes(q.get('classification')||''))state.segment=q.get('classification')||'';
  if(['hour','day','week','month'].includes(q.get('interval')))state.interval=q.get('interval');$('#interval').value=state.interval;
  state.compare=q.get('compare')==='1';$('#compare').checked=state.compare;
  if(METRICS[q.get('metric')])state.chartMetric=q.get('metric');$('#chart-metric').value=state.chartMetric;
  for(const k of FILTER_PARAMS)if(q.has(k))state.filters.set(k,q.get(k));
  await loadReport();
}
document.addEventListener("clickshield:reload",()=>loadReport());
document.addEventListener("clickshield:timeline",()=>reloadTimeline());
init().catch(e=>{$('#report-status').textContent=e.message;toast(e.message,true)});

export {loadReport, loadTimeline, reloadTimeline, renderAll};
