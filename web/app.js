function applyTheme(){document.body.classList.toggle('darkTheme',localStorage.getItem('theme')==='dark')}
applyTheme();window.addEventListener('storage',applyTheme)
function apiBase(){let p=window.location.pathname.replace(/index\.html$/,'');const i=p.lastIndexOf('/ui/');if(i>=0)return p.slice(0,i+4)+'api/';if(!p.endsWith('/'))p+='/';return p+'api/'}
const API=apiBase(),CSRF=document.querySelector('meta[name="zoraxy.csrf.Token"]').content,REPO='https://github.com/MiranoVerhoef/zoraxy-tunnel',IMAGE='ghcr.io/miranoverhoef/zoraxy-tunnel-client'
let STATUS={fingerprint:'',server_host:'',default_tag:'',version:'',control_port:9443,ingress_port:9080},TUNNELS=[],CLIENT_STATS={},svcTunnelId=null,svcServiceId=null,confirmCb=null,installTunnelId=null,installServiceId=null,currentPage=1,pageSize=10,OPEN_MENU=null
const OPEN_TUNNELS=new Set()

async function api(path,opts={}){opts.headers=opts.headers||{};opts.headers['X-CSRF-Token']=CSRF;if(opts.body)opts.headers['Content-Type']='application/json';try{const r=await fetch(API+path,opts),text=await r.text();let data=null;if(text){try{data=JSON.parse(text)}catch{data={error:text}}}return{ok:r.ok,status:r.status,data}}catch{return{ok:false,status:0,data:null}}}
function esc(s){return String(s??'').replace(/[&<>"]/g,c=>({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;'}[c]))}
function toast(msg){const t=document.getElementById('toast');t.textContent=msg;t.classList.add('show');clearTimeout(t._h);t._h=setTimeout(()=>t.classList.remove('show'),1800)}
async function copyText(s){if(!s)return;try{await navigator.clipboard.writeText(s);toast('Copied')}catch{toast('Copy failed')}}
function copyFrom(id){copyText(document.getElementById(id).textContent)}
function closeModal(id){document.getElementById(id).classList.remove('show')}
function anyModalOpen(){return !!document.querySelector('.overlay.show')}
function inputFocused(){const a=document.activeElement;return a&&['INPUT','SELECT','TEXTAREA'].includes(a.tagName)}
function releaseBase(){return REPO+'/releases/download/'+(STATUS.version||'latest')+'/'}
function releasePage(){return REPO+'/releases/'+(STATUS.version?'tag/'+STATUS.version:'latest')}

function normalizeEndpoint(raw){
  let s=String(raw||'').trim(),port=String(STATUS.control_port||9443)
  if(!s)return{ok:false,error:'Enter a public hostname or IP address.'}
  s=s.replace(/^https?:\/\//i,'').replace(/\/$/,'')
  if(/[\/?#\s]/.test(s))return{ok:false,error:'Use only a hostname/IP address and optional port.'}
  if(s.startsWith('[')){
    const end=s.indexOf(']');if(end<0)return{ok:false,error:'Invalid IPv6 address.'}
    const host=s.slice(0,end+1),rest=s.slice(end+1)
    if(!rest)return{ok:true,value:host+':'+port}
    if(!/^:\d+$/.test(rest))return{ok:false,error:'Invalid port.'}
    const p=Number(rest.slice(1));return p>0&&p<=65535?{ok:true,value:host+rest}:{ok:false,error:'Port must be between 1 and 65535.'}
  }
  const colons=(s.match(/:/g)||[]).length
  if(colons===0)return{ok:true,value:s+':'+port}
  if(colons===1){const i=s.lastIndexOf(':'),host=s.slice(0,i),ps=s.slice(i+1),p=Number(ps);if(!host||!/^\d+$/.test(ps)||p<1||p>65535)return{ok:false,error:'Invalid hostname or port.'};return{ok:true,value:host+':'+p}}
  return{ok:true,value:'['+s+']:'+port}
}
function endpointFromStatus(){return normalizeEndpoint(STATUS.server_host)}
function updateEndpointPreview(){const box=document.getElementById('endpointPreview'),r=normalizeEndpoint(document.getElementById('serverHost').value),b=box.querySelector('strong');box.classList.toggle('invalid',!r.ok);b.textContent=r.ok?r.value:r.error;return r}

function showTab(name){
  document.getElementById('viewTunnels').classList.toggle('active',name==='tunnels')
  document.getElementById('viewSettings').classList.toggle('active',name==='settings')
  document.getElementById('tabBtnTunnels').classList.toggle('active',name==='tunnels')
  document.getElementById('tabBtnSettings').classList.toggle('active',name==='settings')
}
function openDocs(){openExternal(REPO+'#readme')}
function openExternal(url){try{window.parent.open(url,'_blank','noopener,noreferrer')}catch{window.open(url,'_blank')}}
function openCreateTunnel(){const ep=endpointFromStatus();if(!ep.ok){showTab('settings');toast('Configure the Control Node endpoint first');return}document.getElementById('newTunnel').value='';document.getElementById('createTunnelModal').classList.add('show');setTimeout(()=>document.getElementById('newTunnel').focus(),50)}

function formatBytes(n){n=Number(n||0);if(n<1024)return n+' B';const u=['KB','MB','GB','TB'];let i=-1;do{n/=1024;i++}while(n>=1024&&i<u.length-1);return n.toFixed(n>=10?1:2)+' '+u[i]}
function formatAge(iso){if(!iso)return'—';const ms=Math.max(0,Date.now()-new Date(iso).getTime());let s=Math.floor(ms/1000),d=Math.floor(s/86400);s%=86400;let h=Math.floor(s/3600);s%=3600;let m=Math.floor(s/60);const p=[];if(d)p.push(d+'d');if(h)p.push(h+'h');if(m||!p.length)p.push(m+'m');return p.slice(0,2).join(' ')}
function platform(c){return[c?.os,c?.arch].filter(Boolean).join(' / ')||'unknown'}
function versionMismatch(c){return !!(c?.version&&STATUS.version&&c.version!==STATUS.version)}
function tunnelState(t){if(!t.enabled)return'disabled';return t.online?'online':'offline'}
function oldestOnlineConnection(){const times=TUNNELS.filter(t=>t.online).map(t=>CLIENT_STATS[t.id]?.connected_at).filter(Boolean).map(x=>new Date(x).getTime()).filter(Number.isFinite);return times.length?new Date(Math.min(...times)).toISOString():''}

async function loadStatus(){
  const {data}=await api('status');if(!data)return;STATUS=data
  document.getElementById('versionPill').textContent=data.version||'—'
  document.getElementById('fingerprint').textContent=data.fingerprint||'—'
  document.getElementById('controlPort').textContent=':'+data.control_port
  document.getElementById('ingressPort').textContent=':'+data.ingress_port
  document.getElementById('summaryIngress').textContent=':'+data.ingress_port
  const serverInput=document.getElementById('serverHost');if(document.activeElement!==serverInput)serverInput.value=data.server_host||''
  const tagInput=document.getElementById('defaultTag');if(document.activeElement!==tagInput)tagInput.value=data.default_tag||''
  const ep=normalizeEndpoint(data.server_host)
  document.getElementById('summaryControl').textContent=ep.ok?ep.value:'Not configured'
  document.getElementById('settingsStatus').textContent=ep.ok?'Configured':'Not configured'
  document.getElementById('settingsStatus').classList.toggle('ok',ep.ok)
  document.getElementById('setupBanner').classList.toggle('hidden',ep.ok)
  updateEndpointPreview()
  updateGlobalStatus()
}
function updateGlobalStatus(){
  const online=TUNNELS.filter(t=>t.online).length,dot=document.getElementById('summaryDot'),label=document.getElementById('summaryStatus'),sub=document.getElementById('summaryUptime')
  dot.classList.toggle('ok',online>0)
  label.textContent=online>0?'Online':'Offline'
  const since=oldestOnlineConnection();sub.textContent=online>0?(since?formatAge(since)+' uptime':online+' client'+(online===1?'':'s')+' connected'):'No clients connected'
}
async function saveSettings(){
  const ep=updateEndpointPreview();if(!ep.ok){toast(ep.error);return}
  const default_tag=document.getElementById('defaultTag').value.trim(),{ok,data}=await api('settings',{method:'POST',body:JSON.stringify({server_host:ep.value,default_tag})})
  if(!ok){toast(data?.error||'Could not save settings');return}
  document.getElementById('serverHost').value=ep.value;toast('Control Node saved');await loadStatus()
}
async function copyConnectionInfo(){const ep=endpointFromStatus();if(!ep.ok){showTab('settings');toast('Configure the Control Node endpoint first');return}copyText(`Zoraxy Tunnel Enhanced\nControl endpoint: ${ep.value}\nIngress: :${STATUS.ingress_port}\nFingerprint: ${STATUS.fingerprint}`)}

async function loadTunnels(){
  const [tRes,cRes]=await Promise.all([api('tunnels'),api('client-stats')])
  if(!tRes.ok){document.getElementById('tunnels').innerHTML='<div class="empty-state"><strong>Could not load tunnels</strong><span>Check the plugin status and try again.</span></div>';return}
  TUNNELS=tRes.data||[];CLIENT_STATS=cRes.ok&&cRes.data?cRes.data:{};renderTunnels();updateGlobalStatus()
}
function filteredTunnels(){
  const q=(document.getElementById('tunnelSearch')?.value||'').trim().toLowerCase(),filter=document.getElementById('tunnelFilter')?.value||'all'
  return TUNNELS.filter(t=>{
    const c=CLIENT_STATS[t.id]||{},state=tunnelState(t),services=t.services||[]
    const hay=[t.name,t.id,c.hostname,c.os,c.arch,...services.flatMap(s=>[s.name,s.host,s.target])].join(' ').toLowerCase()
    const qok=!q||hay.includes(q)
    const fok=filter==='all'||filter===state||(filter==='updates'&&versionMismatch(c))
    return qok&&fok
  })
}
function renderTunnels(){
  const list=filteredTunnels(),totalPages=Math.max(1,Math.ceil(list.length/pageSize));if(currentPage>totalPages)currentPage=totalPages
  const start=(currentPage-1)*pageSize,visible=list.slice(start,start+pageSize),el=document.getElementById('tunnels')
  el.innerHTML=visible.length?visible.map(renderTunnelGroup).join(''):'<div class="empty-state"><strong>No tunnels found</strong><span>Try another search or filter.</span></div>'
  const online=TUNNELS.filter(t=>t.online).length,disabled=TUNNELS.filter(t=>!t.enabled).length,offline=TUNNELS.length-online-disabled
  document.getElementById('tableSummary').textContent=`${TUNNELS.length} tunnel${TUNNELS.length===1?'':'s'}  •  ${online} online  •  ${Math.max(0,offline)} offline  •  ${disabled} disabled`
  document.getElementById('pageCurrent').textContent=currentPage;document.getElementById('prevPage').disabled=currentPage<=1;document.getElementById('nextPage').disabled=currentPage>=totalPages
}
function changePage(delta){const pages=Math.max(1,Math.ceil(filteredTunnels().length/pageSize));currentPage=Math.min(pages,Math.max(1,currentPage+delta));renderTunnels()}
function changePageSize(){pageSize=Number(document.getElementById('rowsPerPage').value)||10;currentPage=1;renderTunnels()}
function toggleTunnel(id){OPEN_TUNNELS.has(id)?OPEN_TUNNELS.delete(id):OPEN_TUNNELS.add(id);renderTunnels()}
function toggleActionMenu(id,event){event.stopPropagation();OPEN_MENU=OPEN_MENU===id?null:id;renderTunnels()}
function closeMenus(){if(OPEN_MENU){OPEN_MENU=null;renderTunnels()}}

function renderTunnelGroup(t){
  const c=CLIENT_STATS[t.id]||{},services=t.services||[],state=tunnelState(t),open=OPEN_TUNNELS.has(t.id),traffic=(c.bytes_to_client||0)+(c.bytes_from_client||0),last=c.last_activity?(formatAge(c.last_activity)+' ago'):'—',uptime=t.online&&c.connected_at?formatAge(c.connected_at):'—',version=c.version||'—',update=versionMismatch(c),menuId='t-'+t.id
  const status=`<span class="status-text ${state==='online'?'':'offline'}"><span class="status-dot ${state==='online'?'ok':''}"></span>${state[0].toUpperCase()+state.slice(1)}</span>`
  return `<div class="tunnel-group ${open?'open':''}">
    <div class="tunnel-row">
      <div class="col-expand"><button class="expand-btn" onclick="event.stopPropagation();toggleTunnel('${t.id}')">›</button></div>
      <div class="name-cell" onclick="toggleTunnel('${t.id}')"><div class="device-icon">▰</div><div class="name-stack"><strong>${esc(t.name)}</strong><span>${esc(platform(c))}</span></div></div>
      <div class="cell">${status}</div><div class="cell">${services.length}</div><div class="cell">${uptime}</div><div class="cell ${update?'version-old':'muted'}">${esc(version)}</div><div class="cell muted">${last}</div>
      <div class="cell"><div class="traffic-main">${formatBytes(traffic)}</div><div class="traffic-sub"><span>↓ ${formatBytes(c.bytes_from_client||0)}</span><span>↑ ${formatBytes(c.bytes_to_client||0)}</span></div></div>
      <div class="cell col-actions"><div class="manage-wrap"><button class="btn secondary compact manage-btn" onclick="toggleActionMenu('${menuId}',event)">Manage <span>⌄</span></button>${renderTunnelMenu(t,menuId,update)}</div></div>
    </div>
    <div class="tunnel-detail">
      <div class="detail-top"><span>Client <strong>${esc(c.hostname||'Not reported')}</strong></span><span>Remote <strong class="mono">${esc(c.remote_addr||'—')}</strong></span><span>Requests <strong>${Number(c.requests||0).toLocaleString()}</strong></span><span>Reconnects <strong>${Number(c.reconnect_count||0)}</strong></span><span>Token <strong class="mono">${esc(t.token_hint||'—')}</strong></span>${update?`<span class="update-warning">Update available: ${esc(STATUS.version)}</span>`:''}</div>
      ${renderServices(t)}
    </div>
  </div>`
}
function renderTunnelMenu(t,id,update){return `<div class="menu ${OPEN_MENU===id?'open':''}" onclick="event.stopPropagation()"><button onclick="OPEN_MENU=null;openSvc('${t.id}')">＋ Register service</button>${update?`<button onclick="OPEN_MENU=null;copyDockerUpdate()">⇧ Copy Docker update command</button>`:''}<button onclick="OPEN_MENU=null;askRegenerate('${t.id}')">↻ Regenerate credential</button><button onclick="OPEN_MENU=null;tunnelAction('${t.id}','toggle')">${t.enabled?'Disable tunnel':'Enable tunnel'}</button><div class="menu-sep"></div><button class="danger-item" onclick="OPEN_MENU=null;askDelete('${t.id}')">Delete tunnel</button></div>`}
function renderServices(t){
  const services=t.services||[]
  if(!services.length)return `<div class="services-box"><div class="services-title">Published services (0)</div><div class="empty-state"><strong>No services published</strong><span>Use Manage → Register service to add one.</span></div></div>`
  return `<div class="services-box"><div class="services-title">Published services (${services.length})</div>${services.map(s=>renderServiceRow(t.id,s)).join('')}</div>`
}
function renderServiceRow(tid,s){
  const installed=!!s.installed_route,menuId='s-'+tid+'-'+s.id,scheme=(String(s.target||'').split(':')[0]||'http').toUpperCase(),target=String(s.target||'').replace(/^https?:\/\//i,'')
  return `<div class="service-row"><div class="service-main"><div class="service-state"><span class="status-dot ${installed&&s.enabled?'ok':''}"></span></div><div class="service-copy"><strong>${esc(s.name||s.host)}</strong><span>${esc(s.host)}${s.path?esc(s.path):''}</span></div></div><div class="service-target"><div class="route-line"><span>${esc(scheme)} → </span>${esc(target)}</div><div class="service-flags">${s.skip_tls_verify?'<span class="warning-chip">⚠ TLS verification off</span>':''}${!installed?'<span class="warning-chip">Route not installed</span>':!s.enabled?'<span class="warning-chip">Disabled</span>':'<span class="ok-chip">Route installed</span>'}</div></div><div class="service-actions">${installed?`<button class="btn secondary compact" onclick="openService('${tid}','${s.id}')">↗ Open</button>`:''}<div class="manage-wrap"><button class="icon-btn" onclick="toggleActionMenu('${menuId}',event)">•••</button>${renderServiceMenu(tid,s,menuId)}</div></div></div>`
}
function renderServiceMenu(tid,s,id){const installed=!!s.installed_route;return `<div class="menu ${OPEN_MENU===id?'open':''}" onclick="event.stopPropagation()"><button onclick="OPEN_MENU=null;openSvc('${tid}','${s.id}')">Edit service</button>${installed?`<button onclick="OPEN_MENU=null;svcAction('${tid}','${s.id}','uninstall')">Uninstall route</button>`:`<button onclick="OPEN_MENU=null;askInstall('${tid}','${s.id}')">Install route</button>`}<button onclick="OPEN_MENU=null;svcAction('${tid}','${s.id}','toggle')">${s.enabled?'Disable service':'Enable service'}</button><div class="menu-sep"></div><button class="danger-item" onclick="OPEN_MENU=null;askDeleteService('${tid}','${s.id}')">Delete service</button></div>`}
function openService(tid,sid){const t=TUNNELS.find(x=>x.id===tid),s=t&&(t.services||[]).find(x=>x.id===sid);if(!s)return;openExternal('https://'+s.host+(s.path||''))}
function copyDockerUpdate(){copyText('docker compose pull && docker compose up -d')}

async function createTunnel(){
  const ep=endpointFromStatus();if(!ep.ok){closeModal('createTunnelModal');showTab('settings');toast('Configure the Control Node endpoint first');return}
  const name=document.getElementById('newTunnel').value.trim();if(!name){toast('Enter a tunnel name');return}
  const {ok,data}=await api('tunnels',{method:'POST',body:JSON.stringify({name})});if(!ok||!data){toast(data?.error||'Could not create tunnel');return}
  closeModal('createTunnelModal');if(data.tunnel?.id)OPEN_TUNNELS.add(data.tunnel.id);showCommands(data.token,true);await Promise.all([loadStatus(),loadTunnels()])
}
async function tunnelAction(id,action){const {ok,data}=await api('tunnels/action',{method:'POST',body:JSON.stringify({id,action})});if(!ok){toast(data?.error||'Action failed');return}if(action==='regenerate'&&data?.token)showCommands(data.token,true);await Promise.all([loadStatus(),loadTunnels()])}
function askRegenerate(id){confirmDialog('Regenerate credential','Generate a new tunnel credential? The currently configured client will stop connecting until its token is updated.',()=>tunnelAction(id,'regenerate'))}
function askDelete(id){const t=TUNNELS.find(x=>x.id===id);confirmDialog('Delete tunnel',`Delete ${t?.name||id}? Installed routes are removed automatically.`,async()=>{await api('tunnels/action',{method:'POST',body:JSON.stringify({id,action:'delete'})});OPEN_TUNNELS.delete(id);toast('Tunnel deleted');await Promise.all([loadStatus(),loadTunnels()])})}

function openSvc(tid,sid=null){
  svcTunnelId=tid;svcServiceId=sid;const t=TUNNELS.find(x=>x.id===tid),s=sid&&t?(t.services||[]).find(x=>x.id===sid):null
  document.getElementById('svcName').value=s?.name||'';document.getElementById('svcHost').value=s?.host||'';document.getElementById('svcPath').value=s?.path||'';document.getElementById('svcTarget').value=s?.target||'';document.getElementById('svcTag').value=s?s.tag||'':STATUS.default_tag||'';document.getElementById('svcSkipTLS').checked=!!s?.skip_tls_verify
  document.getElementById('svcModalTitle').textContent=s?'Edit service':'Register service';document.getElementById('svcSubmitBtn').textContent=s?'Save changes':'Register service';document.getElementById('svcModal').classList.add('show')
}
function closeSvc(){closeModal('svcModal');svcTunnelId=null;svcServiceId=null}
async function submitService(){
  const editing=!!svcServiceId,body={tunnel_id:svcTunnelId,service_id:svcServiceId||'',action:editing?'edit':'add',name:document.getElementById('svcName').value.trim(),host:document.getElementById('svcHost').value.trim(),path:document.getElementById('svcPath').value.trim(),target:document.getElementById('svcTarget').value.trim(),tag:document.getElementById('svcTag').value.trim(),skip_tls_verify:document.getElementById('svcSkipTLS').checked}
  if(!body.host||!body.target){toast('Public host and target are required');return}
  const {ok,data}=await api('services/action',{method:'POST',body:JSON.stringify(body)});if(!ok){toast(data?.error||'Could not save service');return}
  OPEN_TUNNELS.add(svcTunnelId);closeSvc();toast(editing?'Service updated':'Service registered');loadTunnels()
}
async function svcAction(tid,sid,action){const {ok,data}=await api('services/action',{method:'POST',body:JSON.stringify({tunnel_id:tid,service_id:sid,action})});toast(ok?'Done':data?.error||'Action failed');OPEN_TUNNELS.add(tid);loadTunnels()}
function askDeleteService(tid,sid){const t=TUNNELS.find(x=>x.id===tid),s=t&&(t.services||[]).find(x=>x.id===sid);confirmDialog('Delete service',`Delete ${s?.name||s?.host||sid}?`,async()=>{await api('services/action',{method:'POST',body:JSON.stringify({tunnel_id:tid,service_id:sid,action:'delete'})});toast('Service deleted');loadTunnels()})}
function askInstall(tid,sid){const t=TUNNELS.find(x=>x.id===tid),s=t&&(t.services||[]).find(x=>x.id===sid);installTunnelId=tid;installServiceId=sid;document.getElementById('installMsg').textContent=`Install a Zoraxy route for ${s?.host||''}?`;document.getElementById('installModal').classList.add('show')}
function closeInstall(){closeModal('installModal');installTunnelId=null;installServiceId=null}
async function doInstall(issue_cert){const tid=installTunnelId,sid=installServiceId;closeInstall();if(!tid)return;if(issue_cert)toast('Requesting certificate…');const {ok,data}=await api('services/action',{method:'POST',body:JSON.stringify({tunnel_id:tid,service_id:sid,action:'install',issue_cert})});toast(ok?(issue_cert?'Route and certificate installed':'Route installed'):data?.error||'Install failed');OPEN_TUNNELS.add(tid);loadTunnels()}

function confirmDialog(title,msg,cb){document.getElementById('confirmTitle').textContent=title;document.getElementById('confirmMsg').textContent=msg;confirmCb=cb;document.getElementById('confirmModal').classList.add('show')}
function closeConfirm(){closeModal('confirmModal');confirmCb=null}
function runConfirm(){const cb=confirmCb;closeConfirm();if(cb)cb()}

function showCommands(token,fresh){
  const ep=endpointFromStatus();if(!ep.ok){showTab('settings');toast('Configure the Control Node endpoint first');return}
  const host=ep.value,fp=STATUS.fingerprint||'',tok=token||'<TOKEN>'
  document.getElementById('cmd-cli').textContent=`tunnel-client \\
  --server ${host} \\
  --token ${tok} \\
  --fingerprint "${fp}"`
  document.getElementById('cmd-docker').textContent=`docker run -d --name tunnel-client --restart unless-stopped --pull always \\
  --network host \\
  ${IMAGE}:latest \\
  --server ${host} --token ${tok} --fingerprint "${fp}"`
  document.getElementById('cmd-compose').textContent=`services:\n  tunnel-client:\n    image: ${IMAGE}:latest\n    pull_policy: always\n    container_name: tunnel-client\n    restart: unless-stopped\n    network_mode: host\n    command:\n      - --server=${host}\n      - --token=${tok}\n      - --fingerprint=${fp}`
  document.getElementById('tokenNote').innerHTML=fresh?'This credential is shown <b>once</b>. Store it in your Compose file. Normal updates use <b>:latest</b> and do not change the token.':'Regenerate the credential only when you intentionally want to revoke the old one.'
  switchTab('compose');document.getElementById('cmdModal').classList.add('show')
}
function switchTab(tab){['cli','docker','compose'].forEach(t=>{document.getElementById('tab-'+t).style.display=t===tab?'block':'none';document.querySelector(`.modal-tabs [data-tab="${t}"]`).classList.toggle('active',t===tab)})}
function openDownloads(){const base=releaseBase(),items=[['Linux x64','tunnel-client_linux_amd64'],['Linux ARM64','tunnel-client_linux_arm64'],['Windows x64','tunnel-client_windows_amd64.exe'],['Windows ARM64','tunnel-client_windows_arm64.exe'],['macOS Apple Silicon','tunnel-client_darwin_arm64'],['macOS Intel','tunnel-client_darwin_amd64']];document.getElementById('downloadLinks').innerHTML=items.map(([label,file])=>`<button class="btn secondary" style="justify-content:space-between" onclick="openExternal('${base}${file}')"><span>${label}</span><span class="mono" style="font-size:10px;opacity:.65">${file}</span></button>`).join('')+`<button class="btn secondary" onclick="openExternal('${releasePage()}')">View all release assets</button>`;document.getElementById('downloadModal').classList.add('show')}

document.addEventListener('click',e=>{if(!e.target.closest('.manage-wrap'))closeMenus()})
document.addEventListener('keydown',e=>{if(e.key==='Escape'){document.querySelectorAll('.overlay.show').forEach(x=>x.classList.remove('show'));OPEN_MENU=null}})
loadStatus().then(loadTunnels)
setInterval(()=>{if(!anyModalOpen()&&!inputFocused()){loadStatus().then(loadTunnels)}},15000)
