import {$, fmtInt, fmtDec, fmtPct, fmtDur, ratio, parseDate, ruDate} from "./analytics-common.js";

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
  metrika:{group:'Инструменты',panel:'metrika',title:'Сверка с Яндекс Метрикой',short:'Сверка с Метрикой',description:'Отчёты счётчика Метрики за тот же период — чтобы сравнить цифры с ClickShield.'},
};
const REPORT_DIMS={sources:['channel','source'],referrers:['referrer'],campaigns:['campaign','source','channel'],utm:['utm_source','utm_medium','utm_campaign','utm_content'],terms:['utm_term'],landing:['landing_url'],exit:['exit_page'],devices:['device','os'],browsers:['browser'],networks:['network'],geo:['country','region','city'],languages:['language'],daily:['day'],hours:['hour'],activity:['weekday','hour'],quality:['classification'],bots:['bot_category','bot']};
const FILTER_PARAMS=[...new Set(Object.values(DIMS).map(d=>d.param).filter(p=>p&&p!=='classification'))];
const dimLabel=(k,v)=>(DIMS[k]?.fmt||(x=>x))(v??'');


export {METRICS, COLUMN_GROUPS, KPIS, channels, sources, classes, botCategories, weekdays, regionNames, flag, notSet, DIMS, REPORTS, REPORT_DIMS, FILTER_PARAMS, dimLabel};
