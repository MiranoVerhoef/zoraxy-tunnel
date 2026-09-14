let SERVICE_HEALTH={},ACTIVITY_EVENTS=[];

(function installObservabilityUI(){
  const link=document.createElement('link');link.rel='stylesheet';link.href='health.css';document.head.appendChild(link);
  const nav=document.querySelector('.tabs-nav');
  if(nav&&!document.getElementById('tabBtnActivity')){
    const b=document.createElement('button');b.className='tab';b.id='tabBtnActivity';b.innerHTML='<span>◷</span>Activity';b.onclick=()=>showTab('activity');nav.appendChild(b);
  }
  const main=document.querySelector('main');
  if(main&&!document.getElementById('viewActivity')){
    const section=document.createElement('section');section.id='viewActivity';section.className='view';section.innerHTML=`<div class="activity-card"><div class="activity-toolbar"><div><h2>Activity</h2><p>Connector, failover and service-health events are retained across plugin restarts.</p></div><select class="select" id="activityFilter" onchange="renderActivity()"><option value="all">All events</option><option value="connector">Connectors</option><option value="service">Service health</option><option value="plugin">Plugin</option></select><button class="btn secondary" onclick="loadObservability(true)">Refresh</button></div><div class="activity-list" id="activityList"><div class="activity-empty">Loading activity…</div></div><div class="activity-summary" id="activitySummary"></div></div>`;main.appendChild(section);
  }
})();

const _baseShowTab=showTab;
showTab=function(name){
  if(name!=='activity'){
    document.getElementById('viewActivity')?.classList.remove('active');
    document.getElementById('tabBtnActivity')?.classList.remove('active');
    return _baseShowTab(name);
  }
  document.getElementById('viewTunnels')?.classList.remove('active');
  document.getElementById('viewSettings')?.classList.remove('active');
  document.getElementById('viewActivity')?.classList.add('active');
  document.getElementById('tabBtnTunnels')?.classList.remove('active');
  document.getElementById('tabBtnSettings')?.classList.remove('active');
  document.getElementById('tabBtnActivity')?.classList.add('active');
  renderActivity();
};

function healthFor(tid,sid){return SERVICE_HEALTH[tid+'/'+sid]||null}
function healthChip(tid,sid){
  const h=healthFor(tid,sid);if(!h)return '<span class="health-chip">Checking</span>';
  let label=h.status||'unknown';if(label==='healthy'&&h.latency_ms>0)label=`Healthy · ${h.latency_ms} ms`;else label=label[0].toUpperCase()+label.slice(1);
  const title=h.error?` title="${esc(h.error)}"`:'';return `<span class="health-chip ${esc(h.status)}"${title}>${esc(label)}</span>`;
}

const _baseRenderServiceRow=renderServiceRow;
renderServiceRow=function(tid,s){
  const installed=!!s.installed_route,menuId='s-'+tid+'-'+s.id,scheme=(String(s.target||'').split(':')[0]||'http').toUpperCase(),target=String(s.target||'').replace(/^https?:\/\//i,'');
  return `<div class="service-row"><div class="service-main"><div class="service-state"><span class="status-dot ${installed&&s.enabled?'ok':''}"></span></div><div class="service-copy"><strong>${esc(s.name||s.host)}</strong><span>${esc(s.host)}${s.path?esc(s.path):''}</span></div></div><div class="service-target"><div class="route-line"><span>${esc(scheme)} → </span>${esc(target)}</div><div class="service-flags">${healthChip(tid,s.id)}${s.skip_tls_verify?'<span class="warning-chip">⚠ TLS verification off</span>':''}${!installed?'<span class="warning-chip">Route not installed</span>':!s.enabled?'<span class="warning-chip">Disabled</span>':'<span class="ok-chip">Route installed</span>'}</div></div><div class="service-actions">${installed?`<button class="btn secondary compact" onclick="openService('${tid}','${s.id}')">↗ Open</button>`:''}<div class="manage-wrap"><button class="icon-btn" onclick="toggleActionMenu('${menuId}',event)">•••</button>${renderServiceMenu(tid,s,menuId)}</div></div></div>`;
};

function resolveEventContext(e){
  const t=TUNNELS.find(x=>x.id===e.tunnel_id),s=t&&(t.services||[]).find(x=>x.id===e.service_id);const bits=[];
  if(t)bits.push(t.name);else if(e.tunnel_id)bits.push(e.tunnel_id);
  if(e.connector_id)bits.push(e.connector_id);
  if(s)bits.push(s.name||s.host);else if(e.service_id)bits.push(e.service_id);
  return bits.join(' · ')||'System';
}
function renderActivity(){
  const el=document.getElementById('activityList');if(!el)return;const f=document.getElementById('activityFilter')?.value||'all';
  const rows=ACTIVITY_EVENTS.filter(e=>f==='all'||String(e.type||'').startsWith(f+'.'));
  el.innerHTML=rows.length?rows.map(e=>`<div class="activity-row"><div class="activity-time">${new Date(e.time).toLocaleString()}</div><div><span class="activity-level ${esc(e.level||'info')}">${esc(e.level||'info')}</span></div><div class="activity-message">${esc(e.message||e.type||'Event')}</div><div class="activity-context">${esc(resolveEventContext(e))}</div></div>`).join(''):'<div class="activity-empty">No matching events yet.</div>';
  const summary=document.getElementById('activitySummary');if(summary)summary.textContent=`${rows.length} event${rows.length===1?'':'s'} shown · up to 500 events retained locally`;
}

async function loadObservability(showToast=false){
  const [h,e]=await Promise.all([api('health'),api('events?limit=200')]);
  if(h.ok&&Array.isArray(h.data)){SERVICE_HEALTH={};h.data.forEach(x=>SERVICE_HEALTH[x.tunnel_id+'/'+x.service_id]=x);if(typeof renderTunnels==='function')renderTunnels();}
  if(e.ok&&Array.isArray(e.data)){ACTIVITY_EVENTS=e.data;renderActivity();}
  if(showToast)toast(h.ok&&e.ok?'Observability refreshed':'Could not refresh all observability data');
}

setTimeout(loadObservability,600);
setInterval(()=>{if(!anyModalOpen()&&!inputFocused())loadObservability(false)},10000);
