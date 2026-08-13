import {
  dom,
  formatBytes,
  jsonBody,
  openInfoModal,
  requestJSON,
  showToast,
  state
} from './core.js';

export function initUpdater() {
  dom.btnManualCheck.addEventListener('click', () => checkForUpdates(true));
  dom.btnCheckUpdate.addEventListener('click', showAvailableUpdate);
  dom.btnDownloadUpdate.addEventListener('click', downloadUpdate);
  dom.btnReleaseNotes.addEventListener('click', showReleaseNotes);
  dom.btnRestartNow.addEventListener('click', applyUpdate);
  window.addEventListener('beforeunload', () => clearTimeout(state.updateTimer));

  loadStatus().finally(() => {
    const schedule = window.requestIdleCallback || ((callback) => setTimeout(callback, 1200));
    schedule(() => checkForUpdates(false));
  });
}

async function loadStatus() {
  try {
    const status = await requestJSON('api/status');
    setVersion(status.version);
    const progress = status.update || {};
    if (progress.state === 'downloading' || progress.state === 'verifying') {
      state.updateState = progress.state;
      dom.updateCard.hidden = false;
      renderProgress(progress);
      scheduleProgressPoll();
    } else if (progress.state === 'ready') {
      state.updateState = 'ready';
      dom.updateCard.hidden = false;
      renderProgress(progress);
    }
  } catch {
    try {
      const version = await requestJSON('api/version');
      setVersion(version.version);
    } catch {
      // The main UI remains usable without update metadata.
    }
  }
}

async function checkForUpdates(manual) {
  if (['checking', 'downloading', 'verifying', 'ready', 'applying'].includes(state.updateState)) return;
  state.updateState = 'checking';
  dom.btnManualCheck.disabled = true;
  dom.btnManualCheck.textContent = '…';
  if (manual) showToast('Checking for updates');
  try {
    const info = await requestJSON('api/check-update?t=' + Date.now());
    setVersion(info.current_version);
    if (!info.has_update) {
      state.updateInfo = null;
      state.updateState = 'idle';
      dom.btnCheckUpdate.hidden = true;
      dom.updateCard.hidden = true;
      if (manual) showToast('CodeDocs is up to date');
      return;
    }
    state.updateInfo = info;
    state.updateState = 'available';
    dom.btnCheckUpdate.hidden = false;
    dom.btnCheckUpdate.textContent = info.latest_version || 'Update';
    if (manual) {
      showAvailableUpdate();
      showToast('A verified update is available');
    }
  } catch (error) {
    state.updateState = 'idle';
    if (manual) showToast(error.message || 'Unable to check for updates', { error: true });
  } finally {
    dom.btnManualCheck.disabled = false;
    dom.btnManualCheck.textContent = '↻';
  }
}

function showAvailableUpdate() {
  const info = state.updateInfo;
  if (!info) return;
  dom.updateCard.hidden = false;
  dom.updateCardTitle.textContent = info.latest_version || 'Update available';
  dom.updateCardPct.textContent = info.is_verified ? 'SHA-256' : 'Unverified';
  dom.updateProgressFill.style.width = '0%';
  dom.updateCardSub.textContent = info.is_verified
    ? formatBytes(info.size_bytes) + ' · ' + (info.platform || 'compatible build')
    : 'This release cannot be installed automatically.';
  dom.btnReleaseNotes.hidden = !info.release_notes;
  dom.btnDownloadUpdate.hidden = !info.download_url || !info.is_verified;
  dom.btnRestartNow.hidden = true;
}

async function downloadUpdate() {
  const info = state.updateInfo;
  if (!info?.download_url || !info.is_verified) return;
  state.updateState = 'downloading';
  dom.btnDownloadUpdate.disabled = true;
  dom.btnDownloadUpdate.textContent = 'Starting…';
  dom.btnCheckUpdate.hidden = true;
  try {
    await requestJSON('api/download-update', {
      method: 'POST',
      body: jsonBody({ download_url: info.download_url })
    });
    dom.updateCardTitle.textContent = 'Downloading ' + (info.latest_version || 'update');
    scheduleProgressPoll(50);
  } catch (error) {
    state.updateState = 'available';
    dom.btnDownloadUpdate.disabled = false;
    dom.btnDownloadUpdate.textContent = 'Retry';
    dom.btnCheckUpdate.hidden = false;
    showToast(error.message || 'Unable to start download', { error: true });
  }
}

function scheduleProgressPoll(delay = 350) {
  clearTimeout(state.updateTimer);
  state.updateTimer = setTimeout(pollProgress, delay);
}

async function pollProgress() {
  try {
    const progress = await requestJSON('api/update-progress?t=' + Date.now());
    renderProgress(progress);
    if (progress.state === 'downloading' || progress.state === 'verifying') {
      state.updateState = progress.state;
      scheduleProgressPoll(progress.state === 'verifying' ? 220 : 350);
    }
  } catch {
    if (state.updateState === 'downloading' || state.updateState === 'verifying') scheduleProgressPoll(900);
  }
}

function renderProgress(progress) {
  const percent = Math.max(0, Math.min(100, Number(progress.percent) || 0));
  dom.updateCard.hidden = false;
  dom.updateProgressFill.style.width = percent + '%';
  dom.updateCardPct.textContent = percent + '%';
  dom.btnDownloadUpdate.hidden = true;
  dom.btnRestartNow.hidden = true;

  if (progress.state === 'downloading') {
    dom.updateCardTitle.textContent = 'Downloading ' + (progress.version || 'update');
    dom.updateCardSub.textContent = progress.total_bytes > 0
      ? formatBytes(progress.downloaded_bytes) + ' / ' + formatBytes(progress.total_bytes)
      : (progress.message || 'Downloading');
    return;
  }
  if (progress.state === 'verifying') {
    dom.updateCardTitle.textContent = 'Verifying download';
    dom.updateCardSub.textContent = 'Checking SHA-256 and executable architecture';
    return;
  }
  if (progress.state === 'ready') {
    state.updateState = 'ready';
    clearTimeout(state.updateTimer);
    dom.updateProgressFill.style.width = '100%';
    dom.updateCardPct.textContent = 'Ready';
    dom.updateCardTitle.textContent = (progress.version || 'Update') + ' verified';
    dom.updateCardSub.textContent = 'Restart when convenient. Unsaved editor text is not modified.';
    dom.btnRestartNow.hidden = false;
    dom.btnRestartNow.disabled = false;
    dom.btnRestartNow.textContent = 'Restart';
    showToast('Update verified and ready');
    return;
  }
  if (progress.state === 'error') {
    state.updateState = 'available';
    clearTimeout(state.updateTimer);
    dom.updateCardPct.textContent = 'Error';
    dom.updateCardTitle.textContent = 'Update paused';
    dom.updateCardSub.textContent = progress.error || 'Download failed';
    dom.btnDownloadUpdate.hidden = !state.updateInfo?.download_url;
    dom.btnDownloadUpdate.disabled = false;
    dom.btnDownloadUpdate.textContent = 'Retry';
    dom.btnCheckUpdate.hidden = false;
    showToast(progress.error || 'Update download failed', { error: true });
  }
}

function showReleaseNotes() {
  const info = state.updateInfo;
  if (!info) return;
  openInfoModal(info.latest_version || 'Release notes', info.release_notes || 'No release notes.', {
    preformatted: true,
    copyText: info.release_notes || ''
  });
}

async function applyUpdate() {
  if (state.updateState !== 'ready') return;
  state.updateState = 'applying';
  dom.btnRestartNow.disabled = true;
  dom.btnRestartNow.textContent = 'Restarting…';
  dom.updateCardTitle.textContent = 'Installing verified update';
  dom.updateCardSub.textContent = 'CodeDocs will close and reopen automatically.';
  try {
    await requestJSON('api/apply-update', { method: 'POST' });
    showToast('Restarting CodeDocs');
    await waitForRestart();
  } catch (error) {
    state.updateState = 'ready';
    dom.btnRestartNow.disabled = false;
    dom.btnRestartNow.textContent = 'Retry restart';
    showToast(error.message || 'Unable to apply update', { error: true, duration: 5000 });
  }
}

async function waitForRestart() {
  const deadline = Date.now() + 60000;
  let observedOffline = false;
  const expectedVersion = normalizeVersion(state.updateInfo?.latest_version);
  while (Date.now() < deadline) {
    await delay(500);
    try {
      const ping = await requestJSON('api/ping?t=' + Date.now());
      if (observedOffline || (expectedVersion && normalizeVersion(ping.version) === expectedVersion)) {
        location.reload();
        return;
      }
    } catch {
      observedOffline = true;
    }
  }
  throw new Error('Restart is taking longer than expected. Reopen CodeDocs if needed.');
}

function setVersion(version) {
  if (!version) return;
  const normalized = String(version).startsWith('v') ? String(version) : 'v' + version;
  dom.appVersion.textContent = normalized;
  dom.verInfo.textContent = 'CodeDocs ' + normalized;
}

function normalizeVersion(version) {
  return String(version || '').trim().replace(/^v/i, '');
}

function delay(milliseconds) {
  return new Promise((resolve) => setTimeout(resolve, milliseconds));
}
