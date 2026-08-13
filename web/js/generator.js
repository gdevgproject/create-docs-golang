import {
  cleanPath,
  dom,
  formatBytes,
  formatNumber,
  jsonBody,
  requestJSON,
  resetContentState,
  selectedMode,
  setBusy,
  setEditor,
  setGeneratedFile,
  setOutputStatus,
  setStats,
  showToast,
  state,
  copyToClipboard
} from './core.js';
import { syncBookmarksAfterGenerate } from './bookmarks.js';

const previewBytes = 320 * 1024;

export function initGenerator() {
  dom.btnGen.addEventListener('click', startGeneration);
  dom.btnStruct.addEventListener('click', scanStructure);
  dom.btnCancelGen.addEventListener('click', () => cancelActiveWork(true));
  dom.btnLoad.addEventListener('click', loadFullContent);
  dom.btnRenderAll.addEventListener('click', loadFullContent);
  dom.btnStopRender.addEventListener('click', stopContentLoad);
  dom.btnCopy.addEventListener('click', copyCurrentContent);
}

export function cancelActiveWork(notify = false) {
  let cancelled = false;
  if (state.activeSource) {
    state.activeSource.close();
    state.activeSource = null;
    cancelled = true;
  }
  if (state.operationController) {
    state.operationController.abort();
    state.operationController = null;
    cancelled = true;
  }
  if (state.contentController) {
    state.contentController.abort();
    state.contentController = null;
    cancelled = true;
  }
  if (cancelled) {
    setBusy(false);
    dom.statusPanel.hidden = true;
    dom.btnStopRender.hidden = true;
    setOutputStatus('Cancelled', 'idle');
    if (notify) showToast('Operation cancelled');
  }
}

async function scanStructure() {
  const path = cleanPath(dom.pathInput.value);
  if (!path) {
    showToast('Enter a project path first', { error: true });
    dom.pathInput.focus();
    return;
  }
  cancelActiveWork(false);
  resetContentState();
  const controller = new AbortController();
  state.operationController = controller;
  setBusy(true);
  setOutputStatus('Scanning', 'progress');
  setEditor('Scanning project structure…');
  setStats(null);
  showProgress('Scanning project', 15);

  try {
    const result = await requestJSON('api/structure?path=' + encodeURIComponent(path), {
      signal: controller.signal
    });
    if (state.operationController !== controller) return;
    state.cachedFullText = result.data || '';
    state.previewTruncated = false;
    state.contentBytes = new Blob([state.cachedFullText]).size;
    setEditor(state.cachedFullText);
    setOutputStatus(formatNumber(result.count) + ' files', 'success');
    setStats({
      total: result.count,
      lines: result.lines,
      tokens: 0,
      token_mode: 'none',
      size: result.bytes || state.contentBytes,
      elapsed: 0,
      generated_at: new Date().toLocaleString()
    });
    dom.statTokens.textContent = '—';
    dom.btnCopy.disabled = !state.cachedFullText;
    dom.progressFill.style.width = '100%';
    dom.percentText.textContent = '100%';
    showToast('Directory tree ready');
  } catch (error) {
    if (error.name !== 'AbortError') {
      setEditor(error.message || 'Unable to scan this project.');
      setOutputStatus('Error', 'error');
      showToast(error.message || 'Unable to scan project', { error: true });
    }
  } finally {
    if (state.operationController === controller) state.operationController = null;
    setBusy(false);
    hideProgressSoon();
  }
}

function startGeneration() {
  const path = cleanPath(dom.pathInput.value);
  if (!path) {
    showToast('Enter a project path first', { error: true });
    dom.pathInput.focus();
    return;
  }
  cancelActiveWork(false);
  resetContentState();
  setEditor('');
  setStats(null);
  setBusy(true);
  setOutputStatus('Generating', 'progress');
  showProgress('Preparing scan', 0);

  const mode = selectedMode();
  const source = new EventSource(
    'api/generate?path=' + encodeURIComponent(path) + '&mode=' + encodeURIComponent(mode)
  );
  state.activeSource = source;
  let finished = false;

  source.addEventListener('log', (event) => {
    const data = parseEvent(event);
    if (data.message) dom.logText.textContent = data.message;
  });

  source.addEventListener('progress', (event) => {
    const data = parseEvent(event);
    const percent = clampPercent(data.percent);
    dom.progressFill.style.width = percent + '%';
    dom.percentText.textContent = percent + '%';
    dom.logText.textContent = data.message || 'Generating';
    dom.speedText.textContent = Number(data.speed) > 0 ? formatNumber(data.speed) + ' files/s' : '';
  });

  source.addEventListener('complete', async (event) => {
    if (finished) return;
    finished = true;
    const result = parseEvent(event);
    source.close();
    if (state.activeSource === source) state.activeSource = null;
    setBusy(false);
    dom.progressFill.style.width = '100%';
    dom.percentText.textContent = '100%';
    dom.logText.textContent = 'Complete';
    dom.speedText.textContent = '';

    const fileName = result.file_name || result.message || '';
    setGeneratedFile(fileName);
    setStats(result);
    setOutputStatus(formatNumber(result.total) + ' files', 'success');
    dom.btnCopy.disabled = !fileName;

    if (mode === 'full' && fileName) {
      await loadPreview(fileName);
    } else {
      const warnings = Number(result.binary_files || 0) + Number(result.skipped_files || 0) + Number(result.unreadable_files || 0);
      const summary = [
        'Scan complete',
        '',
        formatNumber(result.total) + ' files · ' + formatNumber(result.lines) + ' lines',
        (result.token_mode === 'exact' ? '' : '~') + formatNumber(result.tokens) + ' tokens · ' + formatBytes(result.size),
        warnings > 0 ? formatNumber(warnings) + ' files skipped or represented as metadata' : '',
        '',
        'Load all or copy when you need the generated document.'
      ].filter((line, index, values) => line !== '' || values[index - 1] !== '');
      setEditor(summary.join('\n'));
      dom.btnLoad.hidden = !fileName;
    }

    await syncBookmarksAfterGenerate(path);
    hideProgressSoon();
    showToast('Generated ' + formatNumber(result.total) + ' files in ' + Number(result.elapsed || 0).toFixed(2) + 's');
  });

  source.addEventListener('error', (event) => {
    if (finished) return;
    finished = true;
    const data = event.data ? parseEvent(event) : {};
    source.close();
    if (state.activeSource === source) state.activeSource = null;
    setBusy(false);
    dom.statusPanel.hidden = true;
    setOutputStatus('Error', 'error');
    const message = data.message || 'Generation connection closed unexpectedly';
    setEditor(message);
    showToast(message, { error: true });
  });
}

async function loadPreview(fileName) {
  stopContentLoad();
  const controller = new AbortController();
  state.contentController = controller;
  try {
    const response = await fetch(
      'api/content?file=' + encodeURIComponent(fileName) + '&limit=' + previewBytes,
      { signal: controller.signal, cache: 'no-store' }
    );
    if (!response.ok) throw new Error(await response.text() || 'Unable to load preview');
    const text = await response.text();
    if (state.contentController !== controller) return;
    const total = Number(response.headers.get('X-Content-Size')) || new Blob([text]).size;
    const truncated = response.headers.get('X-Content-Truncated') === 'true';
    state.contentBytes = total;
    state.previewTruncated = truncated;
    state.cachedFullText = truncated ? null : text;
    setEditor(text);
    dom.btnCopy.disabled = false;
    dom.btnLoad.hidden = !truncated;
    dom.virtualBanner.hidden = !truncated;
    if (truncated) {
      dom.vBannerText.textContent = 'Showing ' + formatBytes(new Blob([text]).size) + ' of ' + formatBytes(total);
      dom.btnRenderAll.hidden = false;
      dom.btnStopRender.hidden = true;
    }
  } catch (error) {
    if (error.name !== 'AbortError') {
      dom.btnLoad.hidden = false;
      showToast(error.message || 'Unable to load preview', { error: true });
    }
  } finally {
    if (state.contentController === controller) state.contentController = null;
  }
}

async function loadFullContent() {
  if (!state.lastGeneratedFile) {
    showToast('No generated document is available', { error: true });
    return;
  }
  if (state.cachedFullText != null && !state.previewTruncated) {
    setEditor(state.cachedFullText);
    dom.virtualBanner.hidden = true;
    dom.btnLoad.hidden = true;
    return;
  }
  stopContentLoad();
  const controller = new AbortController();
  state.contentController = controller;
  dom.btnLoad.disabled = true;
  dom.btnLoad.textContent = 'Loading…';
  dom.virtualBanner.hidden = false;
  dom.vBannerText.textContent = 'Loading the complete document';
  dom.btnRenderAll.hidden = true;
  dom.btnStopRender.hidden = false;

  try {
    const text = await fetchFullText(state.lastGeneratedFile, controller.signal);
    if (state.contentController !== controller) return;
    state.cachedFullText = text;
    state.previewTruncated = false;
    state.contentBytes = new Blob([text]).size;
    setEditor(text);
    dom.virtualBanner.hidden = true;
    dom.btnLoad.hidden = true;
    dom.btnCopy.disabled = false;
    showToast('Full document loaded · ' + formatBytes(state.contentBytes));
  } catch (error) {
    if (error.name !== 'AbortError') showToast(error.message || 'Unable to load document', { error: true });
  } finally {
    if (state.contentController === controller) state.contentController = null;
    dom.btnLoad.disabled = false;
    dom.btnLoad.textContent = 'Load all';
    dom.btnStopRender.hidden = true;
    if (state.previewTruncated) dom.btnRenderAll.hidden = false;
  }
}

function stopContentLoad() {
  if (state.contentController) {
    state.contentController.abort();
    state.contentController = null;
  }
  dom.btnStopRender.hidden = true;
  if (state.previewTruncated) dom.btnRenderAll.hidden = false;
}

async function copyCurrentContent() {
  dom.btnCopy.disabled = true;
  const originalLabel = dom.btnCopy.textContent;
  dom.btnCopy.textContent = 'Copying…';
  try {
    let text = state.cachedFullText;
    if (text == null && state.lastGeneratedFile) {
      text = await fetchFullText(state.lastGeneratedFile);
      state.cachedFullText = text;
      state.previewTruncated = false;
      state.contentBytes = new Blob([text]).size;
    }
    if (text == null) text = dom.editor.value;
    if (!text) throw new Error('Nothing to copy');
    await copyToClipboard(text);
  } catch (error) {
    if (error.name !== 'AbortError') showToast(error.message || 'Unable to copy document', { error: true });
  } finally {
    dom.btnCopy.disabled = false;
    dom.btnCopy.textContent = originalLabel;
  }
}

async function fetchFullText(fileName, signal) {
  const response = await fetch('api/content?file=' + encodeURIComponent(fileName), {
    signal,
    cache: 'no-store'
  });
  if (!response.ok) throw new Error(await response.text() || 'Generated file is unavailable');
  return response.text();
}

function showProgress(message, percent) {
  dom.statusPanel.hidden = false;
  dom.logText.textContent = message;
  dom.progressFill.style.width = clampPercent(percent) + '%';
  dom.percentText.textContent = clampPercent(percent) + '%';
  dom.speedText.textContent = '';
}

function hideProgressSoon() {
  setTimeout(() => {
    if (!state.activeSource && !state.operationController) dom.statusPanel.hidden = true;
  }, 650);
}

function parseEvent(event) {
  try {
    return JSON.parse(event.data || '{}');
  } catch {
    return {};
  }
}

function clampPercent(value) {
  return Math.max(0, Math.min(100, Math.round(Number(value) || 0)));
}
