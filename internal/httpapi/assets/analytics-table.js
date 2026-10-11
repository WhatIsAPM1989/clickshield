import {$, esc, PAGE, storage, fmtInt, fmtPct, plural, MAX_SERIES} from "./analytics-common.js";
import {METRICS, COLUMN_GROUPS, DIMS, dimLabel} from "./analytics-model.js";
import {state,colorFor,isTimeReport} from "./analytics-state.js";
import {toast} from "./analytics-api.js";
import {drillURL, canFilter, rowKey, addFilters} from "./analytics-filters.js";
import {delta} from "./analytics-kpis.js";


// ---------- Table ----------
function columns(){return COLUMN_GROUPS.filter(g=>state.groups.has(g.id)).map(g=>({group:g,metrics:g.metrics}))}
function renderColumnGroups(){
  $('#column-groups').innerHTML=COLUMN_GROUPS.map(g=>`<button type="button" data-group="${g.id}" class="${state.groups.has(g.id)?'active':''}" aria-pressed="${state.groups.has(g.id)}">${esc(g.label)}</button>`).join('');
  for(const b of document.querySelectorAll('[data-group]'))b.onclick=()=>{const id=b.dataset.group;if(state.groups.has(id)){if(state.groups.size>1)state.groups.delete(id)}else state.groups.add(id);storage.set('column-groups',[...state.groups]);renderColumnGroups();renderTable()};
  $('#toggle-expand').classList.toggle('hidden',(state.data?.dimensions.length||1)<2);
  $('#toggle-expand').textContent=state.expandAll?'Свернуть всё':'Развернуть всё';
}
function buildTree(){
  const d=state.data,nodes=new Map(),roots=[];
  for(const r of d.rows){const n={row:r,key:rowKey(r.dimensions),children:[]};nodes.set(n.key,n);if(r.level===1)roots.push(n);else nodes.get(rowKey(r.dimensions.slice(0,-1)))?.children.push(n)}
  const k=state.sort.key,dir=state.sort.dir,dim=d.dimensions;
  const cmp=(a,b)=>{if(k==='dimension'){const da=a.row.dimensions.at(-1),db=b.row.dimensions.at(-1),key=dim[a.row.level-1];const num=['day','hour','weekday'].includes(key);return dir*(num?(da<db?-1:da>db?1:0):dimLabel(key,da).localeCompare(dimLabel(key,db),'ru'))}const m=k==='quality'?METRICS.bot_share:METRICS[k];return dir*(m.value(a.row)-m.value(b.row))||b.row.visits-a.row.visits};
  const sortAll=list=>{list.sort(cmp);for(const n of list)sortAll(n.children)};sortAll(roots);
  return roots;
}
function matches(n,q){return dimLabel(state.data.dimensions[n.row.level-1],n.row.dimensions.at(-1)).toLowerCase().includes(q)||n.row.dimensions.at(-1).toLowerCase().includes(q)||n.children.some(c=>matches(c,q))}
function labelHTML(key,v){
  const text=dimLabel(key,v),q=state.search;
  if(DIMS[key]?.url&&v){try{const u=new URL(v);return `<span class="host">${esc(u.host)}</span>${hl(decodeURI(u.pathname)+u.search)}`}catch{}}
  return hl(text);
  function hl(s){if(!q)return esc(s);const i=s.toLowerCase().indexOf(q);return i<0?esc(s):esc(s.slice(0,i))+'<mark>'+esc(s.slice(i,i+q.length))+'</mark>'+esc(s.slice(i+q.length))}
}
function cellHTML(metric,row,total){
  if(metric==='quality'){const v=row.visits||1,p=x=>(x/v*100).toFixed(2);return `<td class="rp-cell"><span class="rp-quality" title="${esc(`Люди ${fmtPct(row.humans/v)} · Подозрительные ${fmtPct(row.suspicious/v)} · Боты ${fmtPct(row.bots/v)} · Краулеры ${fmtPct(row.known_bots/v)}`)}">${[['human',row.humans],['suspicious',row.suspicious],['bot',row.bots],['known',row.known_bots]].map(([c,x])=>x?`<i class="${c}" data-w="${p(x)}"></i>`:'').join('')}</span></td>`}
  const m=METRICS[metric],prev=state.compare&&!total?state.prevIndex.get(rowKey(row.dimensions)):state.compare&&total?state.prev?.totals:undefined;
  let extra='';
  if(state.compare){const dl=delta(metric,row,prev||(m.pct||metric==='depth'||metric==='duration'?null:emptyMetrics()));extra=dl?`<small class="${/good/.test(dl)?'good':/bad/.test(dl)?'bad':'flat'}">${dl.replace(/<[^>]+>/g,'')}</small>`:''}
  if(metric==='visits'&&!total&&!extra)extra=`<span class="rp-share"><i data-w="${(row.visits/(state.data.totals.visits||1)*100).toFixed(2)}"></i></span>`;
  return `<td class="rp-cell">${esc(m.fmt(m.value(row)))}${extra}</td>`;
}
const emptyMetrics=()=>({visits:0,visitors:0,new_visitors:0,pageviews:0,bounces:0,humans:0,suspicious:0,bots:0,known_bots:0,leads:0,qualified:0,converted_visits:0,qualified_visits:0,replays:0,average_duration_ms:0});
function renderTable(){
  const d=state.data,el=$('#report-table');if(!d)return;
  if(!d.rows.length){el.innerHTML='<p class="empty">Визитов нет. Выберите другой период, проект или уберите фильтры.</p>';$('#report-page-label').textContent='';$('#report-prev').disabled=$('#report-next').disabled=true;return}
  const cols=columns(),flat=cols.flatMap(c=>c.metrics),q=state.search,chartable=!isTimeReport()&&!['hours','activity'].includes(state.report);
  let roots=buildTree();if(q)roots=roots.filter(n=>matches(n,q));
  const pages=Math.max(1,Math.ceil(roots.length/PAGE));state.page=Math.min(state.page,pages-1);
  const pageRoots=roots.slice(state.page*PAGE,state.page*PAGE+PAGE),lines=[];
  const walk=n=>{lines.push(n);if(n.children.length&&(state.expandAll||q||state.expanded.has(n.key)))for(const c of n.children)if(!q||matches(c,q))walk(c)};pageRoots.forEach(walk);
  const sortMark=k=>state.sort.key===k?(state.sort.dir<0?' ↓':' ↑'):'';
  const dimTitle=d.dimensions.map(k=>DIMS[k]?.label||k).join(' → ');
  let html=`<table class="rp-table"><thead><tr class="group-row"><th class="dim"></th>${cols.map(c=>`<th colspan="${c.metrics.length}">${esc(c.group.label)}</th>`).join('')}</tr><tr><th class="dim ${state.sort.key==='dimension'?'sorted':''}" data-sort="dimension" scope="col">${esc(dimTitle)}${sortMark('dimension')}</th>${flat.map(k=>`<th scope="col" data-sort="${k}" class="${state.sort.key===k?'sorted':''}" title="${esc(k==='quality'?'Люди / подозрительные / боты / краулеры. Сортировка по доле ботов.':METRICS[k].label)}">${esc(k==='quality'?'Структура':METRICS[k].short||METRICS[k].label)}${sortMark(k)}</th>`).join('')}</tr></thead><tbody>`;
  html+=`<tr class="total"><td class="dim"><div class="rp-dim"><span class="rp-label">Итого и средние</span></div></td>${flat.map(k=>cellHTML(k,{...d.totals,dimensions:[]},true)).join('')}</tr>`;
  for(const n of lines){
    const r=n.row,key=d.dimensions[r.level-1],v=r.dimensions.at(-1),hasKids=n.children.length>0,open=state.expandAll||!!q||state.expanded.has(n.key),sel=r.level===1&&state.selected.includes(v);
    const color=sel&&chartable?colorFor(v):'';
    html+=`<tr class="level-${r.level}" tabindex="0" data-key="${esc(n.key)}"><td class="dim"><div class="rp-dim"><span class="indent" data-indent="${(r.level-1)*22}"></span>${r.level===1&&chartable?`<input type="checkbox" data-select="${esc(v)}" ${sel?'checked':''} aria-label="Показать на графике">`:''}<button type="button" class="rp-caret ${hasKids?'':'none'} ${open?'open':''}" data-toggle="${esc(n.key)}" aria-label="${open?'Свернуть':'Развернуть'}" aria-expanded="${open}" tabindex="${hasKids?0:-1}">›</button>${color?`<i class="rp-swatch" data-c="${color}"></i>`:''}<span class="rp-label" title="${esc(dimLabel(key,v)+(DIMS[key]?.url?'\n'+v:''))}">${labelHTML(key,v)}</span><span class="rp-row-actions"><a href="${esc(drillURL(r.dimensions))}" data-drill>Сессии</a>${canFilter(r.dimensions)?`<button type="button" data-filter="${esc(n.key)}">Фильтр</button>`:''}</span></div></td>${flat.map(k=>cellHTML(k,r)).join('')}</tr>`;
  }
  el.innerHTML=html+'</tbody></table>'+(state.groups.has('quality')?'<div class="rp-quality-legend"><span><i class="human"></i>Люди</span><span><i class="suspicious"></i>Подозрительные</span><span><i class="bot"></i>Боты</span><span><i class="known"></i>Известные краулеры</span><span><i class="pending"></i>Ожидают расчёта</span></div>':'');
  for(const i of el.querySelectorAll('[data-w]'))i.style.width=i.dataset.w+'%';
  for(const i of el.querySelectorAll('[data-indent]'))i.style.width=i.dataset.indent+'px';
  for(const i of el.querySelectorAll('[data-c]'))i.style.background=i.dataset.c;
  const nodeByKey=new Map(lines.map(n=>[n.key,n]));
  for(const th of el.querySelectorAll('[data-sort]'))th.onclick=()=>{const k=th.dataset.sort;state.sort=state.sort.key===k?{key:k,dir:-state.sort.dir}:{key:k,dir:k==='dimension'?1:-1};renderTable()};
  for(const tr of el.querySelectorAll('tbody tr[data-key]')){
    const n=nodeByKey.get(tr.dataset.key);
    tr.onclick=e=>{if(e.target.closest('a,button,input'))return;location.href=drillURL(n.row.dimensions)};
    tr.onkeydown=e=>{if(e.target!==tr)return;if(e.key==='Enter'){e.preventDefault();location.href=drillURL(n.row.dimensions)}if(e.key==='ArrowRight'&&n.children.length){state.expanded.add(n.key);renderTable()}if(e.key==='ArrowLeft'){state.expanded.delete(n.key);renderTable()}};
  }
  for(const b of el.querySelectorAll('[data-toggle]'))b.onclick=()=>{const k=b.dataset.toggle;if(state.expandAll){state.expandAll=false;for(const n of lines)if(n.children.length)state.expanded.add(n.key)}state.expanded.has(k)?state.expanded.delete(k):state.expanded.add(k);renderColumnGroups();renderTable()};
  for(const b of el.querySelectorAll('[data-filter]'))b.onclick=()=>addFilters(nodeByKey.get(b.dataset.filter).row.dimensions);
  for(const c of el.querySelectorAll('[data-select]'))c.onchange=()=>{
    const v=c.dataset.select;
    if(c.checked){if(state.selected.length>=MAX_SERIES){c.checked=false;toast(`На графике не больше ${MAX_SERIES} строк`,true);return}state.selected.push(v)}else state.selected=state.selected.filter(x=>x!==v);
    state.hidden.delete(v);document.dispatchEvent(new Event("clickshield:timeline"));
  };
  $('#report-page-label').textContent=roots.length>PAGE?`${state.page*PAGE+1}–${Math.min((state.page+1)*PAGE,roots.length)} из ${fmtInt(roots.length)}`:`${fmtInt(roots.length)} ${plural(roots.length,'строка','строки','строк')}${q?' найдено':''}`;
  $('#report-prev').disabled=state.page===0;$('#report-next').disabled=state.page>=pages-1;
}


export {columns, renderColumnGroups, buildTree, matches, labelHTML, cellHTML, emptyMetrics, renderTable};
