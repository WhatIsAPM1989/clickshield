import {$} from "./analytics-common.js";

function toast(text,bad=false){const el=$('#toast');el.textContent=text;el.classList.toggle('error',bad);el.classList.remove('hidden');clearTimeout(toast.timer);toast.timer=setTimeout(()=>el.classList.add('hidden'),4000)}
async function api(path,options={}){const r=await fetch(path,{credentials:'same-origin',...options});if(r.status===401){location.href='/login';throw Error('Требуется вход')}const d=await r.json().catch(()=>({}));if(!r.ok)throw Error(d.error||`HTTP ${r.status}`);return d}


export {toast, api};
