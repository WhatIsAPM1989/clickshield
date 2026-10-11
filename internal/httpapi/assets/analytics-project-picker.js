import {$,esc} from "./analytics-common.js";

function projectPicker(){
  const options=[...$('#project-filter').options].filter(o=>o.value);
  return options.length?`<div class="rp-project-pick">${options.map(o=>`<button type="button" class="rp-button" data-pick-project="${esc(o.value)}">${esc(o.text)}</button>`).join('')}</div>`:'<p>Сначала создайте проект в разделе «Проекты».</p>';
}
document.addEventListener('click',e=>{const b=e.target.closest('[data-pick-project]');if(!b)return;$('#project-filter').value=b.dataset.pickProject;$('#project-filter').dispatchEvent(new Event('change'))});

export {projectPicker};
