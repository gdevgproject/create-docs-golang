const requiredIDs = [
  'path-input', 'bm-note', 'bm-list', 'bm-count', 'btn-add-bm',
  'btn-struct', 'btn-gen', 'btn-cancel-gen', 'btn-exclusions', 'btn-counter',
  'editor', 'empty-state', 'file-badge', 'stats-cards', 'stat-files',
  'stat-lines', 'stat-tokens', 'card-tokens', 'stat-size', 'stat-time',
  'stat-date', 'btn-load', 'btn-copy', 'btn-download', 'status-panel',
  'progress-fill', 'percent-text', 'speed-text', 'log-text', 'toast',
  'modal-dialog', 'modal-title', 'modal-content', 'btn-copy-modal',
  'btn-modal-primary', 'btn-close-modal', 'virtual-banner', 'v-banner-text',
  'btn-render-all', 'btn-stop-render', 'app-version', 'ver-info',
  'btn-manual-check', 'btn-check-update', 'update-card', 'update-card-title',
  'update-card-pct', 'update-progress-fill', 'update-card-sub',
  'btn-download-update', 'btn-release-notes', 'btn-restart-now',
  'btn-export-bm', 'btn-import-bm', 'import-file-input', 'projects-panel',
  'history-panel', 'hp-bm-name', 'hp-timeline', 'btn-hp-clear',
  'btn-toggle-projects', 'btn-toggle-history', 'btn-close-projects',
  'btn-close-history', 'panel-backdrop', 'mode-hint'
];

export const dom = Object.fromEntries(requiredIDs.map((id) => [toCamel(id), document.getElementById(id)]));

export const state = {
  allBookmarks: {},
  activeBookmarkId: null,
  lastGeneratedFile: null,
  cachedFullText: null,
  previewTruncated: false,
  contentBytes: 0,
  activeSource: null,
  operationController: null,
  contentController: null,
  tokenController: null,
  updateInfo: null,
  updateState: 'idle',
  updateTimer: null,
  modalCleanup: null,
  toastTimer: null
};

const preferenceKey = 'codedocs.ui.v2';
let preferences = readPreferences();
let pathSaveTimer = null;

export class RequestError extends Error {
  constructor(message, status = 0, payload = null) {
    super(message);
    this.name = 'RequestError';
    this.status = status;
    this.payload = payload;
  }
}

export function initCore() {
  const missing = Object.entries(dom).filter(([, value]) => !value).map(([key]) => key);
  if (missing.length > 0) {
    throw new Error('Missing required UI elements: ' + missing.join(', '));
  }

  dom.pathInput.value = typeof preferences.path === 'string' ? preferences.path : '';
  const preferredMode = preferences.mode === 'full' ? 'full' : 'stats';
  const modeInput = document.querySelector('input[name="gen-mode"][value="' + preferredMode + '"]');
  if (modeInput) modeInput.checked = true;
  updateModeHint();

  dom.pathInput.addEventListener('input', () => {
    clearTimeout(pathSaveTimer);
    pathSaveTimer = setTimeout(() => savePreferences({ path: dom.pathInput.value }), 250);
  });
  document.querySelectorAll('input[name="gen-mode"]').forEach((input) => {
    input.addEventListener('change', () => {
      savePreferences({ mode: selectedMode() });
      updateModeHint();
    });
  });

  dom.btnToggleProjects.addEventListener('click', toggleProjects);
  dom.btnToggleHistory.addEventListener('click', toggleHistory);
  dom.btnCloseProjects.addEventListener('click', closePanels);
  dom.btnCloseHistory.addEventListener('click', closePanels);
  dom.panelBackdrop.addEventListener('click', closePanels);
  dom.btnCloseModal.addEventListener('click', closeModal);
  dom.modalDialog.addEventListener('mousedown', (event) => {
    if (event.target === dom.modalDialog) closeModal();
  });
  dom.btnCopyModal.addEventListener('click', async () => {
    const text = dom.btnCopyModal.dataset.copyText || dom.modalContent.textContent || '';
    if (text) await copyToClipboard(text);
  });

  dom.btnDownload.addEventListener('click', (event) => {
    if (dom.btnDownload.classList.contains('disabled')) event.preventDefault();
  });

  document.addEventListener('keydown', (event) => {
    if (event.key === 'Escape') {
      if (!dom.modalDialog.hidden) closeModal();
      else closePanels();
      return;
    }
    if (event.ctrlKey && event.key === 'Enter' && dom.modalDialog.hidden) {
      event.preventDefault();
      if (!dom.btnGen.disabled) dom.btnGen.click();
    }
  });
  window.addEventListener('resize', handleLayoutResize, { passive: true });
  window.addEventListener('beforeunload', disposeTransientWork);

  if (preferences.projectsCollapsed) document.body.classList.add('projects-collapsed');
  if (preferences.historyCollapsed) document.body.classList.add('history-collapsed');
  handleLayoutResize();
  setEditor('');
}

export function selectedMode() {
  const selected = document.querySelector('input[name="gen-mode"]:checked');
  return selected ? selected.value : 'stats';
}

export function cleanPath(value) {
  return String(value || '').replace(/^[\"'\s]+|[\"'\s]+$/g, '').trim();
}

export function normalizedPath(value) {
  return cleanPath(value).replace(/\\/g, '/').replace(/\/+$/, '').toLocaleLowerCase();
}

export function formatBytes(bytes) {
  const value = Number(bytes) || 0;
  if (value < 1024) return value.toLocaleString() + ' B';
  if (value < 1024 * 1024) return (value / 1024).toFixed(1) + ' KB';
  if (value < 1024 * 1024 * 1024) return (value / (1024 * 1024)).toFixed(2) + ' MB';
  return (value / (1024 * 1024 * 1024)).toFixed(2) + ' GB';
}

export function formatNumber(value) {
  return (Number(value) || 0).toLocaleString();
}

export function formatTime(value) {
  return String(value || '').replace(/\s*\([^)]*\)\s*$/, '').trim() || 'Unknown';
}

export function showToast(message, options = {}) {
  clearTimeout(state.toastTimer);
  dom.toast.textContent = String(message || '');
  dom.toast.classList.toggle('error', options.error === true);
  dom.toast.classList.add('show');
  state.toastTimer = setTimeout(() => dom.toast.classList.remove('show'), options.duration || 2600);
}

export async function requestJSON(path, options = {}) {
  const response = await fetch(path, {
    cache: 'no-store',
    ...options,
    headers: {
      Accept: 'application/json',
      ...(options.body && typeof options.body === 'string' ? { 'Content-Type': 'application/json' } : {}),
      ...(options.headers || {})
    }
  });
  const raw = await response.text();
  let payload = null;
  if (raw) {
    try {
      payload = JSON.parse(raw);
    } catch {
      payload = null;
    }
  }
  if (!response.ok) {
    const message = payload && payload.message ? payload.message : 'Request failed (' + response.status + ')';
    throw new RequestError(message, response.status, payload);
  }
  return payload;
}

export function jsonBody(value) {
  return JSON.stringify(value);
}

export function setEditor(text) {
  const value = String(text || '');
  dom.editor.value = value;
  dom.emptyState.hidden = value.length > 0;
}

export function setOutputStatus(label, kind = 'idle') {
  dom.fileBadge.textContent = label;
  dom.fileBadge.className = 'status-badge ' + kind;
}

export function setStats(result) {
  if (!result) {
    dom.statsCards.hidden = true;
    dom.statDate.textContent = '';
    return;
  }
  dom.statsCards.hidden = false;
  dom.statFiles.textContent = formatNumber(result.total);
  dom.statLines.textContent = formatNumber(result.lines);
  const tokenPrefix = result.token_mode && result.token_mode !== 'exact' ? '~' : '';
  dom.statTokens.textContent = tokenPrefix + formatNumber(result.tokens);
  dom.statSize.textContent = formatBytes(result.size);
  dom.statTime.textContent = Number(result.elapsed || 0).toFixed(2).replace(/\.?0+$/, '') + 's';
  dom.statDate.textContent = result.generated_at || '';
  dom.cardTokens.title = result.token_mode === 'exact'
    ? 'Exact o200k_base token count'
    : 'Estimated token count';
}

export function setGeneratedFile(fileName) {
  state.lastGeneratedFile = fileName || null;
  if (!fileName) {
    dom.btnDownload.href = '#';
    dom.btnDownload.classList.add('disabled');
    dom.btnDownload.setAttribute('aria-disabled', 'true');
    return;
  }
  dom.btnDownload.href = 'api/download?file=' + encodeURIComponent(fileName);
  dom.btnDownload.classList.remove('disabled');
  dom.btnDownload.setAttribute('aria-disabled', 'false');
}

export function resetContentState() {
  state.cachedFullText = null;
  state.previewTruncated = false;
  state.contentBytes = 0;
  if (state.contentController) state.contentController.abort();
  state.contentController = null;
  dom.virtualBanner.hidden = true;
  dom.btnLoad.hidden = true;
  dom.btnLoad.disabled = false;
  dom.btnCopy.disabled = true;
  setGeneratedFile(null);
}

export function setBusy(busy) {
  dom.btnGen.disabled = busy;
  dom.btnStruct.disabled = busy;
  dom.btnCancelGen.hidden = !busy;
}

export async function copyToClipboard(text) {
  try {
    await navigator.clipboard.writeText(text);
  } catch {
    const selectionStart = dom.editor.selectionStart;
    const selectionEnd = dom.editor.selectionEnd;
    const previous = dom.editor.value;
    dom.editor.value = text;
    dom.editor.select();
    const copied = document.execCommand('copy');
    dom.editor.value = previous;
    dom.editor.setSelectionRange(selectionStart, selectionEnd);
    if (!copied) throw new Error('Clipboard permission was denied');
  }
  showToast('Copied to clipboard');
}

export function openInfoModal(title, content, options = {}) {
  prepareModal(title);
  if (content instanceof Node) dom.modalContent.appendChild(content);
  else dom.modalContent.textContent = String(content || '');
  if (options.preformatted) {
    const pre = document.createElement('pre');
    pre.textContent = dom.modalContent.textContent;
    dom.modalContent.replaceChildren(pre);
  }
  if (options.copyText != null) {
    dom.btnCopyModal.hidden = false;
    dom.btnCopyModal.dataset.copyText = String(options.copyText);
  }
  dom.modalDialog.hidden = false;
  queueMicrotask(() => dom.btnCloseModal.focus());
}

export function promptDialog(title, value = '', message = '') {
  return new Promise((resolve) => {
    prepareModal(title);
    if (message) {
      const copy = document.createElement('p');
      copy.className = 'dialog-copy';
      copy.textContent = message;
      dom.modalContent.appendChild(copy);
    }
    const input = document.createElement('input');
    input.className = 'dialog-input';
    input.type = 'text';
    input.maxLength = 120;
    input.value = value;
    dom.modalContent.appendChild(input);
    dom.btnModalPrimary.hidden = false;
    dom.btnModalPrimary.textContent = 'Save';
    const finish = (result) => {
		state.modalCleanup = null;
		dom.btnModalPrimary.removeEventListener('click', submit);
		dom.modalDialog.hidden = true;
		resolve(result);
    };
    const submit = () => finish(input.value.trim());
    dom.btnModalPrimary.addEventListener('click', submit);
    input.addEventListener('keydown', (event) => {
      if (event.key === 'Enter') {
        event.preventDefault();
        submit();
      }
    });
    state.modalCleanup = () => {
      dom.btnModalPrimary.removeEventListener('click', submit);
      resolve(null);
    };
    dom.modalDialog.hidden = false;
    queueMicrotask(() => {
      input.focus();
      input.select();
    });
  });
}

export function confirmDialog(title, message, confirmLabel = 'Delete') {
  return new Promise((resolve) => {
    prepareModal(title);
    const copy = document.createElement('p');
    copy.className = 'dialog-copy';
    copy.textContent = message;
    dom.modalContent.appendChild(copy);
    dom.btnModalPrimary.hidden = false;
    dom.btnModalPrimary.textContent = confirmLabel;
    dom.btnModalPrimary.className = 'button danger compact';
    const confirm = () => {
      state.modalCleanup = null;
      dom.btnModalPrimary.removeEventListener('click', confirm);
      dom.modalDialog.hidden = true;
      resolve(true);
    };
    dom.btnModalPrimary.addEventListener('click', confirm);
    state.modalCleanup = () => {
      dom.btnModalPrimary.removeEventListener('click', confirm);
      resolve(false);
    };
    dom.modalDialog.hidden = false;
    queueMicrotask(() => dom.btnModalPrimary.focus());
  });
}

export function closeModal() {
  if (dom.modalDialog.hidden) return;
  const cleanup = state.modalCleanup;
  state.modalCleanup = null;
  if (cleanup) cleanup();
  dom.modalDialog.hidden = true;
  dom.modalContent.replaceChildren();
}

export function closePanels() {
  document.body.classList.remove('projects-open', 'history-open');
}

export function closeCompactPanels() {
  if (window.innerWidth <= 920) closePanels();
}

export function savePreferences(patch) {
  preferences = { ...preferences, ...patch };
  try {
    localStorage.setItem(preferenceKey, JSON.stringify(preferences));
  } catch {
    // Storage can be unavailable in hardened WebView profiles.
  }
}

export function getPreferences() {
  return { ...preferences };
}

function prepareModal(title) {
  if (state.modalCleanup) {
    const cleanup = state.modalCleanup;
    state.modalCleanup = null;
    cleanup();
  }
  dom.modalTitle.textContent = title;
  dom.modalContent.replaceChildren();
  dom.btnCopyModal.hidden = true;
  dom.btnCopyModal.dataset.copyText = '';
  dom.btnModalPrimary.hidden = true;
  dom.btnModalPrimary.className = 'button primary compact';
}

function toggleProjects() {
  if (window.innerWidth > 1280) {
    const collapsed = document.body.classList.toggle('projects-collapsed');
    savePreferences({ projectsCollapsed: collapsed });
    return;
  }
  const opening = !document.body.classList.contains('projects-open');
  closePanels();
  if (opening) document.body.classList.add('projects-open');
}

function toggleHistory() {
  if (window.innerWidth > 1280) {
    const collapsed = document.body.classList.toggle('history-collapsed');
    savePreferences({ historyCollapsed: collapsed });
    return;
  }
  const opening = !document.body.classList.contains('history-open');
  closePanels();
  if (opening) document.body.classList.add('history-open');
}

function handleLayoutResize() {
  if (window.innerWidth > 1280) closePanels();
}

function updateModeHint() {
  dom.modeHint.textContent = selectedMode() === 'full'
    ? 'Full output is generated; the preview stays memory-safe.'
    : 'Fast scans now; content loads only when needed.';
}

function readPreferences() {
  try {
    const value = JSON.parse(localStorage.getItem(preferenceKey) || '{}');
    return value && typeof value === 'object' ? value : {};
  } catch {
    return {};
  }
}

function disposeTransientWork() {
  if (state.activeSource) state.activeSource.close();
  if (state.operationController) state.operationController.abort();
  if (state.contentController) state.contentController.abort();
  if (state.tokenController) state.tokenController.abort();
  clearTimeout(state.updateTimer);
}

function toCamel(id) {
  return id.replace(/-([a-z])/g, (_, letter) => letter.toUpperCase());
}
