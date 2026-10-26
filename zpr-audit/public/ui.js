'use strict';

const $=id=>document.getElementById(id);
function reportTable(headers,rows){return '<div class="table-wrap"><table><thead><tr>'+headers.map(h=>'<th>'+escapeHTML(h)+'</th>').join('')+'</tr></thead><tbody>'+rows.map(r=>'<tr>'+r.map(c=>'<td>'+escapeHTML(c)+'</td>').join('')+'</tr>').join('')+'</tbody></table></div>'}
function dialog(title,body){$('detail-title').textContent=title;$('detail-body').innerHTML=body;$('detail').showModal()}
function notImplemented(action){dialog('Not Implemented','<p><strong>'+escapeHTML(action)+'</strong></p><p>This action needs backend plumbing. The prototype has not contacted a service, run policy, exported evidence or changed any state.</p>')}
function escapeHTML(s){return String(s).replace(/[&<>"']/g,c=>({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c]))}
function selectOptions(id,values){$(id).replaceChildren(...values.map(v=>{const o=document.createElement('option');o.textContent=v;return o}))}
