import {$, esc, SEQUENTIAL, fmtInt, parseDate, ruDate} from "./analytics-common.js";
import {state} from "./analytics-state.js";
import {api} from "./analytics-api.js";
import {baseQuery, switchReport} from "./analytics-filters.js";
import {projectPicker} from "./analytics-project-picker.js";

 'use strict';
 const el=id=>document.getElementById(id);
 let request=0,lastDefinitions=[],definitionProject='',journeyFilter='';
 const pct=(n,d)=>d?(100*n/d).toLocaleString('ru-RU',{maximumFractionDigits:1})+' %':'0 %';
 const query=()=>{const q=baseQuery();q.delete('report');return q};
 const mutate=(path,method,body)=>api(path,{method,headers:{'Content-Type':'application/json','X-CSRF-Token':state.csrf},body:body?JSON.stringify(body):undefined});
 const status=t=>{el('product-status').textContent=t};
 const empty=(title,text)=>`<div class="rp-empty-state"><strong>${esc(title)}</strong><p>${esc(text)}</p></div>`;
 const bar=(value,max,cls='')=>`<span class="rp-bar ${cls}"><i data-w="${max?(100*value/max).toFixed(2):0}"></i></span>`;
 const finish=root=>{for(const i of root.querySelectorAll('[data-w]'))i.style.width=i.dataset.w+'%';for(const i of root.querySelectorAll('[data-bg]')){i.style.background=i.dataset.bg;if(i.dataset.ink)i.style.color=i.dataset.ink}};
 const stepTypes={page:'Страница',event:'Событие',outcome:'Заявка'};
 const outcomes={LEAD:'любая заявка',SUBMITTED:'отправлена',QUALIFIED:'качественная',SPAM:'спам',LOST:'потеряна'};
 function stepText(s){
  const v=s.type==='outcome'?outcomes[s.value]||s.value:s.value;
  const props=s.properties&&Object.keys(s.properties).length?' · '+Object.entries(s.properties).map(([k,x])=>`${k}=${JSON.stringify(x)}`).join(', '):'';
  return `${stepTypes[s.type]||s.type}: ${v}${s.match==='prefix'?' (начинается с)':''}${props}`;
 }
 function pagePath(u){if(u==='[entry]')return 'Вход на сайт';if(u==='[exit]')return 'Последняя страница визита';try{const x=new URL(u);return decodeURI(x.pathname)+x.search}catch{return u}}
 function needProject(){if(el('project-filter').value)return false;el('product-result').innerHTML=empty('Выберите проект','Цели и воронки настраиваются для одного сайта.').replace('</div>',projectPicker()+'</div>');status('');return true}

 async function load(extra={}) {
  const n=++request,kind=el('product-kind').value,q=query();for(const [k,v] of Object.entries(extra))q.set(k,v);
  const root=el('product-result');root.classList.add('loading');status('Загружаем…');
  if((kind==='goals'||kind==='funnels')&&needProject()){root.classList.remove('loading');return}
  try {
   const {data}=await api('/v1/analytics/'+kind+'?'+q);if(n!==request)return;
   status('');root.classList.remove('loading');
   ({events,goals:definitions,funnels:definitions,journey,retention,heatmap})[kind](root,data,kind);
   finish(root);
  } catch(e){if(n===request){root.classList.remove('loading');status('Отчёт не загружен: '+e.message)}}
 }

 function events(root,data){
  if(!data.length){root.innerHTML=empty('Событий пока нет','Отправляйте их из кода сайта: ClickShield.track(\'phone_call\') или атрибутом data-cs-event на кнопке.');return}
  const max=Math.max(...data.map(r=>r.visits));
  root.innerHTML=`<table class="rp-simple-table"><thead><tr><th>Событие</th><th class="num">Срабатывания</th><th class="num">Визиты с событием</th><th class="num">Посетители</th><th class="num">На визит</th></tr></thead><tbody>${data.map((r,i)=>`<tr tabindex="0" data-i="${i}" title="Показать параметры"><td><code>${esc(r.name)}</code></td><td class="num">${fmtInt(r.events)}</td><td class="num">${fmtInt(r.visits)}${bar(r.visits,max)}</td><td class="num">${fmtInt(r.visitors)}</td><td class="num">${(r.visits?r.events/r.visits:0).toLocaleString('ru-RU',{maximumFractionDigits:2})}</td></tr>`).join('')}</tbody></table>`;
  for(const row of root.querySelectorAll('tbody tr')){const open=()=>properties(root,data[Number(row.dataset.i)].name);row.onclick=open;row.onkeydown=e=>{if(e.key==='Enter')open()}}
 }
 async function properties(root,name){
  const current=++request;status('Загружаем параметры…');
  try{
   const p=query();p.set('event',name);const r=await api('/v1/analytics/properties?'+p);if(current!==request)return;
   const max=Math.max(1,...r.data.map(v=>v.events));
   status('');
   root.innerHTML=`<div class="rp-subhead"><button type="button" class="rp-button" id="events-back">← Все события</button><h3>Параметры события <code>${esc(name)}</code></h3><span class="muted">до 500 сочетаний</span></div>${r.data.length?`<table class="rp-simple-table"><thead><tr><th>Параметр</th><th>Значение</th><th class="num">Срабатывания</th></tr></thead><tbody>${r.data.map(v=>`<tr><td>${esc(v.property)}</td><td><code>${esc(JSON.stringify(v.value))}</code></td><td class="num">${fmtInt(v.events)}${bar(v.events,max)}</td></tr>`).join('')}</tbody></table>`:empty('Без параметров','Это событие отправляется без параметров.')}`;
   finish(root);el('events-back').onclick=()=>load();
  }catch(e){if(current===request)status(e.message)}
 }

 function definitions(root,data,kind){
  lastDefinitions=data.map(r=>r.definition);
  if(!data.length){root.innerHTML=kind==='goals'?empty('Целей пока нет','Цель — действие, ради которого существует сайт: заявка, звонок, визит на страницу «Спасибо». Нажмите «Создать цель или воронку».'):empty('Воронок пока нет','Воронка показывает, на каком шаге теряются посетители: например, каталог → карточка товара → заявка.');return}
  root.innerHTML=`<div class="${kind==='goals'?'rp-goal-grid':'rp-funnel-list'}">${data.map(r=>kind==='goals'?goalCard(r):funnelCard(r)).join('')}</div>`;
  for(const b of root.querySelectorAll('[data-edit]'))b.onclick=()=>openDefinition(lastDefinitions.find(d=>d.id===b.dataset.edit));
  for(const b of root.querySelectorAll('[data-delete]'))b.onclick=async()=>{
   if(b.dataset.armed!=='1'){b.dataset.armed='1';b.textContent='Точно удалить?';b.classList.add('rp-danger');setTimeout(()=>{if(b.isConnected){b.dataset.armed='';b.textContent='Удалить';b.classList.remove('rp-danger')}},4000);return}
   try{await mutate(`/v1/projects/${el('project-filter').value}/definitions/${b.dataset.delete}`,'DELETE');await load()}catch(e){status(e.message)}
  };
 }
 const cardActions=d=>`<div class="rp-card-actions"><button type="button" class="rp-button rp-small-button" data-edit="${esc(d.id)}">Изменить</button><button type="button" class="rp-button rp-small-button" data-delete="${esc(d.id)}">Удалить</button></div>`;
 function goalCard(r){
  const d=r.definition,reached=r.counts[0]||0;
  return `<article class="rp-card"><div class="rp-card-head"><h3>${esc(d.name)}</h3>${cardActions(d)}</div><p class="rp-card-sub">${esc(stepText(d.steps[0]))}</p><div class="rp-goal-numbers"><div><strong>${fmtInt(reached)}</strong><span>визитов достигли цели</span></div><div><strong>${pct(reached,r.eligible_visits)}</strong><span>конверсия из ${fmtInt(r.eligible_visits)} визитов</span></div></div>${bar(reached,r.eligible_visits,'thick')}</article>`;
 }
 function funnelCard(r){
  const d=r.definition,first=r.counts[0]||0,last=r.counts.at(-1)||0;
  const steps=d.steps.map((s,i)=>{
   const c=r.counts[i]||0,prev=i?r.counts[i-1]||0:0,lost=prev-c;
   return `${i?`<div class="rp-funnel-drop">${lost>0?`↓ ушли ${fmtInt(lost)} · ${pct(lost,prev)}`:'↓ без потерь'}</div>`:''}<div class="rp-funnel-step"><span class="rp-step-no">${i+1}</span><div class="rp-funnel-main"><div class="rp-funnel-label"><span>${esc(stepText(s))}</span><span><strong>${fmtInt(c)}</strong> · ${pct(c,first)}</span></div><span class="rp-funnel-bar"><i data-w="${first?(100*c/first).toFixed(2):0}"></i></span></div></div>`;
  }).join('');
  return `<article class="rp-card"><div class="rp-card-head"><h3>${esc(d.name)}</h3>${cardActions(d)}</div><p class="rp-card-sub">Окно между шагами — ${fmtInt(d.window_minutes)} мин. Подходящих визитов за период: ${fmtInt(r.eligible_visits)}.</p><div class="rp-funnel">${steps}</div><div class="rp-funnel-total"><span>Прошли всю воронку</span><strong>${fmtInt(last)}</strong><span>${pct(last,first)} от первого шага · ${pct(last,r.eligible_visits)} от всех визитов</span></div></article>`;
 }

 function journey(root,data){
  if(!data.length){root.innerHTML=empty('Переходов нет','Пути появятся, когда посетители начнут просматривать несколько страниц за визит.');return}
  const draw=()=>{
   const f=journeyFilter.toLowerCase(),rows=data.filter(r=>!f||pagePath(r.from).toLowerCase().includes(f)||pagePath(r.to).toLowerCase().includes(f)),max=Math.max(1,...rows.map(r=>r.transitions));
   el('journey-table').innerHTML=rows.length?`<table class="rp-simple-table"><thead><tr><th>Откуда</th><th></th><th>Куда</th><th class="num">Переходы</th><th class="num">Визиты</th></tr></thead><tbody>${rows.map(r=>`<tr><td class="${r.from==='[entry]'?'rp-pseudo':''}" title="${esc(r.from)}">${esc(pagePath(r.from))}</td><td class="rp-arrow">→</td><td class="${r.to==='[exit]'?'rp-pseudo':''}" title="${esc(r.to)}">${esc(pagePath(r.to))}</td><td class="num">${fmtInt(r.transitions)}${bar(r.transitions,max)}</td><td class="num">${fmtInt(r.visits)}</td></tr>`).join('')}</tbody></table>`:empty('Ничего не найдено','Измените текст поиска.');
   finish(el('journey-table'));
  };
  root.innerHTML=`<div class="rp-subhead"><input id="journey-filter" type="search" placeholder="Страница содержит…" aria-label="Фильтр по странице" value="${esc(journeyFilter)}"><span class="muted">До 200 самых частых переходов. «Последняя страница визита» не всегда означает уход: визит может продолжаться.</span></div><div id="journey-table"></div>`;
  el('journey-filter').oninput=e=>{journeyFilter=e.target.value.trim();draw()};draw();
 }

 function retention(root,data){
  const groups=new Map();for(const c of data){if(!groups.has(c.cohort))groups.set(c.cohort,[]);groups.get(c.cohort).push(c)}
  if(!groups.size){root.innerHTML=empty('Когорт нет','За период нет новых посетителей.');return}
  const days=Math.max(...[...groups.values()].map(c=>c.length));
  const total=[...Array(days)].map((_,i)=>{let v=0,r=0;for(const cells of groups.values()){const c=cells[i];if(c?.observed){v+=c.visitors;r+=c.returned}}return v?r/v:null});
  const color=rate=>{if(rate==null)return ['',''];if(!rate)return ['#f4f5fa',''];const i=Math.min(SEQUENTIAL.length-1,Math.round(Math.sqrt(rate)*(SEQUENTIAL.length-1)));return [SEQUENTIAL[i],i>=7?'#fff':'']};
  const cell=(rate,title)=>{const [bg,ink]=color(rate);return `<td class="rp-ret" ${bg?`data-bg="${bg}"`:''} ${ink?`data-ink="${ink}"`:''} title="${esc(title)}">${rate==null?'—':Math.round(rate*100)+'%'}</td>`};
  status('Когорта — посетители, впервые пришедшие в этот день. Ячейка — доля из них, вернувшихся через N дней. Будущие дни показаны прочерком.');
  root.innerHTML=`<div class="rp-ret-wrap"><table class="rp-ret-table"><thead><tr><th class="rp-ret-date">Первый визит</th><th class="num">Новых</th>${[...Array(days)].map((_,i)=>`<th>Д${i}</th>`).join('')}</tr></thead><tbody><tr class="rp-ret-avg"><td class="rp-ret-date">Все когорты</td><td class="num">${fmtInt([...groups.values()].reduce((a,c)=>a+c[0].visitors,0))}</td>${total.map((r,i)=>cell(r,`День ${i}: в среднем ${r==null?'—':pct(r,1)}`)).join('')}</tr>${[...groups].map(([date,cells])=>`<tr><td class="rp-ret-date">${esc(ruDate(parseDate(date),{day:'numeric',month:'short',weekday:'short'}))}</td><td class="num">${fmtInt(cells[0].visitors)}</td>${cells.map(c=>cell(c.observed?c.returned/c.visitors:null,`${c.returned} из ${c.visitors}`)).join('')}</tr>`).join('')}</tbody></table></div>`;
 }

 function heatmap(root,data){
  if(!data.pages.length){root.innerHTML=empty('Карт пока нет','Они появятся после новых кликов и прокруток с обновлённым tracker.js.');return}
  const pages=[...new Set(data.pages.map(p=>p.url))],widths=data.pages.filter(p=>p.url===data.page_url);
  const maxScroll=Math.max(1,data.visits);
  root.innerHTML=`<div class="rp-subhead"><label class="rp-inline">Страница<select id="heatmap-page">${pages.map(p=>`<option ${p===data.page_url?'selected':''} value="${esc(p)}">${esc(pagePath(p))}</option>`).join('')}</select></label><label class="rp-inline">Ширина окна<select id="heatmap-width">${widths.map(p=>`<option value="${p.width}" ${p.width===data.width?'selected':''}>${p.width}px · ${fmtInt(p.visits)} визитов</option>`).join('')}</select></label><span class="muted">${fmtInt(data.visits)} визитов</span></div>
   <div class="rp-heat-layout"><div class="rp-card"><div class="rp-card-head"><h3>Клики</h3><span class="muted">координаты нормализованы по ширине окна и высоте страницы</span></div><div class="rp-heat-frame"><canvas id="heatmap-canvas" width="500" height="800" aria-label="Агрегированная карта кликов"></canvas></div><div class="rp-heatmap-scale"><span>реже</span><i class="rp-heat-gradient"></i><span>чаще</span></div></div>
   <div class="rp-card"><div class="rp-card-head"><h3>Глубина прокрутки</h3><span class="muted">доля визитов, дочитавших до уровня</span></div><div class="rp-scroll">${data.scroll.map(r=>`<div class="rp-scroll-row"><span>${r.depth}%</span><span class="rp-funnel-bar"><i data-w="${(100*r.visits/maxScroll).toFixed(2)}"></i></span><strong>${pct(r.visits,data.visits)}</strong></div>`).join('')}</div></div></div>`;
  const c=el('heatmap-canvas'),ctx=c.getContext('2d');
  ctx.fillStyle='#fafbfe';ctx.fillRect(0,0,c.width,c.height);ctx.strokeStyle='#eceef4';
  for(let i=1;i<10;i++){ctx.beginPath();ctx.moveTo(i*c.width/10,0);ctx.lineTo(i*c.width/10,c.height);ctx.moveTo(0,i*c.height/10);ctx.lineTo(c.width,i*c.height/10);ctx.stroke()}
  const max=Math.max(1,...data.clicks.map(p=>p.count));
  ctx.globalCompositeOperation='multiply';
  for(const p of data.clicks){const x=(p.x+.5)*5,y=(p.y+.5)*8,a=.15+.85*p.count/max,g=ctx.createRadialGradient(x,y,0,x,y,22);g.addColorStop(0,`rgba(208,59,59,${a})`);g.addColorStop(.5,`rgba(250,178,25,${a*.55})`);g.addColorStop(1,'rgba(250,178,25,0)');ctx.fillStyle=g;ctx.beginPath();ctx.arc(x,y,22,0,2*Math.PI);ctx.fill()}
  el('heatmap-page').onchange=()=>load({page_url:el('heatmap-page').value});
  el('heatmap-width').onchange=()=>load({page_url:data.page_url,width:el('heatmap-width').value});
 }

 function addStep(step={type:'page',value:'',match:'exact',properties:{}}) {
  const row=document.createElement('div');row.className='rp-step';
  row.innerHTML=`<span class="rp-step-no"></span><label>Действие<select class="step-type"><option value="page">Страница</option><option value="event">Событие</option><option value="outcome">Заявка</option></select></label><label>Значение<input class="step-value" maxlength="2048" required></label><label>Совпадение<select class="step-match"><option value="exact">Точное</option><option value="prefix">Начинается с</option></select></label><label>Параметры события, JSON<input class="step-properties" value="{}"></label><button type="button" class="rp-icon-button" aria-label="Убрать шаг">×</button>`;
  row.querySelector('.step-type').value=step.type;row.querySelector('.step-value').value=step.value;row.querySelector('.step-match').value=step.match||'exact';row.querySelector('.step-properties').value=JSON.stringify(step.properties||{});
  const sync=()=>{const t=row.querySelector('.step-type').value,v=row.querySelector('.step-value');v.placeholder=t==='page'?'/contacts':t==='event'?'phone_call':'LEAD или QUALIFIED';row.querySelector('.step-properties').closest('label').classList.toggle('hidden',t!=='event')};
  row.querySelector('.step-type').onchange=sync;sync();
  row.querySelector('button').onclick=()=>{row.remove();number()};el('definition-steps').append(row);number();
 }
 const number=()=>[...el('definition-steps').children].forEach((r,i)=>r.querySelector('.rp-step-no').textContent=i+1);
 function openDefinition(d) {
  definitionProject=el('project-filter').value;if(!definitionProject){needProject();return}
  el('definition-id').value=d?.id||'';el('definition-name').value=d?.name||'';el('definition-kind').value=d?.kind||(el('product-kind').value==='funnels'?'funnel':'goal');el('definition-window').value=d?.window_minutes||60;el('definition-steps').replaceChildren();
  for(const s of d?.steps||(el('definition-kind').value==='funnel'?[{type:'page',value:'',match:'exact'},{type:'outcome',value:'LEAD',match:'exact'}]:[{type:'page',value:'',match:'exact'}]))addStep(s);
  el('definition-error').textContent='';el('definition-dialog').showModal();
 }
 el('definition-form').onsubmit=async e=>{e.preventDefault();try{const id=el('definition-id').value,steps=[...el('definition-steps').children].map(row=>({type:row.querySelector('.step-type').value,value:row.querySelector('.step-value').value,match:row.querySelector('.step-match').value,properties:JSON.parse(row.querySelector('.step-properties').value||'{}')}));const kind=el('definition-kind').value;await mutate(`/v1/projects/${definitionProject}/definitions${id?'/'+id:''}`,id?'PUT':'POST',{name:el('definition-name').value,kind,steps,window_minutes:Number(el('definition-window').value)});el('definition-dialog').close();const target=kind==='goal'?'goals':'funnels';if(state.report!==target)switchReport(target);else await load()}catch(e){el('definition-error').textContent=e instanceof SyntaxError?'Параметры события должны быть JSON-объектом, например {"plan":"pro"}':e.message}};
 el('new-definition').onclick=()=>openDefinition();el('close-definition').onclick=()=>el('definition-dialog').close();el('add-definition-step').onclick=()=>{if(el('definition-steps').children.length<12)addStep()};el('load-product').onclick=()=>load();el('product-kind').onchange=()=>load();
 document.addEventListener('clickshield:filters',()=>{++request;el('product-result').replaceChildren();status('')});
