// v1.13 interaction polish kept separate from the main wizard so the setup
// flow can evolve without touching older enrollment compatibility layers.

firstRunChoice=function(mode){
  FIRST_RUN_MODE=mode==='manual'?'manual':'auto';
  document.querySelectorAll('#firstRunWizard input[name="firstRunMode"]').forEach(r=>{
    r.checked=r.value===FIRST_RUN_MODE;
    r.closest('.setup-choice')?.classList.toggle('selected',r.checked);
  });
};

setWizardAddMode=function(mode){
  const field=document.getElementById('wizardAddConnectorId');
  const id=(field?.value||'').trim();
  if(id&&/^[A-Za-z0-9._-]{1,128}$/.test(id))CONNECT_SETUP.connectorId=id;
  CONNECT_SETUP.mode=mode==='manual'?'manual':'auto';
  renderConnectorSetup();
};

connectorSetupIdChanged=function(value){
  const id=String(value||'').trim();
  if(!id||!/^[A-Za-z0-9._-]{1,128}$/.test(id))return;
  CONNECT_SETUP.connectorId=id;
  CONNECT_SETUP.connected=false;
  prepareConnectorCommands(false);
  const wait=document.querySelector('#connectorWaitStatus .setup-status-copy span');
  if(wait)wait.textContent='Zoraxy is checking every 2 seconds for '+id+'.';
  const title=document.querySelector('#connectorSetupWizard .setup-wizard-head h2');
  if(title)title.textContent='Connect '+id;
  ['compose','docker','cli'].forEach(method=>{
    const pre=document.querySelector('#setupMethod-'+method+' pre');
    if(pre)pre.textContent=CONNECT_SETUP.commands[method]||'';
  });
  updateConnectorWaitUI();
  if(!CONNECT_SETUP.poll)startConnectorSetupPolling();
};

openFirstRunWizard=async function(){
  await Promise.all([loadStatus(),loadTunnels()]);
  FIRST_RUN_MODE='auto';
  FIRST_RUN_CONNECTOR_TOUCHED=false;
  document.getElementById('firstRunWizardModal').classList.add('show');
  renderFirstRun('welcome');
};
