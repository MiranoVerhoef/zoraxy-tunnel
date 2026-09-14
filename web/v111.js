let FIRST_UPDATE_MODE='auto';
const CONNECTOR_UPDATE_BUSY=new Set();
const WUD_ADMIN_USER='zoraxy-tunnel';
let CURRENT_WUD_PASSWORD='';

function resetWudCredential(){CURRENT_WUD_PASSWORD=''}
function generateWudPassword(){
  const alphabet='ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz23456789';
  const bytes=new Uint8Array(32);
  if(window.crypto&&window.crypto.getRandomValues)window.crypto.getRandomValues(bytes);
  else for(let i=0;i<bytes.length;i++)bytes[i]=Math.floor(Math.random()*256);
  let out='';for(const b of bytes)out+=alphabet[b%alphabet.length];return out;
}
function ensureWudPassword(){if(!CURRENT_WUD_PASSWORD)CURRENT_WUD_PASSWORD=generateWudPassword();return CURRENT_WUD_PASSWORD}

(function installFirstTunnelUpdateChoice(){
  const modal=document.querySelector('#createTunnelModal .modal');
  const field=modal?.querySelector('.field');
  if(!modal||!field||document.getElementById('firstUpdateOptions'))return;
  const block=document.createElement('div');
  block.id='firstUpdateOptions';
  block.className='first-update-options';
  block.innerHTML=`<div class="enroll-section-title">Update method</div>
    <label class="update-choice selected" id="firstUpdateChoiceAuto"><input type="radio" name="firstTunnelUpdateMode" value="auto" checked><span class="update-choice-icon">↻</span><span><strong>Automatic updates</strong><small>Recommended for Docker. The connector watches <code>:latest</code> and updates automatically when a new image is published.</small><em>Requires access to <code>/var/run/docker.sock</code> through the updater sidecar.</em></span></label>
    <label class="update-choice" id="firstUpdateChoiceManual"><input type="radio" name="firstTunnelUpdateMode" value="manual"><span class="update-choice-icon">⌁</span><span><strong>Manual updates</strong><small>No Docker socket access. Update whenever you choose with <code>docker compose pull && docker compose up -d</code>.</small></span></label>`;
  field.after(block);
  block.querySelectorAll('input[name="firstTunnelUpdateMode"]').forEach(r=>r.addEventListener('change',()=>selectFirstUpdateMode(r.value)));
})();

function selectFirstUpdateMode(mode){
  FIRST_UPDATE_MODE=mode==='auto'?'auto':'manual';
  document.getElementById('firstUpdateChoiceAuto')?.classList.toggle('selected',FIRST_UPDATE_MODE==='auto');
  document.getElementById('firstUpdateChoiceManual')?.classList.toggle('selected',FIRST_UPDATE_MODE==='manual');
  const auto=document.querySelector('input[name="firstTunnelUpdateMode"][value="auto"]');
  const manual=document.querySelector('input[name="firstTunnelUpdateMode"][value="manual"]');
  if(auto)auto.checked=FIRST_UPDATE_MODE==='auto';
  if(manual)manual.checked=FIRST_UPDATE_MODE==='manual';
}

const _v111OpenCreateTunnel=openCreateTunnel;
openCreateTunnel=function(){
  resetWudCredential();
  const result=_v111OpenCreateTunnel();
  selectFirstUpdateMode('auto');
  CURRENT_UPDATE_MODE='auto';
  return result;
};

const _v112OpenAddConnector=openAddConnector;
openAddConnector=function(tid){resetWudCredential();return _v112OpenAddConnector(tid)};

const _v111CreateTunnel=createTunnel;
createTunnel=async function(){
  CURRENT_UPDATE_MODE=FIRST_UPDATE_MODE;
  const result=await _v111CreateTunnel();
  configureCommandTabs();
  refreshCommands();
  return result;
};

function v111SafeContainerName(cid){
  return ('zoraxy-tunnel-'+String(cid||'connector').toLowerCase().replace(/[^a-z0-9_.-]+/g,'-')).slice(0,63);
}

refreshCommands=function(){
  const ep=endpointFromStatus();if(!ep.ok)return;
  const host=ep.value,fp=STATUS.fingerprint||'',tok=CURRENT_CMD_TOKEN||'<TOKEN>',raw=document.getElementById('connectorId')?.value||defaultConnectorID(CURRENT_CMD_TUNNEL),cid=raw.trim()||'connector-1',safeName=v111SafeContainerName(cid);
  if(CURRENT_UPDATE_MODE==='auto'){
    const wudPassword=ensureWudPassword();
    document.getElementById('cmd-cli').textContent='Automatic update mode is available for Docker Compose. Choose Manual updates to use the standalone CLI client.';
    document.getElementById('cmd-docker').textContent='Automatic update mode is available for Docker Compose so the updater sidecar can be installed together with the client.';
    document.getElementById('cmd-compose').textContent=`services:\n  tunnel-client:\n    image: ${IMAGE}:latest\n    pull_policy: always\n    container_name: ${safeName}\n    restart: unless-stopped\n    network_mode: host\n    environment:\n      ZORAXY_TUNNEL_UPDATE_MODE: auto\n      ZORAXY_TUNNEL_UPDATER_USER: ${WUD_ADMIN_USER}\n      ZORAXY_TUNNEL_UPDATER_PASSWORD: ${wudPassword}\n    command:\n      - --server=${host}\n      - --token=${tok}\n      - --fingerprint=${fp}\n      - --connector-id=${cid}\n    labels:\n      - wud.watch=true\n      - wud.watch.digest=true\n      - wud.trigger.include=docker.zoraxy\n\n  tunnel-client-updater:\n    image: getwud/wud:latest\n    container_name: ${safeName}-updater\n    restart: unless-stopped\n    ports:\n      - 127.0.0.1:3009:3000\n    volumes:\n      - /var/run/docker.sock:/var/run/docker.sock\n    environment:\n      WUD_AUTH_ADMIN_USER: ${WUD_ADMIN_USER}\n      WUD_AUTH_ADMIN_PASSWORD: ${wudPassword}\n      WUD_WATCHER_LOCAL_WATCHBYDEFAULT: \"false\"\n      WUD_TRIGGER_DOCKER_ZORAXY_AUTO: \"true\"\n      WUD_TRIGGER_DOCKER_ZORAXY_PRUNE: \"true\"\n      WUD_TRIGGER_DOCKER_ZORAXY_THRESHOLD: all`;
    return;
  }
  document.getElementById('cmd-cli').textContent=`tunnel-client \\\n  --server ${host} \\\n  --token ${tok} \\\n  --fingerprint \"${fp}\" \\\n  --connector-id ${cid}`;
  document.getElementById('cmd-docker').textContent=`docker run -d --name ${safeName} --restart unless-stopped --pull always \\\n  --network host \\\n  -e ZORAXY_TUNNEL_UPDATE_MODE=manual \\\n  ${IMAGE}:latest \\\n  --server ${host} --token ${tok} --fingerprint \"${fp}\" --connector-id ${cid}`;
  document.getElementById('cmd-compose').textContent=`services:\n  tunnel-client:\n    image: ${IMAGE}:latest\n    pull_policy: always\n    container_name: ${safeName}\n    restart: unless-stopped\n    network_mode: host\n    environment:\n      ZORAXY_TUNNEL_UPDATE_MODE: manual\n    command:\n      - --server=${host}\n      - --token=${tok}\n      - --fingerprint=${fp}\n      - --connector-id=${cid}`;
};

async function requestConnectorUpdate(tunnelID,encodedConnectorID){
  const connectorID=decodeURIComponent(encodedConnectorID||'');
  const key=tunnelID+'|'+connectorID;
  if(CONNECTOR_UPDATE_BUSY.has(key))return;
  CONNECTOR_UPDATE_BUSY.add(key);renderTunnels();
  const {ok,data}=await api('connectors/update',{method:'POST',body:JSON.stringify({tunnel_id:tunnelID,connector_id:connectorID})});
  if(ok){toast(data?.message||'Update check requested');setTimeout(()=>loadTunnels(),2200);setTimeout(()=>loadTunnels(),6000)}
  else toast(data?.error||data||'Could not request connector update');
  setTimeout(()=>{CONNECTOR_UPDATE_BUSY.delete(key);renderTunnels()},6500);
}

renderConnectors=function(t,c){
  const list=connectorList(c),preferred=c.preferred_connector_id||'';
  const addButton=`<button class="btn primary compact add-connector-btn" onclick="openAddConnector('${t.id}')">＋ Add connector</button>`;
  if(!list.length)return `<div class="connector-section"><div class="connector-head"><strong>Connectors</strong><span>No connector telemetry yet. Connect a client to see its status here.</span><div class="connector-spacer"></div>${addButton}</div></div>`;
  const rows=list.map(cn=>{
    const uptime=cn.online&&cn.connected_at?formatAge(cn.connected_at):'—',traffic=(cn.bytes_to_client||0)+(cn.bytes_from_client||0),plat=[cn.os,cn.arch].filter(Boolean).join(' / ')||'unknown',last=cn.last_activity?formatAge(cn.last_activity)+' ago':'—',mode=cn.update_mode==='auto'?'auto':'manual',key=t.id+'|'+cn.id,busy=CONNECTOR_UPDATE_BUSY.has(key),controlReady=!!cn.updater_control;
    const modeBadge=mode==='auto'?'<span class="connector-badge auto-update">Auto update</span>':'<span class="connector-badge">Manual update</span>';
    const controlBadge=mode==='auto'&&cn.online&&!controlReady?'<span class="warning-chip" title="Redeploy using the v1.12 automatic Compose configuration">Update control unavailable</span>':'';
    const updateButton=mode==='auto'&&cn.online&&controlReady?`<button class="btn secondary compact update-now ${busy?'updating':''}" onclick="requestConnectorUpdate('${t.id}','${encodeURIComponent(cn.id)}')">${busy?'Checking…':'↻ Update now'}</button>`:mode==='auto'&&cn.online?'<button class="btn secondary compact" disabled title="Redeploy this connector using the v1.12 automatic Compose configuration">↻ Update now</button>':'';
    const preferredButton=cn.preferred?'<button class="btn secondary compact" disabled>Preferred</button>':`<button class="btn secondary compact" onclick="setPreferredConnector('${t.id}','${encodeURIComponent(cn.id)}')">Make preferred</button>`;
    return `<div class="connector-row"><div class="connector-identity"><strong><span class="status-dot ${cn.online?'ok':''}"></span>${esc(cn.hostname||cn.id)}</strong><span class="mono">${esc(cn.id)}</span><div class="connector-badges">${connectorBadges(cn)}${modeBadge}${controlBadge}</div></div><div class="connector-cell connector-platform">${esc(plat)}<small>${esc(cn.version||'version unknown')}</small></div><div class="connector-cell connector-uptime">${uptime}<small>${last}</small></div><div class="connector-cell connector-remote mono">${esc(cn.remote_addr||'—')}<small>${Number(cn.reconnect_count||0)} reconnects</small></div><div class="connector-cell connector-traffic">${formatBytes(traffic)}<small>↓ ${formatBytes(cn.bytes_from_client||0)} · ↑ ${formatBytes(cn.bytes_to_client||0)}</small></div><div class="connector-action">${updateButton}${preferredButton}</div></div>`;
  }).join('');
  return `<div class="connector-section"><div class="connector-head"><strong>Connectors (${c.online_connector_count||0}/${list.length} online)</strong><span>${preferred?'Preferred connector automatically takes traffic again when it reconnects.':'Automatic sticky failover is active; the current primary remains selected while healthy.'}</span><div class="connector-spacer"></div>${addButton}${preferred?`<button class="btn secondary compact" onclick="setPreferredConnector('${t.id}','')">Clear preference</button>`:''}</div>${rows}</div>`;
};

let V111_REFRESH_BUSY=false;
async function refreshLiveConnections(){
  if(document.hidden||V111_REFRESH_BUSY)return;
  V111_REFRESH_BUSY=true;
  try{await loadTunnels()}finally{V111_REFRESH_BUSY=false}
}
setInterval(refreshLiveConnections,5000);
window.addEventListener('focus',refreshLiveConnections);
document.addEventListener('visibilitychange',()=>{if(!document.hidden)refreshLiveConnections()});
