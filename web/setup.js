const ENHANCED_REPO='https://github.com/MiranoVerhoef/zoraxy-tunnel-enhanced';

// Keep all documentation and release links on the renamed repository while the
// Docker image name stays stable for backwards-compatible Compose files.
openDocs=function(){openExternal(ENHANCED_REPO+'#readme')};
releaseBase=function(){return ENHANCED_REPO+'/releases/download/'+(STATUS.version||'latest')+'/'};
releasePage=function(){return ENHANCED_REPO+'/releases/'+(STATUS.version?'tag/'+STATUS.version:'latest')};

const CONNECT_SETUP={
  tunnelId:'',tunnelName:'',token:'',connectorId:'',mode:'auto',connected:false,
  context:'normal',phase:'install',poll:null,commands:{},openedMethod:'',afterFinish:null
};
let FIRST_RUN_MODE='auto';
let FIRST_RUN_CONNECTOR_TOUCHED=false;

(function installGuidedSetup(){
  const connector=document.createElement('div');
  connector.className='overlay';
  connector.id='connectorSetupWizardModal';
  connector.innerHTML='<div class="modal setup-wizard-modal"><div id="connectorSetupWizard"></div></div>';
  document.body.appendChild(connector);

  const first=document.createElement('div');
  first.className='overlay';
  first.id='firstRunWizardModal';
  first.innerHTML='<div class="modal setup-wizard-modal"><div id="firstRunWizard"></div></div>';
  document.body.appendChild(first);

  const settingsHead=document.querySelector('.settings-head');
  if(settingsHead&&!document.getElementById('runSetupWizardBtn')){
    const b=document.createElement('button');
    b.id='runSetupWizardBtn';b.className='btn secondary compact setup-settings-button';b.textContent='Run setup wizard';b.onclick=()=>openFirstRunWizard(true);
    settingsHead.appendChild(b);
  }

  const setupBanner=document.getElementById('setupBanner');
  const configure=setupBanner?.querySelector('button');
  if(configure){configure.textContent='Guided setup';configure.onclick=()=>openFirstRunWizard(true)}
})();

function setupProgress(step,total=4){
  let html='';for(let i=1;i<=total;i++)html+=`<span class="${i<step?'done':i===step?'active':''}"></span>`;return `<div class="setup-progress">${html}</div>`;
}
function setupHeader(title,copy,closable=true){return `<div class="setup-wizard-head"><div><h2>${esc(title)}</h2><p>${esc(copy)}</p></div>${closable?'<button class="close" onclick="connectorSetupLater()">×</button>':''}</div>`}
function setupMethod(id,title,subtitle,command){
  const open=CONNECT_SETUP.openedMethod===id;
  return `<div class="setup-method ${open?'open':''}" id="setupMethod-${id}"><button class="setup-method-head" onclick="toggleSetupMethod('${id}')"><strong>${esc(title)}</strong><span>${esc(subtitle)} ${open?'⌃':'⌄'}</span></button><div class="setup-method-body"><pre>${esc(command||'')}</pre><div class="setup-method-actions"><button class="btn secondary compact" onclick="copySetupCommand('${id}')">Copy</button></div></div></div>`;
}
function toggleSetupMethod(id){CONNECT_SETUP.openedMethod=CONNECT_SETUP.openedMethod===id?'':id;renderConnectorSetup()}
function copySetupCommand(id){const cmd=CONNECT_SETUP.commands[id]||'';copyText(cmd)}

function prepareConnectorCommands(resetSecret=false){
  if(resetSecret&&typeof resetWudCredential==='function')resetWudCredential();
  CURRENT_UPDATE_MODE=CONNECT_SETUP.mode;
  CURRENT_CMD_TOKEN=CONNECT_SETUP.token||'<TOKEN>';
  CURRENT_CMD_TUNNEL=CONNECT_SETUP.tunnelName||'connector';
  const field=document.getElementById('connectorId');if(field)field.value=CONNECT_SETUP.connectorId;
  refreshCommands();
  CONNECT_SETUP.commands={
    cli:document.getElementById('cmd-cli')?.textContent||'',
    docker:document.getElementById('cmd-docker')?.textContent||'',
    compose:document.getElementById('cmd-compose')?.textContent||''
  };
}

function launchConnectorSetup(opts){
  stopConnectorSetupPolling();
  CONNECT_SETUP.tunnelId=opts.tunnelId||'';
  CONNECT_SETUP.tunnelName=opts.tunnelName||'Tunnel';
  CONNECT_SETUP.token=opts.token||'';
  CONNECT_SETUP.connectorId=opts.connectorId||defaultConnectorID(CONNECT_SETUP.tunnelName);
  CONNECT_SETUP.mode=opts.mode==='manual'?'manual':'auto';
  CONNECT_SETUP.connected=false;
  CONNECT_SETUP.context=opts.context||'normal';
  CONNECT_SETUP.phase='install';
  CONNECT_SETUP.openedMethod='';
  CONNECT_SETUP.afterFinish=typeof opts.afterFinish==='function'?opts.afterFinish:null;
  prepareConnectorCommands(true);
  document.getElementById('connectorSetupWizardModal').classList.add('show');
  renderConnectorSetup();
  startConnectorSetupPolling();
}

function renderConnectorSetup(){
  const root=document.getElementById('connectorSetupWizard');if(!root)return;
  if(CONNECT_SETUP.phase==='install'){
    const methods=CONNECT_SETUP.mode==='auto'
      ? setupMethod('compose','Docker Compose','Automatic updates',CONNECT_SETUP.commands.compose)
      : setupMethod('compose','Docker Compose','Recommended',CONNECT_SETUP.commands.compose)+setupMethod('docker','Docker run','Manual updates',CONNECT_SETUP.commands.docker)+setupMethod('cli','Standalone client','Manual updates',CONNECT_SETUP.commands.cli);
    root.innerHTML=`${setupHeader('Connect '+CONNECT_SETUP.connectorId,'Install this connector, then leave this window open while Zoraxy waits for it to come online.')}${setupProgress(2,3)}<div class="setup-body"><div class="setup-inline-form"><div class="field span-2"><label>Connector ID</label><input class="input mono" id="setupConnectorId" value="${esc(CONNECT_SETUP.connectorId)}" oninput="connectorSetupIdChanged(this.value)"><small>Keep this stable and unique for this host.</small></div></div><div class="setup-status-card" id="connectorWaitStatus"><div class="setup-status-icon">…</div><div class="setup-status-copy"><strong>Waiting for connection</strong><span>Zoraxy is checking every 2 seconds for ${esc(CONNECT_SETUP.connectorId)}.</span></div><button class="btn secondary compact" onclick="checkConnectorSetupConnection()">Check now</button></div><div class="setup-step-title">Installation</div><p class="setup-step-copy">Choose a method below. Commands stay collapsed until you open them.</p><div class="setup-methods">${methods}</div>${CONNECT_SETUP.mode==='auto'?'<div class="setup-note"><strong>Automatic updates:</strong> the generated Compose stack includes the authenticated updater sidecar and requires access to <code>/var/run/docker.sock</code>.</div>':''}</div><div class="setup-actions"><button class="btn secondary" onclick="connectorSetupLater()">Setup later</button><div class="setup-spacer"></div><button class="btn secondary" onclick="checkConnectorSetupConnection()">Check connection</button><button class="btn primary" id="connectorSetupContinue" onclick="connectorSetupContinue()" disabled>Continue</button></div>`;
    updateConnectorWaitUI();return;
  }
  if(CONNECT_SETUP.phase==='redundancy'){
    const stats=CLIENT_STATS[CONNECT_SETUP.tunnelId]||{},online=connectorList(stats).filter(x=>x.online).length,total=connectorList(stats).length;
    root.innerHTML=`${setupHeader('Connector online','The tunnel is connected and ready for traffic.')}${setupProgress(3,3)}<div class="setup-body"><div class="setup-finish"><div class="setup-finish-mark">✓</div><h3>${esc(CONNECT_SETUP.connectorId)} is connected</h3><p>${online} of ${Math.max(total,online)} connector${Math.max(total,online)===1?'':'s'} online for ${esc(CONNECT_SETUP.tunnelName)}.</p></div><div class="setup-step-title" style="margin-top:22px">Add redundancy?</div><p class="setup-step-copy">You can add another connector now for automatic failover, or finish and add one later.</p><div class="setup-choice-grid"><button class="setup-choice" onclick="showAdditionalConnectorForm()"><span class="setup-status-icon">＋</span><span><strong>Add another connector</strong><small>Create another independent credential without affecting this connector.</small></span></button><button class="setup-choice" onclick="finishConnectorSetup()"><span class="setup-status-icon">✓</span><span><strong>Finish for now</strong><small>Return to the tunnel dashboard. More connectors can be added at any time.</small></span></button></div></div><div class="setup-actions"><button class="btn secondary" onclick="connectorSetupLater()">Close</button><div class="setup-spacer"></div><button class="btn primary" onclick="finishConnectorSetup()">Finish</button></div>`;return;
  }
  if(CONNECT_SETUP.phase==='add'){
    root.innerHTML=`${setupHeader('Add another connector','Create an independent credential for another host. Existing connectors stay online.')}${setupProgress(1,3)}<div class="setup-body"><div class="setup-inline-form"><div class="field span-2"><label>Connector ID</label><input class="input mono" id="wizardAddConnectorId" value="${esc(CONNECT_SETUP.connectorId)}"><small>Use a stable name such as homelab-backup or nas-2.</small></div></div><div class="enroll-section-title" style="margin-top:17px">Update method</div><div class="setup-choice-grid"><label class="setup-choice ${CONNECT_SETUP.mode==='auto'?'selected':''}"><input type="radio" name="wizardAddMode" value="auto" ${CONNECT_SETUP.mode==='auto'?'checked':''} onchange="setWizardAddMode('auto')"><span><strong>Automatic updates</strong><small>Docker Compose with authenticated updater sidecar.</small><em>Requires Docker socket access.</em></span></label><label class="setup-choice ${CONNECT_SETUP.mode==='manual'?'selected':''}"><input type="radio" name="wizardAddMode" value="manual" ${CONNECT_SETUP.mode==='manual'?'checked':''} onchange="setWizardAddMode('manual')"><span><strong>Manual updates</strong><small>No Docker socket. Update the connector yourself.</small></span></label></div></div><div class="setup-actions"><button class="btn secondary" onclick="CONNECT_SETUP.phase='redundancy';renderConnectorSetup()">Back</button><div class="setup-spacer"></div><button class="btn primary" onclick="createWizardAdditionalConnector()">Create connector</button></div>`;
  }
}

function connectorSetupIdChanged(value){
  const id=String(value||'').trim();if(!id||!/^[A-Za-z0-9._-]{1,128}$/.test(id))return;
  CONNECT_SETUP.connectorId=id;CONNECT_SETUP.connected=false;prepareConnectorCommands(false);renderConnectorSetup();
}
function updateConnectorWaitUI(){
  const card=document.getElementById('connectorWaitStatus'),btn=document.getElementById('connectorSetupContinue');if(!card||!btn)return;
  if(CONNECT_SETUP.connected){
    const c=(connectorList(CLIENT_STATS[CONNECT_SETUP.tunnelId]||{}).find(x=>x.id===CONNECT_SETUP.connectorId)||{});
    card.classList.add('connected');card.querySelector('.setup-status-icon').textContent='✓';card.querySelector('strong').textContent='Connected';card.querySelector('.setup-status-copy span').textContent=[c.hostname,c.version,c.remote_addr].filter(Boolean).join(' · ')||CONNECT_SETUP.connectorId;btn.disabled=false;btn.textContent='Continue';
  }else{card.classList.remove('connected');btn.disabled=true}
}
async function checkConnectorSetupConnection(){
  if(!CONNECT_SETUP.tunnelId)return false;
  const {ok,data}=await api('client-stats');if(!ok||!data)return false;CLIENT_STATS=data;
  const match=connectorList(data[CONNECT_SETUP.tunnelId]||{}).find(x=>x.id===CONNECT_SETUP.connectorId&&x.online);
  CONNECT_SETUP.connected=!!match;updateConnectorWaitUI();if(match)stopConnectorSetupPolling();return !!match;
}
function startConnectorSetupPolling(){stopConnectorSetupPolling();checkConnectorSetupConnection();CONNECT_SETUP.poll=setInterval(checkConnectorSetupConnection,2000)}
function stopConnectorSetupPolling(){if(CONNECT_SETUP.poll){clearInterval(CONNECT_SETUP.poll);CONNECT_SETUP.poll=null}}
function connectorSetupContinue(){if(!CONNECT_SETUP.connected)return;CONNECT_SETUP.phase='redundancy';CONNECT_SETUP.openedMethod='';loadTunnels().then(renderConnectorSetup)}
function setWizardAddMode(mode){CONNECT_SETUP.mode=mode==='manual'?'manual':'auto';renderConnectorSetup()}
async function showAdditionalConnectorForm(){
  await loadTunnels();const t=TUNNELS.find(x=>x.id===CONNECT_SETUP.tunnelId);CONNECT_SETUP.connectorId=t?nextConnectorID(t):(CONNECT_SETUP.tunnelName.toLowerCase().replace(/[^a-z0-9._-]+/g,'-')+'-2');CONNECT_SETUP.mode='auto';CONNECT_SETUP.phase='add';renderConnectorSetup();
}
async function createWizardAdditionalConnector(){
  const id=(document.getElementById('wizardAddConnectorId')?.value||'').trim();if(!id){toast('Enter a connector ID');return}if(!/^[A-Za-z0-9._-]{1,128}$/.test(id)){toast('Connector ID may only contain letters, numbers, dot, underscore and dash');return}
  const {ok,data}=await api('connectors/add',{method:'POST',body:JSON.stringify({tunnel_id:CONNECT_SETUP.tunnelId,connector_id:id})});if(!ok){toast(data?.error||'Could not create connector credential');return}
  CONNECT_SETUP.token=data.token||'';CONNECT_SETUP.connectorId=id;CONNECT_SETUP.connected=false;CONNECT_SETUP.phase='install';CONNECT_SETUP.openedMethod='';prepareConnectorCommands(true);renderConnectorSetup();startConnectorSetupPolling();await loadTunnels();
}
async function finishConnectorSetup(){
  stopConnectorSetupPolling();document.getElementById('connectorSetupWizardModal').classList.remove('show');CONNECT_SETUP.token='';await loadTunnels();const cb=CONNECT_SETUP.afterFinish;CONNECT_SETUP.afterFinish=null;if(cb)cb();
}
async function closeConnectorSetupLater(){
  stopConnectorSetupPolling();document.getElementById('connectorSetupWizardModal').classList.remove('show');if(CONNECT_SETUP.context==='first-run')await api('setup-state',{method:'POST',body:JSON.stringify({action:'dismiss'})});CONNECT_SETUP.token='';await loadTunnels();
}
function connectorSetupLater(){
  if(CONNECT_SETUP.token&&!CONNECT_SETUP.connected){confirmDialog('Close setup?','This credential is only shown once. Copy an installation command before closing, otherwise you may need to create a new connector credential.',closeConnectorSetupLater);return}closeConnectorSetupLater();
}

// Replace the old create-tunnel -> expanded Compose flow with the guided connector wizard.
createTunnel=async function(){
  const ep=endpointFromStatus();if(!ep.ok){closeModal('createTunnelModal');showTab('settings');toast('Configure the Control Node endpoint first');return}
  const name=document.getElementById('newTunnel').value.trim();if(!name){toast('Enter a tunnel name');return}
  const mode=FIRST_UPDATE_MODE==='manual'?'manual':'auto';
  const {ok,data}=await api('tunnels',{method:'POST',body:JSON.stringify({name})});if(!ok||!data){toast(data?.error||'Could not create tunnel');return}
  closeModal('createTunnelModal');if(data.tunnel?.id)OPEN_TUNNELS.add(data.tunnel.id);await Promise.all([loadStatus(),loadTunnels()]);
  launchConnectorSetup({tunnelId:data.tunnel.id,tunnelName:data.tunnel.name||name,token:data.token,connectorId:defaultConnectorID(data.tunnel.name||name),mode,context:'normal'});
};

// Add Connector now uses the same connection-aware wizard instead of immediately dumping Compose text.
submitAddConnector=async function(){
  const t=TUNNELS.find(x=>x.id===ADD_CONNECTOR_TUNNEL);if(!t)return;
  const connector_id=(document.getElementById('addConnectorId')?.value||'').trim();if(!connector_id){toast('Enter a connector ID');return}if(!/^[A-Za-z0-9._-]{1,128}$/.test(connector_id)){toast('Connector ID may only contain letters, numbers, dot, underscore and dash');return}
  const mode=document.querySelector('input[name="connectorUpdateMode"]:checked')?.value==='manual'?'manual':'auto';
  const {ok,data}=await api('connectors/add',{method:'POST',body:JSON.stringify({tunnel_id:t.id,connector_id})});if(!ok){toast(data?.error||'Could not create connector credential');return}
  closeModal('addConnectorModal');OPEN_TUNNELS.add(t.id);await loadTunnels();launchConnectorSetup({tunnelId:t.id,tunnelName:data.tunnel_name||t.name,token:data.token,connectorId:connector_id,mode,context:'normal'});
};

function firstRunChoice(mode){FIRST_RUN_MODE=mode==='manual'?'manual':'auto';renderFirstRun('tunnel')}
function firstRunNameChanged(value){if(FIRST_RUN_CONNECTOR_TOUCHED)return;const field=document.getElementById('firstRunConnectorId');if(field)field.value=defaultConnectorID(value||'connector')}
function firstRunConnectorTouched(){FIRST_RUN_CONNECTOR_TOUCHED=true}

async function openFirstRunWizard(manual=false){
  await Promise.all([loadStatus(),loadTunnels()]);
  if(manual)await api('setup-state',{method:'POST',body:JSON.stringify({action:'reset'})});
  FIRST_RUN_MODE='auto';FIRST_RUN_CONNECTOR_TOUCHED=false;document.getElementById('firstRunWizardModal').classList.add('show');renderFirstRun('welcome');
}
function renderFirstRun(step){
  const root=document.getElementById('firstRunWizard');if(!root)return;
  if(step==='welcome'){
    root.innerHTML=`<div class="setup-wizard-head"><div><h2>Welcome to Zoraxy Tunnel Enhanced</h2><p>A short guided setup will configure the control node and connect your first tunnel.</p></div></div>${setupProgress(1,4)}<div class="setup-body"><div class="setup-finish"><div class="setup-finish-mark">⇄</div><h3>Set up your tunnel node</h3><p>The wizard checks each step as you go. Nothing is hidden behind a large command block, and you can leave setup at any time.</p></div><div class="setup-summary"><div class="setup-summary-item"><span>Step 1</span><strong>Configure control endpoint</strong></div><div class="setup-summary-item"><span>Step 2</span><strong>Create your first tunnel</strong></div><div class="setup-summary-item"><span>Step 3</span><strong>Connect and verify a client</strong></div><div class="setup-summary-item"><span>Optional</span><strong>Add a redundant connector</strong></div></div></div><div class="setup-actions"><button class="btn secondary" onclick="dismissFirstRunSetup()">Setup later</button><div class="setup-spacer"></div><button class="btn primary" onclick="renderFirstRun('control')">Start setup</button></div>`;return;
  }
  if(step==='control'){
    root.innerHTML=`<div class="setup-wizard-head"><div><h2>Control node</h2><p>Tell clients how to reach this Zoraxy instance.</p></div></div>${setupProgress(1,4)}<div class="setup-body"><div class="setup-inline-form"><div class="field span-2"><label>Public hostname or address</label><input class="input" id="firstRunHost" value="${esc(STATUS.server_host||'')}" placeholder="tunnel.example.com"><small>Port ${STATUS.control_port||9443} is added automatically when omitted.</small></div><div class="field span-2"><label>Default Zoraxy TAG <span>optional</span></label><input class="input" id="firstRunTag" value="${esc(STATUS.default_tag||'ZoraxyTunnel')}" placeholder="ZoraxyTunnel"></div></div><div class="setup-summary"><div class="setup-summary-item"><span>Control port</span><strong class="mono">:${STATUS.control_port||9443}</strong></div><div class="setup-summary-item"><span>Ingress port</span><strong class="mono">:${STATUS.ingress_port||9080}</strong></div></div></div><div class="setup-actions"><button class="btn secondary" onclick="dismissFirstRunSetup()">Setup later</button><div class="setup-spacer"></div><button class="btn secondary" onclick="renderFirstRun('welcome')">Back</button><button class="btn primary" onclick="saveFirstRunControl()">Save & continue</button></div>`;return;
  }
  if(step==='tunnel'){
    const defaultName='HomeLab',defaultID=defaultConnectorID(defaultName);
    root.innerHTML=`<div class="setup-wizard-head"><div><h2>Create your first tunnel</h2><p>Create the logical tunnel and choose how its first connector should be updated.</p></div></div>${setupProgress(2,4)}<div class="setup-body"><div class="setup-inline-form"><div class="field"><label>Tunnel name</label><input class="input" id="firstRunTunnelName" value="${defaultName}" oninput="firstRunNameChanged(this.value)"></div><div class="field"><label>Connector ID</label><input class="input mono" id="firstRunConnectorId" value="${defaultID}" oninput="firstRunConnectorTouched()"><small>Unique and stable for this host.</small></div></div><div class="enroll-section-title" style="margin-top:18px">Update method</div><div class="setup-choice-grid"><label class="setup-choice ${FIRST_RUN_MODE==='auto'?'selected':''}"><input type="radio" name="firstRunMode" value="auto" ${FIRST_RUN_MODE==='auto'?'checked':''} onchange="firstRunChoice('auto')"><span><strong>Automatic updates</strong><small>Recommended for Docker. Uses the authenticated updater sidecar.</small><em>Requires /var/run/docker.sock.</em></span></label><label class="setup-choice ${FIRST_RUN_MODE==='manual'?'selected':''}"><input type="radio" name="firstRunMode" value="manual" ${FIRST_RUN_MODE==='manual'?'checked':''} onchange="firstRunChoice('manual')"><span><strong>Manual updates</strong><small>No Docker socket. You decide when the connector is updated.</small></span></label></div></div><div class="setup-actions"><button class="btn secondary" onclick="dismissFirstRunSetup()">Setup later</button><div class="setup-spacer"></div><button class="btn secondary" onclick="renderFirstRun('control')">Back</button><button class="btn primary" onclick="createFirstRunTunnel()">Create tunnel</button></div>`;return;
  }
  if(step==='finish'){
    const t=TUNNELS.find(x=>x.id===CONNECT_SETUP.tunnelId),stats=CLIENT_STATS[CONNECT_SETUP.tunnelId]||{},online=connectorList(stats).filter(x=>x.online).length;
    root.innerHTML=`<div class="setup-wizard-head"><div><h2>Setup complete</h2><p>Your control node and first tunnel are ready.</p></div></div>${setupProgress(4,4)}<div class="setup-body"><div class="setup-finish"><div class="setup-finish-mark">✓</div><h3>Zoraxy Tunnel Enhanced is ready</h3><p>${esc(t?.name||CONNECT_SETUP.tunnelName)} has ${online} online connector${online===1?'':'s'}. You can now publish a service through the tunnel or return to the dashboard.</p></div></div><div class="setup-actions"><button class="btn secondary" onclick="completeFirstRun(false)">Go to dashboard</button><div class="setup-spacer"></div><button class="btn primary" onclick="completeFirstRun(true)">Register first service</button></div>`;
  }
}
async function saveFirstRunControl(){
  const raw=(document.getElementById('firstRunHost')?.value||'').trim(),ep=normalizeEndpoint(raw);if(!ep.ok){toast(ep.error);return}const default_tag=(document.getElementById('firstRunTag')?.value||'').trim();const {ok,data}=await api('settings',{method:'POST',body:JSON.stringify({server_host:ep.value,default_tag})});if(!ok){toast(data?.error||'Could not save Control Node');return}await loadStatus();renderFirstRun('tunnel');
}
async function createFirstRunTunnel(){
  const name=(document.getElementById('firstRunTunnelName')?.value||'').trim(),connectorId=(document.getElementById('firstRunConnectorId')?.value||'').trim();if(!name){toast('Enter a tunnel name');return}if(!connectorId||!/^[A-Za-z0-9._-]{1,128}$/.test(connectorId)){toast('Enter a valid connector ID');return}
  const {ok,data}=await api('tunnels',{method:'POST',body:JSON.stringify({name})});if(!ok||!data){toast(data?.error||'Could not create tunnel');return}
  document.getElementById('firstRunWizardModal').classList.remove('show');OPEN_TUNNELS.add(data.tunnel.id);await loadTunnels();launchConnectorSetup({tunnelId:data.tunnel.id,tunnelName:data.tunnel.name||name,token:data.token,connectorId,mode:FIRST_RUN_MODE,context:'first-run',afterFinish:()=>{document.getElementById('firstRunWizardModal').classList.add('show');renderFirstRun('finish')}});
}
async function dismissFirstRunSetup(){await api('setup-state',{method:'POST',body:JSON.stringify({action:'dismiss'})});document.getElementById('firstRunWizardModal').classList.remove('show');toast('Setup saved for later')}
async function completeFirstRun(registerService){await api('setup-state',{method:'POST',body:JSON.stringify({action:'complete'})});document.getElementById('firstRunWizardModal').classList.remove('show');await Promise.all([loadStatus(),loadTunnels()]);if(registerService&&CONNECT_SETUP.tunnelId)openSvc(CONNECT_SETUP.tunnelId)}

async function maybeStartFirstRun(){
  await Promise.all([loadStatus(),loadTunnels()]);const {ok,data}=await api('setup-state');if(!ok)return;const fresh=!data?.completed&&!data?.dismissed&&TUNNELS.length===0;if(fresh)openFirstRunWizard(false)
}
setTimeout(maybeStartFirstRun,450);
