

'use strict';
const $=q=>document.querySelector(q);
const esc=v=>String(v??'').replace(/[&<>'"]/g,c=>({'&':'&amp;','<':'&lt;','>':'&gt;',"'":'&#39;','"':'&quot;'}[c]));
const zone=Intl.DateTimeFormat().resolvedOptions().timeZone||'Europe/Moscow';
const PAGE=50,MAX_SERIES=20;
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


const plural=(n,a,b,c)=>{const m=n%10,h=n%100;return m===1&&h!==11?a:m>=2&&m<=4&&(h<10||h>=20)?b:c};
const fmtRange=(a,b)=>a===b?ruDate(parseDate(a),{day:'numeric',month:'long',year:'numeric'}):`${ruDate(parseDate(a),{day:'numeric',month:'short'})} — ${ruDate(parseDate(b),{day:'numeric',month:'short',year:'numeric'})}`;

export {$, esc, zone, PAGE, PALETTE, SEQUENTIAL, storage, fmtInt, fmtDec, fmtPct, fmtDur, ratio, pad, isoDate, parseDate, parseBucket, ruDate, plural, fmtRange, MAX_SERIES};
