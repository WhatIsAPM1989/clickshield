'use strict';
const inviteErrors={invalid:'Ссылка недействительна или уже использована. Попросите новую.',email:'Укажите корректный email.',short:'Пароль должен быть не короче 12 символов.',mismatch:'Пароли не совпадают.',exists:'Пользователь с таким email уже есть.'};
const code=new URLSearchParams(location.search).get('error');
if(code){const el=document.getElementById('invite-error');el.textContent=inviteErrors[code]||'Ошибка.';el.classList.remove('hidden');if(code==='invalid')document.getElementById('invite-form').remove()}
document.getElementById('invite-form')?.setAttribute('action',location.pathname);
