let CURRENT_UPDATE_MODE='manual';
let ADD_CONNECTOR_TUNNEL='';

(function installConnectorEnrollmentUI(){
  const modal=document.createElement('div');
  modal.className='overlay';
  modal.id='addConnectorModal';
  modal.innerHTML=`<div class="modal connector-enroll-modal"><div class="modal-head"><div><h2>Add connector</h2><p>Add another host to this tunnel without changing any existing connector credential.</p></div><button class="close" onclick="closeModal('addConnectorModal')">×</button></div><div class="connector-enroll-body"><div class="field"><label>Connector ID</label><input class="input mono" id="addConnectorId" placeholder="e.g. homelab-backup"><small>Use a unique, stable ID for this host.</small></div><div class="enroll-section-title">Update method</div><label class="update-choice selected" id="updateChoiceAuto"><input type="radio" name="connectorUpdateMode" value="auto" checked onchange="selectConnectorUpdateMode('auto')"><span class="update-choice-icon">↻</span><span><strong>Automatic updates</strong><small>Recommended for Docker. Includes a dedicated updater service that watches the <code>:latest</code> image and recreates only this tunnel client when its digest changes.</small><em>Requires access to <code>/var/run/docker.sock</code>.</em></span></label><label class="update-choice" id="updateChoiceManual"><input type="radio" name="connectorUpdateMode" value="manual" onchange="selectConnectorUpdateMode('manual')"><span class="update-choice-icon">⌁</span><span><strong>Manual updates</strong><small>No Docker socket access. The Compose file stays unchanged; update when you choose with <code>docker compose pull && docker compose up -d</code>.</small></span></label><div class="docker-socket-warning"><strong>Docker socket access is powerful.</strong><span>Automatic mode mounts the socket only into the updater sidecar, not into the tunnel client itself. Any container with Docker socket access can effectively control the Docker host.</span></div></div><div class="modal-actions"><button class="btn secondary" onclick="closeModal('addConnectorModal')">Cancel</button><button class="btn primary" onclick="submitAddConnector()">Create connector credential</button></div></div>`;
  document.body.appendChild(modal);
})();

function selectConnectorUpdateMode(mode){
  CURRENT_UPDATE_MODE=mode==='auto'?'auto':'manual';
  document.getElementById('updateChoiceAuto')?.classList.toggle('selected',CURRENT_UPDATE_MODE==='auto');
  document.getElementById('updateChoiceManual')?.classList.toggle('selected',CURRENT_UPDATE_MODE==='manual');
  document.querySelector('.docker-socket-warning')?.classList.toggle('hidden',CURRENT_UPDATE_MODE!=='auto');
}

function nextConnectorID(t){
  const c=CLIENT_STATS[t.id]||{},used=new Set(connectorList(c).map(x=>String(x.id||'').toLowerCase()));
  let base=String(t.name||'connector').trim().toLowerCase().replace(/[^a-z0-9._-]+/g,'-').replace(/^-+|-+$/g,'')||'connector';
  let n=Math.max(1,used.size+1),id='';
  do{id=`${base}-${n++}`}while(used.has(id.toLowerCase()));
  return id;
}

function openAddConnector(tid){
  const t=TUNNELS.find(x=>x.id===tid);if(!t)return;
  ADD_CONNECTOR_TUNNEL=tid;
  CURRENT_UPDATE_MODE='auto';
  const input=document.getElementById('addConnectorId');if(input)input.value=nextConnectorID(t);
  const auto=document.querySelector('input[name="connectorUpdateMode"][value="auto"]');if(auto)auto.checked=true;
  const manual=document.querySelector('input[name="connectorUpdateMode"][value="manual"]');if(manual)manual.checked=false;
  selectConnectorUpdateMode('auto');
  document.getElementById('addConnectorModal').classList.add('show');
  setTimeout(()=>input?.focus(),50);
}

async function submitAddConnector(){
  const t=TUNNELS.find(x=>x.id===ADD_CONNECTOR_TUNNEL);if(!t)return;
  const connector_id=(document.getElementById('addConnectorId')?.value||'').trim();
  if(!connector_id){toast('Enter a connector ID');return}
  if(!/^[A-Za-z0-9._-]{1,128}$/.test(connector_id)){toast('Connector ID may only contain letters, numbers, dot, underscore and dash');return}
  const {ok,data}=await api('connectors/add',{method:'POST',body:JSON.stringify({tunnel_id:t.id,connector_id})});
  if(!ok){toast(data?.error||'Could not create connector credential');return}
  closeModal('addConnectorModal');
  CURRENT_UPDATE_MODE=document.querySelector('input[name="connectorUpdateMode"]:checked')?.value==='auto'?'auto':'manual';
  showCommands(data.token,true,data.tunnel_name||t.name);
  const field=document.getElementById('connectorId');if(field)field.value=connector_id;
  refreshCommands();
  configureCommandTabs();
  document.getElementById('tokenNote').innerHTML=`New connector credential created for <b>${esc(connector_id)}</b>. Existing connectors keep working. ${CURRENT_UPDATE_MODE==='auto'?'<br>Automatic updates use a Docker-socket updater sidecar; the tunnel client itself does not receive the socket.':'<br>This connector uses manual updates and does not require Docker socket access.'}`;
  OPEN_TUNNELS.add(t.id);loadTunnels();
}

function configureCommandTabs(){
  const cli=document.querySelector('.modal-tabs [data-tab="cli"]'),docker=document.querySelector('.modal-tabs [data-tab="docker"]'),compose=document.querySelector('.modal-tabs [data-tab="compose"]');
  if(cli)cli.style.display=CURRENT_UPDATE_MODE==='auto'?'none':'';
  if(docker)docker.style.display=CURRENT_UPDATE_MODE==='auto'?'none':'';
  if(compose)compose.style.display='';
  switchTab('compose');
}

const _enrollBaseRefreshCommands=refreshCommands;
refreshCommands=function(){
  if(CURRENT_UPDATE_MODE!=='auto')return _enrollBaseRefreshCommands();
  const ep=endpointFromStatus();if(!ep.ok)return;
  const host=ep.value,fp=STATUS.fingerprint||'',tok=CURRENT_CMD_TOKEN||'<TOKEN>',raw=document.getElementById('connectorId')?.value||defaultConnectorID(CURRENT_CMD_TUNNEL),cid=raw.trim()||'connector-1';
  const safeName=('zoraxy-tunnel-'+cid.toLowerCase().replace(/[^a-z0-9_.-]+/g,'-')).slice(0,63);
  document.getElementById('cmd-cli').textContent='Automatic update mode is available for Docker Compose. Choose Manual updates to use the standalone CLI client.';
  document.getElementById('cmd-docker').textContent='Automatic update mode is available for Docker Compose so the updater sidecar can be installed together with the client.';
  document.getElementById('cmd-compose').textContent=`services:\n  tunnel-client:\n    image: ${IMAGE}:latest\n    pull_policy: always\n    container_name: ${safeName}\n    restart: unless-stopped\n    network_mode: host\n    command:\n      - --server=${host}\n      - --token=${tok}\n      - --fingerprint=${fp}\n      - --connector-id=${cid}\n    labels:\n      - wud.watch=true\n      - wud.watch.digest=true\n      - wud.trigger.docker.zoraxy.enabled=true\n\n  tunnel-client-updater:\n    image: getwud/wud:latest\n    container_name: ${safeName}-updater\n    restart: unless-stopped\n    volumes:\n      - /var/run/docker.sock:/var/run/docker.sock\n    environment:\n      - WUD_WATCHER_LOCAL_WATCHBYDEFAULT=false\n      - WUD_TRIGGER_DOCKER_ZORAXY_PRUNE=true`;
};

const _enrollBaseOpenCreateTunnel=openCreateTunnel;
openCreateTunnel=function(){CURRENT_UPDATE_MODE='manual';return _enrollBaseOpenCreateTunnel()};

const _enrollBaseRenderConnectors=renderConnectors;
renderConnectors=function(t,c){
  let html=_enrollBaseRenderConnectors(t,c),button=`<button class="btn primary compact add-connector-btn" onclick="openAddConnector('${t.id}')">＋ Add connector</button>`;
  if(html.includes('<div class="connector-spacer"></div>'))return html.replace('<div class="connector-spacer"></div>',`<div class="connector-spacer"></div>${button}`);
  return html.replace('</span></div></div>',`</span><div class="connector-spacer"></div>${button}</div></div>`);
};

const _enrollBaseRenderTunnelMenu=renderTunnelMenu;
renderTunnelMenu=function(t,id,update){
  let html=_enrollBaseRenderTunnelMenu(t,id,update);
  return html.replace('<button onclick="OPEN_MENU=null;openSvc',`<button onclick="OPEN_MENU=null;openAddConnector('${t.id}')">＋ Add connector</button><button onclick="OPEN_MENU=null;openSvc`);
};
