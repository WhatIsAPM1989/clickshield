import {$, esc, zone, isoDate, parseDate} from "./analytics-common.js";
import {DIMS, REPORTS, dimLabel} from "./analytics-model.js";
import {state,isTimeReport} from "./analytics-state.js";


// ---------- Period ----------
function periodDays(){return Math.round((parseDate($('#date-to').value)-parseDate($('#date-from').value))/864e5)+1}
function autoInterval(){const days=periodDays();return days<=3?'hour':days<=92?'day':days<=400?'week':'month'}
function effectiveInterval(){return state.interval==='auto'||state.interval==='hour'&&periodDays()>31?autoInterval():state.interval}
const intervalNames={hour:'по часам',day:'по дням',week:'по неделям',month:'по месяцам'};
function range(fromDate,toDate){const end=parseDate(toDate);end.setDate(end.getDate()+1);return {from:parseDate(fromDate).toISOString(),to:end.toISOString()}}
function previousDates(){
  const from=parseDate($('#date-from').value),days=periodDays(),pFrom=new Date(from),pTo=new Date(from);
  pFrom.setDate(pFrom.getDate()-days);pTo.setDate(pTo.getDate()-1);return [isoDate(pFrom),isoDate(pTo)];
}
function setPeriod(kind){
  const today=new Date(),start=new Date(),end=new Date();
  if(kind==='yesterday'){start.setDate(today.getDate()-1);end.setDate(today.getDate()-1)}
  else if(kind!=='today')start.setDate(today.getDate()-Number(kind)+1);
  $('#date-from').value=isoDate(start);$('#date-to').value=isoDate(end);
}
function markPeriod(){
  const from=$('#date-from').value,to=$('#date-to').value,today=new Date();
  for(const b of document.querySelectorAll('[data-period]')){
    const k=b.dataset.period,s=new Date(),e=new Date();
    if(k==='yesterday'){s.setDate(today.getDate()-1);e.setDate(today.getDate()-1)}else if(k!=='today')s.setDate(today.getDate()-Number(k)+1);
    b.classList.toggle('active',isoDate(s)===from&&isoDate(e)===to);
  }
  for(const o of $('#interval').options){if(o.value==='hour')o.disabled=periodDays()>31;if(o.value==='auto')o.textContent=`авто (${intervalNames[autoInterval()]})`}
}

// ---------- Queries ----------
function baseQuery(fromDate=$('#date-from').value,toDate=$('#date-to').value){
  const q=new URLSearchParams({report:state.report,timezone:zone,...range(fromDate,toDate)});
  if($('#project-filter').value)q.set('project_id',$('#project-filter').value);
  if(state.segment)q.set('classification',state.segment);
  for(const [k,v] of state.filters)q.set(k,v);
  return q;
}
function syncURL(){
  const q=new URLSearchParams({report:state.report,from_date:$('#date-from').value,to_date:$('#date-to').value});
  if($('#project-filter').value)q.set('project_id',$('#project-filter').value);
  if(state.segment)q.set('classification',state.segment);
  if(state.interval!=='auto')q.set('interval',state.interval);
  if(state.compare)q.set('compare','1');
  if(state.chartMetric!=='visits')q.set('metric',state.chartMetric);
  for(const [k,v] of state.filters)q.set(k,v);
  history.replaceState(null,'','/analytics?'+q);
}
function drillURL(dims,classification=state.segment){
  const q=new URLSearchParams({from_date:$('#date-from').value,to_date:$('#date-to').value});
  if($('#project-filter').value)q.set('project_id',$('#project-filter').value);
  if(classification)q.set('classification',classification);
  for(const [k,v] of state.filters)q.set(k,v);
  dims.forEach((v,i)=>{const k=state.data.dimensions[i],d=DIMS[k];if(k==='day'){q.set('from_date',v);q.set('to_date',v)}else if(d?.param&&(v||d.empty))q.set(d.param,v)});
  return '/?'+q;
}
const canFilter=dims=>dims.some((v,i)=>{const d=DIMS[state.data.dimensions[i]];return d&&(d.param&&(v||d.empty)||state.data.dimensions[i]==='day')});
const rowKey=dims=>dims.join('\u0001');

// ---------- Catalog and filters ----------
function renderCatalog(){
  const groups={};for(const [k,r] of Object.entries(REPORTS))(groups[r.group]||=[]).push([k,r]);
  $('#report-catalog').innerHTML=Object.entries(groups).map(([g,items])=>`<h3>${esc(g)}</h3>${items.map(([k,r])=>`<button type="button" data-report="${k}" class="${k===state.report?'active':''}" ${k===state.report?'aria-current="page"':''}>${esc(r.short)}</button>`).join('')}`).join('');
  $('#report-select').innerHTML=Object.entries(groups).map(([g,items])=>`<optgroup label="${esc(g)}">${items.map(([k,r])=>`<option value="${k}" ${k===state.report?'selected':''}>${esc(r.short)}</option>`).join('')}</optgroup>`).join('');
  for(const b of document.querySelectorAll('[data-report]'))b.onclick=()=>switchReport(b.dataset.report);
}
function switchReport(kind){if(!REPORTS[kind])return;state.report=kind;state.expanded.clear();state.expandAll=false;state.search='';$('#table-search').value='';defaultSort();document.dispatchEvent(new Event("clickshield:reload"))}
function renderChips(){
  for(const b of document.querySelectorAll('[data-class]'))b.classList.toggle('active',b.dataset.class===state.segment);
  const keyDim=p=>Object.entries(DIMS).find(([,d])=>d.param===p);
  $('#filter-chips').innerHTML=[...state.filters].map(([p,v])=>{const [k,d]=keyDim(p)||[p,{label:p}];return `<span class="rp-chip" title="${esc(d.label+': '+dimLabel(k,v))}"><span>${esc(d.label)}: <b>${esc(dimLabel(k,v))}</b></span><button type="button" data-unfilter="${esc(p)}" aria-label="Убрать фильтр ${esc(d.label)}">×</button></span>`}).join('')+(state.filters.size>1?'<button type="button" class="rp-button rp-small-button" data-unfilter="*">Сбросить фильтры</button>':'');
  for(const b of document.querySelectorAll('[data-unfilter]'))b.onclick=()=>{if(b.dataset.unfilter==='*')state.filters.clear();else state.filters.delete(b.dataset.unfilter);document.dispatchEvent(new Event("clickshield:reload"))};
}
function addFilters(dims){
  dims.forEach((v,i)=>{const k=state.data.dimensions[i],d=DIMS[k];if(k==='day'){$('#date-from').value=v;$('#date-to').value=v}else if(d?.param==='classification')state.segment=v==='UNCLASSIFIED'?'':v;else if(d?.param&&(v||d.empty))state.filters.set(d.param,v)});
  document.dispatchEvent(new Event("clickshield:reload"));
}


function defaultSort(){state.sort=isTimeReport()?{key:"dimension",dir:1}:{key:"visits",dir:-1}}

export {defaultSort,periodDays, autoInterval, effectiveInterval, intervalNames, range, previousDates, setPeriod, markPeriod, baseQuery, syncURL, drillURL, canFilter, rowKey, renderCatalog, switchReport, renderChips, addFilters};
