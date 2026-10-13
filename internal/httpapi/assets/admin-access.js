'use strict';
// Owner-only "Доступ" tab: invite links and user activation. Uses api/toast/esc/fmtDate from admin.js.
async function loadAccess(){const [inv,usr]=await Promise.all([api('/v1/invites'),api('/v1/users')]);
$('#invites').innerHTML=inv.invites.length?inv.invites.map(i=>`<tr><td>${fmtDate(i.created_at)}</td><td>${fmtDate(i.expires_at)}</td><td><button data-revoke="${esc(i.id)}" type="button">Отозвать</button></td></tr>`).join(''):'<tr><td colspan="3" class="empty">Нет активных ссылок.</td></tr>';
$('#users').innerHTML=usr.users.map(u=>`<tr><td>${esc(u.username)}</td><td>${fmtDate(u.created_at)}</td><td>${u.active?'Активен':'<span class="muted">Отключён</span>'}</td><td>${u.username.toLowerCase()===$('#username').textContent.toLowerCase()?'':`<button data-user="${esc(u.id)}" data-active="${!u.active}" type="button">${u.active?'Отключить':'Включить'}</button>`}</td></tr>`).join('')}
async function accessAction(fn){try{await fn();await loadAccess()}catch(e){toast(e.message,true)}}
$('#create-invite').onclick=()=>accessAction(async()=>{const r=await api('/v1/invites',{method:'POST'});$('#access-url').value=location.origin+r.path;$('#access-link').classList.remove('hidden');$('#access-link-hint').classList.remove('hidden')});
$('#copy-invite').onclick=()=>navigator.clipboard.writeText($('#access-url').value).then(()=>toast('Ссылка скопирована'),()=>{$('#access-url').select();toast('Скопируйте ссылку вручную',true)});
$('#invites').onclick=e=>{const id=e.target.dataset.revoke;if(id)accessAction(()=>api(`/v1/invites/${id}`,{method:'DELETE'}))};
$('#users').onclick=e=>{const id=e.target.dataset.user;if(id)accessAction(()=>api(`/v1/users/${id}/active`,{method:'POST',body:{active:e.target.dataset.active==='true'}}))};
document.querySelector('[data-view="access"]').addEventListener('click',()=>loadAccess().catch(e=>toast(e.message,true)));
