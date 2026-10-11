'use strict';
const $=q=>document.querySelector(q);
const esc=v=>String(v??'').replace(/[&<>'"]/g,c=>({'&':'&amp;','<':'&lt;','>':'&gt;',"'":'&#39;','"':'&quot;'}[c]));
const zone=Intl.DateTimeFormat().resolvedOptions().timeZone||'Europe/Moscow';
const PAGE=50,MAX_SERIES=8;
const PALETTE=['#2a78d6','#eb6834','#1baf7a','#eda100','#e87ba4','#008300','#6250d6','#e34948'];
const SEQUENTIAL=['#cde2fb','#b7d3f6','#9ec5f4','#86b6ef','#6da7ec','#5598e7','#3987e5','#2a78d6','#256abf','#1c5cab','#184f95','#104281','#0d366b'];
const storage={get(k,d){try{const v=localStorage.getItem('cs-reports-'+k);return v===null?d:JSON.parse(v)}catch{return d}},set(k,v){try{localStorage.setItem('cs-reports-'+k,JSON.stringify(v))}catch{}}};

// ---------- Formatting ----------
const fmtInt=v=>Math.round(Number(v)||0).toLocaleString('ru-RU');
const fmtDec=v=>(Number(v)||0).toLocaleString('ru-RU',{minimumFractionDigits:2,maximumFractionDigits:2});
const fmtPct=v=>((Number(v)||0)*100).toLocaleString('ru-RU',{maximumFractionDigits:1})+' %';
function fmtDur(ms){const s=Math.round((Number(ms)||0)/1000),h=Math.floor(s/3600),m=Math.floor(s%3600/60),r=String(s%60).padStart(2,'0');return h?`${h}:${String(m).padStart(2,'0')}:${r}`:`${m}:${r}`}
const ratio=(a,b)=>b?a/b:0;
const pad=n=>String(n).padStart(2,'0');
const isoDate=d=>`${d.getFullYear()}-${pad(d.getMonth()+1)}-${pad(d.getDate())}`;
const parseDate=s=>{const [y,m,d]=s.split('-').map(Number);return new Date(y,m-1,d)};
const parseBucket=s=>{const [d,t]=s.split(' ');const [y,m,day]=d.split('-').map(Number);return new Date(y,m-1,day,Number((t||'0').split(':')[0]))};
const ruDate=(d,o)=>d.toLocaleDateString('ru-RU',o);

// ---------- Metrics ----------
// better: which direction is an improvement; additive metrics can be stacked and summed.
const METRICS={
  visits:{label:'Визиты',value:m=>m.visits,fmt:fmtInt,better:'up',additive:true},
  visitors:{label:'Посетители',value:m=>m.visitors,fmt:fmtInt,better:'up'},
  new_share:{label:'Новые посетители',short:'Новые',kpi:'Доля новых',value:m=>ratio(m.new_visitors,m.visitors),fmt:fmtPct,pct:true},
  pageviews:{label:'Просмотры',value:m=>m.pageviews,fmt:fmtInt,better:'up',additive:true},
  depth:{label:'Глубина просмотра',short:'Глубина',kpi:'Глубина',value:m=>ratio(m.pageviews,m.visits),fmt:fmtDec,better:'up'},
  duration:{label:'Время на сайте',hint:'Среднее активное время визита: вкладка на экране и посетитель действует на странице',value:m=>m.average_duration_ms,fmt:fmtDur,better:'up',axis:v=>fmtDur(v)},
  bounce:{label:'Отказы',value:m=>ratio(m.bounces,m.visits),fmt:fmtPct,better:'down',pct:true},
  bot_share:{label:'Доля ботов',value:m=>ratio(m.bots+m.known_bots,m.visits),fmt:fmtPct,better:'down',pct:true},
  suspicious_share:{label:'Подозрительные',value:m=>ratio(m.suspicious,m.visits),fmt:fmtPct,better:'down',pct:true},
  leads:{label:'Заявки',value:m=>m.leads,fmt:fmtInt,better:'up',additive:true},
  qualified:{label:'Квалифицированные заявки',short:'Квалиф.',value:m=>m.qualified,fmt:fmtInt,better:'up',additive:true},
  conversion:{label:'Конверсия в заявку',short:'Конверсия',kpi:'Конверсия',value:m=>ratio(m.converted_visits,m.visits),fmt:fmtPct,better:'up',pct:true},
  replays:{label:'Записи посещений',short:'Записи',value:m=>m.replays,fmt:fmtInt,additive:true},
};
const COLUMN_GROUPS=[
  {id:'traffic',label:'Посещаемость',metrics:['visits','visitors','new_share']},
  {id:'behavior',label:'Поведение',metrics:['pageviews','depth','duration','bounce']},
  {id:'quality',label:'Качество',metrics:['quality','bot_share','suspicious_share']},
  {id:'goals',label:'Заявки',metrics:['leads','qualified','conversion']},
  {id:'replays',label:'Записи',metrics:['replays']},
];
const KPIS=['visits','visitors','new_share','bounce','depth','duration','bot_share','conversion'];

// ---------- Dimensions ----------
const channels={paid:'Реклама',paid_social:'Реклама в соцсетях',display:'Медийная реклама',organic_search:'Поиск',social:'Соцсети',referral:'Переходы с сайтов',email:'Email-рассылки',ai:'AI-ассистенты',tagged:'Размеченные переходы',direct_unknown:'Прямые заходы'};
const sources={direct:'Прямой / неизвестный',unknown:'Неизвестный',yandex:'Яндекс',google:'Google',vk:'VK',telegram:'Telegram'};
const classes={HUMAN:'Люди',SUSPICIOUS:'Подозрительные',BOT:'Боты',KNOWN_BOT:'Известные краулеры',UNCLASSIFIED:'Ожидают расчёта'};
const botCategories={search:'Поисковые системы',ai:'AI-краулеры',social:'Соцсети и превью ссылок',seo:'SEO-сервисы',ads:'Реклама',monitoring:'Мониторинг и аудит',feeds:'RSS и архивы',security:'Сканеры',other:'Прочие краулеры',automation:'Автоматизация'};
const weekdays=['','Понедельник','Вторник','Среда','Четверг','Пятница','Суббота','Воскресенье'];
let regionNames=null;try{regionNames=new Intl.DisplayNames(['ru'],{type:'region'})}catch{}
const flag=c=>/^[A-Z]{2}$/.test(c)?String.fromCodePoint(...[...c].map(x=>0x1F1A5+x.charCodeAt(0))):'';
const notSet='(не задано)';
// param: query parameter that filters sessions by this value; empty: whether an empty value can be filtered.
const DIMS={
  channel:{label:'Канал',param:'channel',fmt:v=>channels[v]||v||'Не определён'},
  source:{label:'Источник',param:'source',fmt:v=>sources[v]||v||'Не определён'},
  referrer:{label:'Сайт-источник',param:'referrer',empty:true,fmt:v=>v||'Без перехода с сайта'},
  campaign:{label:'Кампания',param:'campaign',empty:true,fmt:v=>v||'Без кампании'},
  utm_source:{label:'UTM Source',param:'utm_source',empty:true,fmt:v=>v||notSet},
  utm_medium:{label:'UTM Medium',param:'utm_medium',empty:true,fmt:v=>v||notSet},
  utm_campaign:{label:'UTM Campaign',param:'utm_campaign',empty:true,fmt:v=>v||notSet},
  utm_content:{label:'UTM Content',param:'utm_content',empty:true,fmt:v=>v||notSet},
  utm_term:{label:'Ключевая фраза',param:'utm_term',empty:true,fmt:v=>v||'(фраза не передана)'},
  landing_url:{label:'Страница входа',param:'landing_url',url:true,fmt:v=>v||'Не определена'},
  exit_page:{label:'Страница выхода',param:'exit_page',url:true,empty:true,fmt:v=>v||'Не определена'},
  device:{label:'Тип устройства',param:'device',empty:true,fmt:v=>({desktop:'Компьютеры',mobile:'Смартфоны',tablet:'Планшеты',unknown:'Не определено'})[v]||v},
  os:{label:'Операционная система',param:'os',empty:true,fmt:v=>v==='unknown'||!v?'Не определена':v},
  browser:{label:'Браузер',param:'browser',empty:true,fmt:v=>v==='unknown'||!v?'Не определён':v==='Yandex'?'Яндекс Браузер':v},
  region:{label:'Регион',param:'region',empty:true,fmt:v=>v||'Не определён'},
  city:{label:'Город',param:'city',empty:true,fmt:v=>v||'Не определён'},
  language:{label:'Язык',param:'language',empty:true,fmt:v=>v||'Не определён'},
  country:{label:'Страна',param:'country',empty:true,fmt:v=>v?`${flag(v)} ${/^[A-Za-z]{2}$/.test(v)?(regionNames?.of(v)||v):v}`.trim():'Не определена'},
  network:{label:'Сеть (провайдер)',param:'network',empty:true,fmt:v=>v||'Не определена'},
  classification:{label:'Классификация',param:'classification',fmt:v=>classes[v]||v},
  bot_category:{label:'Тип краулера',param:'bot_category',fmt:v=>v?botCategories[v]||v:'Не опознаны как краулеры'},
  bot:{label:'Краулер',param:'bot',fmt:v=>v||'Без названия'},
  day:{label:'Дата',time:true,fmt:v=>ruDate(parseDate(v),{weekday:'short',day:'numeric',month:'long',year:'numeric'})},
  hour:{label:'Час',time:true,fmt:v=>`${v}:00–${v}:59`},
  weekday:{label:'День недели',time:true,fmt:v=>weekdays[Number(v)]||v},
};
const REPORTS={
  sources:{group:'Источники',title:'Источники, сводка',short:'Сводка по каналам',description:'Откуда приходят визиты: реклама, поиск, соцсети, сайты и прямые заходы — с качеством трафика и заявками.'},
  referrers:{group:'Источники',title:'Сайты-источники',short:'Сайты',description:'Сайты, с которых переходят посетители (по домену реферера).'},
  campaigns:{group:'Источники',title:'Рекламные кампании',short:'Кампании',description:'Кампания → источник → канал. Видно, какие кампании приводят людей, а какие — ботов.'},
  utm:{group:'Источники',title:'Метки UTM',short:'Метки UTM',description:'utm_source → utm_medium → utm_campaign → utm_content.'},
  terms:{group:'Источники',title:'Ключевые фразы',short:'Ключевые фразы',description:'Значения utm_term — обычно ключевые фразы или условия показа из рекламы.'},
  landing:{group:'Содержание',title:'Страницы входа',short:'Страницы входа',description:'С каких страниц начинаются визиты. Метки и click ID не дробят один адрес.'},
  exit:{group:'Содержание',title:'Страницы выхода',short:'Страницы выхода',description:'Последняя просмотренная страница визита.'},
  devices:{group:'Технологии',title:'Устройства и ОС',short:'Устройства и ОС',description:'Тип устройства → операционная система.'},
  browsers:{group:'Технологии',title:'Браузеры',short:'Браузеры',description:'Браузеры посетителей по User-Agent.'},
  networks:{group:'Технологии',title:'Сети и провайдеры',short:'Сети и провайдеры',description:'Автономные системы (провайдеры и хостинги). Высокая доля ботов из хостингов — частый признак скликивания.'},
  geo:{group:'География',title:'Страны, регионы и города',short:'География',description:'Страна → регион → город. Данные локальной GeoIP-базы или доверенного прокси.'},
 languages:{group:'Технологии',title:'Языки посетителей',short:'Языки',description:'Язык браузера посетителя.'},
  daily:{group:'Время',title:'Посещаемость по дням',short:'По дням',description:'Динамика визитов по дням в вашем часовом поясе.'},
  hours:{group:'Время',title:'Время суток',short:'По часам суток',description:'В какие часы приходят визиты, люди и заявки.'},
  activity:{group:'Время',title:'Активность по дням недели',short:'Дни недели × часы',description:'Тепловая карта: день недели × час. Помогает настроить расписание показов рекламы.'},
  quality:{group:'Качество трафика',title:'Качество трафика',short:'Классификация визитов',description:'Сколько визитов ClickShield признал людьми, подозрительными и ботами.'},
  bots:{group:'Качество трафика',title:'Известные краулеры',short:'Краулеры',description:'Поисковые и AI-краулеры, сервисы превью, мониторинга и SEO.'},
  events:{group:'Поведение',panel:'product',title:'Бизнес-события',short:'Бизнес-события',description:'События из ClickShield.track и атрибутов data-cs-event. Нажмите событие, чтобы увидеть его параметры.'},
  goals:{group:'Поведение',panel:'product',title:'Цели',short:'Цели',description:'Сколько визитов достигли цели: страница, событие или заявка.'},
  funnels:{group:'Поведение',panel:'product',title:'Воронки',short:'Воронки',description:'Шаги проходятся по порядку в одном визите. Видно, где теряются посетители.'},
  journey:{group:'Поведение',panel:'product',title:'Пути посетителей',short:'Пути по сайту',description:'Самые частые переходы между страницами: откуда приходят на страницу и куда уходят дальше.'},
  heatmap:{group:'Поведение',panel:'product',title:'Карта кликов и скроллинга',short:'Карта кликов',description:'Куда кликают и до какой глубины прокручивают страницу. Отдельно для каждой страницы и ширины окна.'},
  retention:{group:'Поведение',panel:'product',title:'Удержание и когорты',short:'Удержание',description:'Какая доля новых посетителей возвращается на сайт через N дней после первого визита.'},
  links:{group:'Инструменты',panel:'tools',title:'Короткие ссылки',short:'Короткие ссылки',description:'Ссылки /l/… для рассылок и офлайна: перенаправляют на сайт с UTM-метками и считают переходы.'},
  pixels:{group:'Инструменты',panel:'tools',title:'Пиксели',short:'Пиксели',description:'Прозрачный GIF 1×1 для писем и сторонних страниц. Считает запросы, включая автоматические проверки почтовых сервисов.'},
  annotations:{group:'Инструменты',panel:'tools',title:'Отметки на графике',short:'Отметки на графике',description:'Запуски кампаний, релизы и другие события — показываются на графиках отчётов.'},
  keys:{group:'Инструменты',panel:'tools',title:'API-ключи',short:'API-ключи',description:'Ключи для чтения аналитики одного проекта через API и MCP. Ключ показывается один раз.'},
  metrika:{group:'Инструменты',panel:'metrika',title:'Сверка с Яндекс Метрикой',short:'Сверка с Метрикой',description:'Отчёты счётчика Метрики за тот же период — чтобы сравнить цифры с ClickShield.'},
};
const REPORT_DIMS={sources:['channel','source'],referrers:['referrer'],campaigns:['campaign','source','channel'],utm:['utm_source','utm_medium','utm_campaign','utm_content'],terms:['utm_term'],landing:['landing_url'],exit:['exit_page'],devices:['device','os'],browsers:['browser'],networks:['network'],geo:['country','region','city'],languages:['language'],daily:['day'],hours:['hour'],activity:['weekday','hour'],quality:['classification'],bots:['bot_category','bot']};
const FILTER_PARAMS=[...new Set(Object.values(DIMS).map(d=>d.param).filter(p=>p&&p!=='classification'))];
const dimLabel=(k,v)=>(DIMS[k]?.fmt||(x=>x))(v??'');

// ---------- State ----------
const state={
  csrf:'',report:'sources',data:null,prev:null,prevIndex:new Map(),timeline:null,prevTimeline:null,
  filters:new Map(),segment:'',compare:false,interval:'auto',chartMetric:'visits',chartType:storage.get('chart-type','line'),
  selected:[],hidden:new Set(),colors:new Map(),sort:{key:'visits',dir:-1},expanded:new Set(),expandAll:false,
  search:'',page:0,groups:new Set(storage.get('column-groups',['traffic','behavior','quality','goals'])),
  request:0,timelineRequest:0,metrikaOffset:1,metrikaRequest:0,
};

function toast(text,bad=false){const el=$('#toast');el.textContent=text;el.classList.toggle('error',bad);el.classList.remove('hidden');clearTimeout(toast.timer);toast.timer=setTimeout(()=>el.classList.add('hidden'),4000)}
async function api(path,options={}){const r=await fetch(path,{credentials:'same-origin',...options});if(r.status===401){location.href='/login';throw Error('Требуется вход')}const d=await r.json().catch(()=>({}));if(!r.ok)throw Error(d.error||`HTTP ${r.status}`);return d}

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

// ---------- Loading ----------
// Behaviour reports, tools and the Metrica comparison live in their own panels under the same filters.
// Buttons that choose a project in place, for panels that work with one site only.
function projectPicker(){
  const options=[...$('#project-filter').options].filter(o=>o.value);
  return options.length?`<div class="rp-project-pick">${options.map(o=>`<button type="button" class="rp-button" data-pick-project="${esc(o.value)}">${esc(o.text)}</button>`).join('')}</div>`:'<p>Сначала создайте проект в разделе «Проекты».</p>';
}
document.addEventListener('click',e=>{const b=e.target.closest('[data-pick-project]');if(!b)return;$('#project-filter').value=b.dataset.pickProject;$('#project-filter').dispatchEvent(new Event('change'))});
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
function defaultSort(){state.sort=isTimeReport()?{key:'dimension',dir:1}:{key:'visits',dir:-1}}
async function loadReport({keepSelection=false}={}){
 document.dispatchEvent(new Event("clickshield:filters"));
  if(!$('#report-filters').reportValidity())return;
  if($('#date-from').value>$('#date-to').value){toast('Начало периода должно быть раньше окончания',true);return}
  const request=++state.request,meta=REPORTS[state.report];
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
      loadTimeline(),
    ]);
    if(request!==state.request)return;
    state.data=data;state.prev=prev;state.prevIndex=new Map((prev?.rows||[]).map(r=>[rowKey(r.dimensions),r]));
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
  if(isTimeReport())q.set('total_only','true');else for(const k of state.selected)q.append('series',k);
  const prevQ=state.compare?(()=>{const p=baseQuery(...previousDates());p.set('interval',interval);p.set('total_only','true');return p})():null;
  try{
    const [tl,prev]=await Promise.all([api('/v1/reports/timeline?'+q),prevQ?api('/v1/reports/timeline?'+prevQ):Promise.resolve(null)]);
    if(request!==state.timelineRequest)return;
    state.timeline=tl;state.prevTimeline=prev;
    if(!isTimeReport()&&!state.selected.length)state.selected=tl.series.map(s=>s.key);
    for(const s of tl.series)colorFor(s.key);
  }catch(e){if(request===state.timelineRequest){state.timeline=null;state.prevTimeline=null}}
}
async function reloadTimeline(){
  $('#report-chart').classList.add('loading');
  await loadTimeline();
  $('#report-chart').classList.remove('loading');renderChart();renderKpis();renderTable();
}
function colorFor(key){
  if(!state.colors.has(key)){const used=new Set(state.colors.values());state.colors.set(key,PALETTE.find(c=>!used.has(c))||PALETTE[state.colors.size%PALETTE.length])}
  return state.colors.get(key);
}
const isTimeReport=()=>!!DIMS[REPORT_DIMS[state.report]?.[0]]?.time;
const plural=(n,a,b,c)=>{const m=n%10,h=n%100;return m===1&&h!==11?a:m>=2&&m<=4&&(h<10||h>=20)?b:c};
const fmtRange=(a,b)=>a===b?ruDate(parseDate(a),{day:'numeric',month:'long',year:'numeric'}):`${ruDate(parseDate(a),{day:'numeric',month:'short'})} — ${ruDate(parseDate(b),{day:'numeric',month:'short',year:'numeric'})}`;

function renderAll(){renderKpis();renderChart();renderInsights();renderColumnGroups();renderTable()}

// ---------- Catalog and filters ----------
function renderCatalog(){
  const groups={};for(const [k,r] of Object.entries(REPORTS))(groups[r.group]||=[]).push([k,r]);
  $('#report-catalog').innerHTML=Object.entries(groups).map(([g,items])=>`<h3>${esc(g)}</h3>${items.map(([k,r])=>`<button type="button" data-report="${k}" class="${k===state.report?'active':''}" ${k===state.report?'aria-current="page"':''}>${esc(r.short)}</button>`).join('')}`).join('');
  $('#report-select').innerHTML=Object.entries(groups).map(([g,items])=>`<optgroup label="${esc(g)}">${items.map(([k,r])=>`<option value="${k}" ${k===state.report?'selected':''}>${esc(r.short)}</option>`).join('')}</optgroup>`).join('');
  for(const b of document.querySelectorAll('[data-report]'))b.onclick=()=>switchReport(b.dataset.report);
}
function switchReport(kind){if(!REPORTS[kind])return;state.report=kind;state.expanded.clear();state.expandAll=false;state.search='';$('#table-search').value='';defaultSort();loadReport()}
function renderChips(){
  for(const b of document.querySelectorAll('[data-class]'))b.classList.toggle('active',b.dataset.class===state.segment);
  const keyDim=p=>Object.entries(DIMS).find(([,d])=>d.param===p);
  $('#filter-chips').innerHTML=[...state.filters].map(([p,v])=>{const [k,d]=keyDim(p)||[p,{label:p}];return `<span class="rp-chip" title="${esc(d.label+': '+dimLabel(k,v))}"><span>${esc(d.label)}: <b>${esc(dimLabel(k,v))}</b></span><button type="button" data-unfilter="${esc(p)}" aria-label="Убрать фильтр ${esc(d.label)}">×</button></span>`}).join('')+(state.filters.size>1?'<button type="button" class="rp-button rp-small-button" data-unfilter="*">Сбросить фильтры</button>':'');
  for(const b of document.querySelectorAll('[data-unfilter]'))b.onclick=()=>{if(b.dataset.unfilter==='*')state.filters.clear();else state.filters.delete(b.dataset.unfilter);loadReport()};
}
function addFilters(dims){
  dims.forEach((v,i)=>{const k=state.data.dimensions[i],d=DIMS[k];if(k==='day'){$('#date-from').value=v;$('#date-to').value=v}else if(d?.param==='classification')state.segment=v==='UNCLASSIFIED'?'':v;else if(d?.param&&(v||d.empty))state.filters.set(d.param,v)});
  loadReport();
}

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
 setTimeout(()=>window.ClickShieldChartAnnotations?.(),0);
  const el=$('#report-chart'),m=METRICS[state.chartMetric];
  for(const b of document.querySelectorAll('[data-type]')){b.classList.toggle('active',b.dataset.type===state.chartType);b.disabled=b.dataset.type==='area'&&(!m.additive||isTimeReport())}
  $('#chart-type').classList.toggle('hidden',isTimeReport());
  window.ClickShieldChartScale=null;
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
  window.ClickShieldChartScale={top:mt,bottom:mt+ih,toX(ms){
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
    state.hidden.delete(v);reloadTimeline();
  };
  $('#report-page-label').textContent=roots.length>PAGE?`${state.page*PAGE+1}–${Math.min((state.page+1)*PAGE,roots.length)} из ${fmtInt(roots.length)}`:`${fmtInt(roots.length)} ${plural(roots.length,'строка','строки','строк')}${q?' найдено':''}`;
  $('#report-prev').disabled=state.page===0;$('#report-next').disabled=state.page>=pages-1;
}

// ---------- Export ----------
function exportReport(){
  const d=state.data;if(!d)return;
  const cell=v=>{let s=String(v??'');if(/^[=+\-@\t\r]/.test(s))s="'"+s;return '"'+s.replaceAll('"','""')+'"'};
  const keys=Object.keys(METRICS),head=[...d.dimensions.map(k=>DIMS[k]?.label||k),...keys.map(k=>METRICS[k].label),...(state.compare?keys.map(k=>METRICS[k].label+' (прошлый период)'):[])];
  const line=(dims,r,p)=>[...d.dimensions.map((k,i)=>i<dims.length?dimLabel(k,dims[i]):''),...keys.map(k=>METRICS[k].fmt(METRICS[k].value(r))),...(state.compare?keys.map(k=>p?METRICS[k].fmt(METRICS[k].value(p)):''):[])];
  const leaves=d.rows.filter(r=>r.level===d.dimensions.length);
  const lines=[head,...leaves.map(r=>line(r.dimensions,r,state.prevIndex.get(rowKey(r.dimensions)))),['Итого',...line([],d.totals,state.prev?.totals).slice(1)]];
  const blob=new Blob(['﻿'+lines.map(l=>l.map(cell).join(';')).join('\r\n')],{type:'text/csv;charset=utf-8'}),url=URL.createObjectURL(blob),a=document.createElement('a');
  a.href=url;a.download=`clickshield-${state.report}-${$('#date-from').value}-${$('#date-to').value}.csv`;a.click();setTimeout(()=>URL.revokeObjectURL(url),1000);
}

// ---------- Yandex Metrica comparison ----------
function clearMetrika(){++state.metrikaRequest;state.metrikaOffset=1;$('#metrika-report-table').replaceChildren();$('#metrika-report-status').textContent='Фильтры изменились. Загрузите отчёт счётчика заново.';$('#metrika-prev').disabled=true;$('#metrika-next').disabled=true;$('#load-metrika-report').disabled=false}
async function loadMetrikaReport(reset=true){const pid=$('#project-filter').value;if(!pid){toast('Выберите один проект',true);return}if(reset)state.metrikaOffset=1;const request=++state.metrikaRequest;const button=$('#load-metrika-report');button.disabled=true;$('#metrika-prev').disabled=true;$('#metrika-next').disabled=true;$('#metrika-report-status').textContent='Запрашиваем данные Метрики…';try{const q=new URLSearchParams({report:$('#metrika-report-kind').value,from:$('#date-from').value,to:$('#date-to').value,offset:String(state.metrikaOffset)});const data=await api(`/v1/projects/${pid}/metrika-report?${q}`);if(request!==state.metrikaRequest)return;$('#metrika-report-status').textContent=`Строки ${data.data.length?state.metrikaOffset:0}–${state.metrikaOffset+data.data.length-1} из ${data.total_rows}${data.sampled?' · Семплирование '+Math.round(data.sample_share*100)+'%':''}. Модель атрибуции: последний источник. Данные Метрики могут отличаться от ClickShield.`;$('#metrika-report-table').innerHTML=data.data.length?`<table class="rp-simple-table"><thead><tr><th>Группировка</th><th class="num">Визиты</th><th class="num">Посетители</th><th class="num">Отказы, %</th><th class="num">Время, с</th></tr></thead><tbody>${data.data.map(r=>`<tr><td>${r.dimensions.map(d=>esc(d.name||d.id||'—')).join(' / ')}</td>${r.metrics.map(m=>`<td class="num">${Number(m).toLocaleString('ru-RU',{maximumFractionDigits:1})}</td>`).join('')}</tr>`).join('')}</tbody></table>`:'<p class="empty">Данных нет.</p>';$('#metrika-prev').disabled=state.metrikaOffset===1;$('#metrika-next').disabled=state.metrikaOffset+data.data.length>data.total_rows}catch(e){if(request!==state.metrikaRequest)return;$('#metrika-report-status').textContent=e.message;$('#metrika-report-table').replaceChildren()}finally{if(request===state.metrikaRequest)button.disabled=false}}

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
  let width=0;new ResizeObserver(([e])=>{const w=Math.round(e.contentRect.width);if(w!==width&&state.data){width=w;renderChart()}}).observe($('#report-chart'));
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
init().catch(e=>{$('#report-status').textContent=e.message;toast(e.message,true)});
