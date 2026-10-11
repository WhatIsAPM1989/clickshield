import {$} from "./analytics-common.js";
import {METRICS, DIMS, dimLabel} from "./analytics-model.js";
import {state} from "./analytics-state.js";
import {rowKey} from "./analytics-filters.js";

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


export {exportReport};
