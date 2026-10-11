(() => {
 'use strict';
 // Uses state, api, baseQuery, esc and fmtInt from analytics.js.
 const el=id=>document.getElementById(id);
 let request=0,items=[],editingKind='',editingProject='',chartRequest=0;
 const mutate=(path,method,body)=>api(path,{method,headers:{'Content-Type':'application/json','X-CSRF-Token':state.csrf},body:body?JSON.stringify(body):undefined});
 const path=(pid,kind)=>kind==='keys'?`/v1/projects/${pid}/analytics-keys`:`/v1/projects/${pid}/tools/${kind}`;
 const status=t=>{el('tools-status').textContent=t};
 const empty=(title,text)=>`<div class="rp-empty-state"><strong>${esc(title)}</strong><p>${esc(text)}</p></div>`;
 const pill=(on,yes,no)=>`<span class="rp-pill ${on?'on':'off'}">${esc(on?yes:no)}</span>`;
 const when=v=>new Date(v).toLocaleString('ru-RU',{day:'numeric',month:'short',year:'numeric',hour:'2-digit',minute:'2-digit'});
 const copyButton=text=>`<button type="button" class="rp-button rp-small-button" data-copy="${esc(text)}">Копировать</button>`;
 const emptyTexts={links:['Ссылок пока нет','Создайте короткую ссылку для письма, QR-кода или поста — переходы по ней будут посчитаны, а UTM-метки сохранятся.'],pixels:['Пикселей пока нет','Пиксель вставляется в письмо или стороннюю страницу и считает её открытия.'],annotations:['Отметок пока нет','Отметьте запуск кампании или релиз — отметка появится на графиках отчётов.'],keys:['Ключей пока нет','Выпустите ключ, чтобы читать аналитику проекта через API или MCP.']};

 async function loadTools(){
  const pid=el('project-filter').value,kind=el('tool-kind').value,n=++request,root=el('tools-result');
  if(!pid){root.innerHTML=empty('Выберите проект','Инструменты настраиваются для одного сайта.').replace('</div>',projectPicker()+'</div>');status('');el('new-tool').disabled=true;el('new-tool').title='Сначала выберите проект';return}
  el('new-tool').disabled=false;el('new-tool').title='';root.classList.add('loading');status('Загружаем…');
  try{
   const r=await api(path(pid,kind)+'?'+baseQuery());if(n!==request)return;
   items=kind==='keys'?r.keys:r.data;root.classList.remove('loading');status('');
   if(!items.length){root.innerHTML=empty(...emptyTexts[kind]);return}
   const head={links:['Ссылка','Короткий адрес','Куда ведёт','Переходы',''],pixels:['Пиксель','HTML-код для вставки','Запросы',''],annotations:['Когда','Отметка',''],keys:['Ключ','Префикс','Действует до','Последнее использование','']}[kind];
   const rows=items.map(item=>{
    const actions=`<td class="rp-actions-cell">${kind!=='keys'?`<button type="button" class="rp-button rp-small-button" data-edit="${esc(item.id)}">Изменить</button>`:''}<button type="button" class="rp-button rp-small-button" data-delete="${esc(item.id)}" ${item.revoked_at?'disabled':''}>${kind==='keys'?'Отозвать':'Удалить'}</button></td>`;
    if(kind==='links'){const url=location.origin+'/l/'+item.slug;return `<tr><td><strong>${esc(item.name)}</strong> ${pill(item.enabled,'включена','выключена')}</td><td><div class="rp-copy"><code>${esc(url)}</code>${copyButton(url)}</div></td><td class="rp-wrap" title="${esc(item.destination)}">${esc(item.destination)}</td><td class="num">${fmtInt(item.hits)}</td>${actions}</tr>`}
    if(kind==='pixels'){const code=`<img src="${location.origin}/p/${item.id}" width="1" height="1" alt="">`;return `<tr><td><strong>${esc(item.name)}</strong> ${pill(item.enabled,'включён','выключен')}</td><td><div class="rp-copy"><code>${esc(code)}</code>${copyButton(code)}</div></td><td class="num">${fmtInt(item.hits)}</td>${actions}</tr>`}
    if(kind==='annotations')return `<tr><td class="rp-nowrap">${esc(when(item.at))}</td><td class="rp-wrap">${esc(item.text)}</td>${actions}</tr>`;
    const expired=new Date(item.expires_at)<new Date();
    return `<tr><td><strong>${esc(item.name)}</strong> ${item.revoked_at?pill(false,'','отозван'):expired?pill(false,'','истёк'):pill(true,'действует','')}</td><td><code>${esc(item.prefix)}…</code></td><td class="rp-nowrap">${esc(when(item.expires_at))}</td><td class="rp-nowrap">${item.last_used_at?esc(when(item.last_used_at)):'—'}</td>${actions}</tr>`;
   });
   root.innerHTML=`<table class="rp-simple-table"><thead><tr>${head.map((h,i)=>`<th class="${['Переходы','Запросы'].includes(h)?'num':''}">${esc(h)}</th>`).join('')}</tr></thead><tbody>${rows.join('')}</tbody></table>`;
   for(const b of root.querySelectorAll('[data-copy]'))b.onclick=async()=>{try{await navigator.clipboard.writeText(b.dataset.copy);toast('Скопировано')}catch{toast('Не удалось скопировать',true)}};
   for(const b of root.querySelectorAll('[data-edit]'))b.onclick=()=>openTool(items.find(v=>v.id===b.dataset.edit));
   for(const b of root.querySelectorAll('[data-delete]'))b.onclick=async()=>{
    if(b.dataset.armed!=='1'){const label=b.textContent;b.dataset.armed='1';b.textContent='Точно?';b.classList.add('rp-danger');setTimeout(()=>{if(b.isConnected){b.dataset.armed='';b.textContent=label;b.classList.remove('rp-danger')}},4000);return}
    try{await mutate(path(pid,kind)+'/'+b.dataset.delete,'DELETE');await loadTools();window.ClickShieldChartAnnotations()}catch(e){status(e.message)}
   };
  }catch(e){if(n===request){root.classList.remove('loading');status('Не загружено: '+e.message)}}
 }
 function openTool(item){
  editingProject=el('project-filter').value;editingKind=el('tool-kind').value;if(!editingProject)return;
  el('tool-id').value=item?.id||'';el('tool-error').textContent='';
  const fields=el('tool-fields');fields.replaceChildren();
  const input=(label,id,type,value,max,hint)=>{const l=document.createElement('label');l.append(label);const i=document.createElement('input');i.id=id;i.type=type;i.value=value??'';i.required=true;if(max)i.maxLength=max;if(hint)i.placeholder=hint;l.append(i);fields.append(l);return i};
  if(editingKind==='annotations'){
   el('tool-dialog-title').textContent=item?'Изменить отметку':'Новая отметка';
   const date=new Date(item?.at||Date.now());input('Дата и время','tool-at','datetime-local',new Date(date.getTime()-date.getTimezoneOffset()*60000).toISOString().slice(0,16));input('Текст','tool-text','text',item?.text,500,'Например, запуск кампании в VK');
  }else{
   el('tool-dialog-title').textContent=({links:item?'Изменить ссылку':'Новая короткая ссылка',pixels:item?'Изменить пиксель':'Новый пиксель',keys:'Новый API-ключ'})[editingKind];
   input('Название','tool-name','text',item?.name,120,editingKind==='links'?'Например, октябрьская рассылка':'');
   if(editingKind==='links'){input('Адрес назначения с UTM-метками','tool-destination','url',item?.destination,4096,'https://example.ru/promo?utm_source=mail');input('Короткий адрес (необязательно)','tool-slug','text',item?.slug,80,'oct-mail').required=false}
   else if(editingKind==='keys'){const i=input('Срок действия, дней','tool-days','number',90);i.min=1;i.max=365}
   if(editingKind!=='keys'){const l=document.createElement('label');l.className='rp-check';const i=document.createElement('input');i.type='checkbox';i.id='tool-enabled';i.checked=item?.enabled??true;l.append(i,editingKind==='links'?' Ссылка работает':' Пиксель работает');fields.append(l)}
  }
  el('tool-dialog').showModal();
 }
 el('tool-form').onsubmit=async e=>{e.preventDefault();try{const id=el('tool-id').value,body=editingKind==='annotations'?{at:new Date(el('tool-at').value).toISOString(),text:el('tool-text').value}:editingKind==='keys'?{name:el('tool-name').value,days:Number(el('tool-days').value)}:editingKind==='links'?{name:el('tool-name').value,destination:el('tool-destination').value,slug:el('tool-slug').value,enabled:el('tool-enabled').checked}:{name:el('tool-name').value,enabled:el('tool-enabled').checked};const r=await mutate(path(editingProject,editingKind)+(id?'/'+id:''),id?'PUT':'POST',body);el('tool-dialog').close();if(editingKind==='keys'){el('new-key-token').value=r.token;el('key-result').hidden=false}await loadTools();window.ClickShieldChartAnnotations()}catch(e){el('tool-error').textContent=e.message}};
 el('load-tools').onclick=loadTools;el('tool-kind').onchange=()=>{el('new-key-token').value='';el('key-result').hidden=true;loadTools()};el('new-tool').onclick=()=>openTool();el('close-tool').onclick=()=>el('tool-dialog').close();el('hide-key').onclick=()=>{el('new-key-token').value='';el('key-result').hidden=true};
 el('copy-key').onclick=async()=>{try{await navigator.clipboard.writeText(el('new-key-token').value);toast('Ключ скопирован')}catch{el('new-key-token').select();status('Скопируйте выделенный ключ.')}};
 document.addEventListener('clickshield:filters',()=>{++request;el('tools-result').replaceChildren();status('');el('key-result').hidden=true;el('new-key-token').value=''});

 // Marks annotations on the time chart, using the scale analytics.js publishes for it.
 window.ClickShieldChartAnnotations=async()=>{
  const n=++chartRequest,pid=el('project-filter').value,chart=el('report-chart');
  chart.querySelectorAll('.chart-annotation-marker').forEach(e=>e.remove());el('chart-annotations')?.replaceChildren();
  if(!pid)return;
  try{
   const r=await api(`/v1/projects/${pid}/tools/annotations?`+baseQuery());if(n!==chartRequest||!r.data.length)return;
   const list=el('chart-annotations');if(list)list.innerHTML=r.data.map(a=>`<span title="${esc(when(a.at))}"><i></i>${esc(new Date(a.at).toLocaleDateString('ru-RU',{day:'numeric',month:'short'}))} — ${esc(a.text)}</span>`).join('');
   const svg=chart.querySelector('svg'),scale=window.ClickShieldChartScale;if(!svg||!scale)return;
   for(const a of r.data){
    const x=scale.toX(new Date(a.at).getTime());if(x==null)continue;
    const g=document.createElementNS('http://www.w3.org/2000/svg','g');g.classList.add('chart-annotation-marker');
    const title=document.createElementNS(g.namespaceURI,'title');title.textContent=when(a.at)+' — '+a.text;
    const line=document.createElementNS(g.namespaceURI,'line');for(const[k,v]of Object.entries({x1:x,x2:x,y1:scale.top,y2:scale.bottom,stroke:'#bd6600','stroke-dasharray':'3 4','stroke-width':1.5}))line.setAttribute(k,v);
    const dot=document.createElementNS(g.namespaceURI,'circle');for(const[k,v]of Object.entries({cx:x,cy:scale.top,r:5,fill:'#bd6600',stroke:'#fff','stroke-width':2}))dot.setAttribute(k,v);
    g.append(title,line,dot);svg.insertBefore(g,svg.querySelector('.hit'));
   }
  }catch{}
 };
})();
