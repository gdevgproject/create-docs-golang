import {
  cleanPath,
  closeCompactPanels,
  confirmDialog,
  dom,
  formatBytes,
  formatNumber,
  formatTime,
  getPreferences,
  jsonBody,
  normalizedPath,
  promptDialog,
  requestJSON,
  resetContentState,
  savePreferences,
  setEditor,
  setGeneratedFile,
  setOutputStatus,
  setStats,
  showToast,
  state
} from './core.js';

export function initBookmarks() {
  state.activeBookmarkId = getPreferences().activeBookmarkId || null;
  dom.btnAddBm.addEventListener('click', saveCurrentProject);
  dom.bmNote.addEventListener('keydown', (event) => {
    if (event.key === 'Enter') {
      event.preventDefault();
      saveCurrentProject();
    }
  });
  dom.bmList.addEventListener('click', handleBookmarkClick);
  dom.hpTimeline.addEventListener('click', handleHistoryClick);
  dom.btnHpClear.addEventListener('click', clearActiveHistory);
  dom.btnExportBm.addEventListener('click', exportBookmarks);
  dom.btnImportBm.addEventListener('click', () => dom.importFileInput.click());
  dom.importFileInput.addEventListener('change', () => {
    const file = dom.importFileInput.files && dom.importFileInput.files[0];
    importBookmarks(file).finally(() => {
      dom.importFileInput.value = '';
    });
  });
  return loadBookmarks();
}

export async function loadBookmarks() {
  try {
    const data = await requestJSON('api/bookmarks');
    state.allBookmarks = data && typeof data === 'object' ? data : {};
    if (state.activeBookmarkId && !state.allBookmarks[state.activeBookmarkId]) {
      state.activeBookmarkId = null;
      savePreferences({ activeBookmarkId: null });
    }
    renderBookmarkList();
    renderHistory(activeBookmark());
    return state.allBookmarks;
  } catch (error) {
    dom.bmList.replaceChildren(emptyElement('Unable to load saved projects'));
    showToast(error.message || 'Unable to load saved projects', { error: true });
    return state.allBookmarks;
  }
}

export async function syncBookmarksAfterGenerate(projectPath) {
  await loadBookmarks();
  const target = normalizedPath(projectPath);
  const match = sortedBookmarks().find((bookmark) => normalizedPath(bookmark.path) === target);
  if (!match) return null;
  state.activeBookmarkId = match.id;
  savePreferences({ activeBookmarkId: match.id });
  renderBookmarkList();
  renderHistory(match);
  dom.hpTimeline.scrollTop = 0;
  return match;
}

function renderBookmarkList() {
  const bookmarks = sortedBookmarks();
  dom.bmCount.textContent = String(bookmarks.length);
  if (bookmarks.length === 0) {
    dom.bmList.replaceChildren(emptyElement('No saved projects'));
    return;
  }

  const fragment = document.createDocumentFragment();
  bookmarks.forEach((bookmark, index) => {
    const card = document.createElement('div');
    card.className = 'bookmark-card' + (bookmark.id === state.activeBookmarkId ? ' selected' : '');
    card.dataset.id = bookmark.id;

    const copy = document.createElement('button');
    copy.type = 'button';
    copy.className = 'bookmark-copy';
    copy.setAttribute('aria-label', 'Open ' + (bookmark.note || projectName(bookmark.path)));
    const nameRow = document.createElement('div');
    nameRow.className = 'bookmark-name-row';
    const name = document.createElement('span');
    name.className = 'bookmark-name';
    name.textContent = bookmark.note || projectName(bookmark.path);
    name.title = bookmark.note || bookmark.path;
    nameRow.appendChild(name);

    const historyCount = bookmark.history?.length || (bookmark.last_result ? 1 : 0);
    if (historyCount > 0) {
      const count = document.createElement('span');
      count.className = 'history-count';
      count.textContent = String(historyCount);
      count.title = historyCount + ' saved scans';
      nameRow.appendChild(count);
    }

    const path = document.createElement('div');
    path.className = 'bookmark-path';
    path.textContent = bookmark.path;
    path.title = bookmark.path;
    copy.append(nameRow, path);

    const actions = document.createElement('div');
    actions.className = 'bookmark-actions';
    actions.append(
      bookmarkButton('up', '↑', 'Move up', index === 0),
      bookmarkButton('down', '↓', 'Move down', index === bookmarks.length - 1),
      bookmarkButton('rename', '✎', 'Rename'),
      bookmarkButton('delete', '×', 'Delete')
    );
    card.append(copy, actions);
    fragment.appendChild(card);
  });
  dom.bmList.replaceChildren(fragment);
}

function renderHistory(bookmark) {
  if (!bookmark) {
    dom.hpBmName.textContent = 'History';
    dom.btnHpClear.hidden = true;
    dom.hpTimeline.replaceChildren(emptyElement('Select a saved project'));
    return;
  }
  dom.hpBmName.textContent = bookmark.note || projectName(bookmark.path);
  dom.hpBmName.title = bookmark.path;
  const history = bookmark.history?.length ? bookmark.history : (bookmark.last_result ? [bookmark.last_result] : []);
  dom.btnHpClear.hidden = history.length === 0;
  if (history.length === 0) {
    dom.hpTimeline.replaceChildren(emptyElement('No scans yet'));
    return;
  }

  const fragment = document.createDocumentFragment();
  history.forEach((result, index) => {
    const card = document.createElement('article');
    card.className = 'history-card';
    card.dataset.index = String(index);

    const head = document.createElement('div');
    head.className = 'history-card-head';
    const titleWrap = document.createElement('div');
    titleWrap.style.minWidth = '0';
    const title = document.createElement('div');
    title.className = 'history-title' + (result.label ? ' labelled' : '');
    title.textContent = result.label || formatTime(result.generated_at);
    title.title = result.label || result.generated_at || '';
    titleWrap.appendChild(title);
    if (result.label) {
      const time = document.createElement('div');
      time.className = 'history-time';
      time.textContent = formatTime(result.generated_at);
      titleWrap.appendChild(time);
    }

    const actions = document.createElement('div');
    actions.className = 'history-actions';
    actions.append(
      historyButton('rename', 'Name'),
      historyButton('restore', 'Open'),
      historyButton('delete', '×')
    );
    head.append(titleWrap, actions);

    const metrics = document.createElement('div');
    metrics.className = 'history-metrics';
    metrics.append(
      metric(formatNumber(result.total) + ' files'),
      metric(formatNumber(result.lines) + ' lines'),
      metric((result.token_mode === 'exact' ? '' : '~') + formatNumber(result.tokens) + ' tok'),
      metric(formatBytes(result.size)),
      metric(Number(result.elapsed || 0).toFixed(2).replace(/\.?0+$/, '') + 's')
    );
    card.append(head, metrics);
    fragment.appendChild(card);
  });
  dom.hpTimeline.replaceChildren(fragment);
}

async function handleBookmarkClick(event) {
  const card = event.target.closest('.bookmark-card');
  if (!card) return;
  const bookmark = state.allBookmarks[card.dataset.id];
  if (!bookmark) return;
  const actionButton = event.target.closest('[data-action]');
  if (!actionButton) {
    selectBookmark(bookmark, true);
    return;
  }
  event.stopPropagation();
  const bookmarks = sortedBookmarks();
  const index = bookmarks.findIndex((item) => item.id === bookmark.id);
  switch (actionButton.dataset.action) {
    case 'up':
      await moveBookmark(bookmarks, index, -1);
      break;
    case 'down':
      await moveBookmark(bookmarks, index, 1);
      break;
    case 'rename':
      await renameBookmark(bookmark);
      break;
    case 'delete':
      await deleteBookmark(bookmark);
      break;
  }
}

async function handleHistoryClick(event) {
  const button = event.target.closest('[data-history-action]');
  const card = event.target.closest('.history-card');
  const bookmark = activeBookmark();
  if (!button || !card || !bookmark) return;
  const history = bookmark.history?.length ? bookmark.history : (bookmark.last_result ? [bookmark.last_result] : []);
  const result = history[Number(card.dataset.index)];
  if (!result) return;
  switch (button.dataset.historyAction) {
    case 'restore':
      restoreResult(bookmark, result);
      break;
    case 'rename':
      await renameHistory(bookmark, result);
      break;
    case 'delete':
      await deleteHistory(bookmark, result);
      break;
  }
}

function selectBookmark(bookmark, restore) {
  state.activeBookmarkId = bookmark.id;
  dom.pathInput.value = bookmark.path;
  savePreferences({ activeBookmarkId: bookmark.id, path: bookmark.path });
  renderBookmarkList();
  renderHistory(bookmark);
  if (restore) {
    const result = bookmark.last_result || bookmark.history?.[0];
    if (result) restoreResult(bookmark, result);
    else showToast('Project selected');
  }
  closeCompactPanels();
}

function restoreResult(bookmark, result) {
  resetContentState();
  setStats(result);
  setOutputStatus(formatNumber(result.total) + ' files', 'success');
  setGeneratedFile(result.file_name || '');
  dom.btnLoad.hidden = !result.file_name;
  dom.btnCopy.disabled = !result.file_name;
  const lines = [
    bookmark.note || projectName(bookmark.path),
    '',
    formatTime(result.generated_at),
    formatNumber(result.total) + ' files · ' + formatNumber(result.lines) + ' lines',
    (result.token_mode === 'exact' ? '' : '~') + formatNumber(result.tokens) + ' tokens · ' + formatBytes(result.size),
    '',
    result.file_name ? 'Load all or copy when you need the document.' : 'The generated file is no longer available.'
  ];
  setEditor(lines.join('\n'));
  showToast('Saved result restored');
}

async function saveCurrentProject() {
  const path = cleanPath(dom.pathInput.value);
  if (!path) {
    showToast('Enter a project path first', { error: true });
    dom.pathInput.focus();
    return;
  }
  try {
    const response = await requestJSON('api/bookmarks', {
      method: 'POST',
      body: jsonBody({ path, note: dom.bmNote.value.trim() })
    });
    state.allBookmarks = response.data || {};
    const match = Object.values(state.allBookmarks).find((item) => normalizedPath(item.path) === normalizedPath(path));
    if (match) {
      state.activeBookmarkId = match.id;
      savePreferences({ activeBookmarkId: match.id });
    }
    dom.bmNote.value = '';
    renderBookmarkList();
    renderHistory(activeBookmark());
    showToast('Project saved');
  } catch (error) {
    showToast(error.message || 'Unable to save project', { error: true });
  }
}

async function renameBookmark(bookmark) {
  const label = await promptDialog('Rename project', bookmark.note || projectName(bookmark.path));
  if (label == null) return;
  await mutateBookmarks('api/bookmarks', 'PUT', { id: bookmark.id, note: label }, 'Project renamed');
}

async function deleteBookmark(bookmark) {
  const confirmed = await confirmDialog(
    'Delete project',
    'Remove “' + (bookmark.note || projectName(bookmark.path)) + '” and its local history?',
    'Delete'
  );
  if (!confirmed) return;
  if (state.activeBookmarkId === bookmark.id) {
    state.activeBookmarkId = null;
    savePreferences({ activeBookmarkId: null });
  }
  await mutateBookmarks('api/bookmarks', 'DELETE', { id: bookmark.id }, 'Project deleted');
}

async function moveBookmark(bookmarks, index, direction) {
  const target = index + direction;
  if (index < 0 || target < 0 || target >= bookmarks.length) return;
  const reordered = [...bookmarks];
  const [moved] = reordered.splice(index, 1);
  reordered.splice(target, 0, moved);
  await mutateBookmarks('api/bookmarks', 'PUT', {
    action: 'reorder',
    ordered_ids: reordered.map((item) => item.id)
  });
}

async function renameHistory(bookmark, result) {
  const label = await promptDialog('Name this scan', result.label || '', formatTime(result.generated_at));
  if (label == null) return;
  await mutateBookmarks('api/bookmarks/history', 'PUT', {
    id: bookmark.id,
    generated_at: result.generated_at,
    label
  }, label ? 'Scan named' : 'Scan name cleared');
}

async function deleteHistory(bookmark, result) {
  const confirmed = await confirmDialog('Delete scan', 'Remove this scan from local history?', 'Delete');
  if (!confirmed) return;
  await mutateBookmarks('api/bookmarks/history', 'DELETE', {
    id: bookmark.id,
    generated_at: result.generated_at
  }, 'Scan deleted');
}

async function clearActiveHistory() {
  const bookmark = activeBookmark();
  if (!bookmark) return;
  const confirmed = await confirmDialog('Clear history', 'Remove every saved scan for this project?', 'Clear');
  if (!confirmed) return;
  await mutateBookmarks('api/bookmarks/history', 'DELETE', { id: bookmark.id }, 'History cleared');
}

async function mutateBookmarks(path, method, body, successMessage = '') {
  try {
    const response = await requestJSON(path, { method, body: jsonBody(body) });
    state.allBookmarks = response.data || {};
    renderBookmarkList();
    renderHistory(activeBookmark());
    if (successMessage) showToast(successMessage);
  } catch (error) {
    showToast(error.message || 'Unable to update saved projects', { error: true });
  }
}

function exportBookmarks() {
  const anchor = document.createElement('a');
  anchor.href = 'api/bookmarks/export';
  anchor.click();
  showToast('Backup export started');
}

async function importBookmarks(file) {
  if (!file) return;
  if (file.size > 32 * 1024 * 1024) {
    showToast('Backup is larger than 32 MB', { error: true });
    return;
  }
  try {
    const response = await requestJSON('api/bookmarks/import', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: await file.text()
    });
    state.allBookmarks = response.data || {};
    renderBookmarkList();
    renderHistory(activeBookmark());
    showToast('Backup imported and merged');
  } catch (error) {
    showToast(error.message || 'Unable to import backup', { error: true });
  }
}

function activeBookmark() {
  return state.activeBookmarkId ? state.allBookmarks[state.activeBookmarkId] || null : null;
}

function sortedBookmarks() {
  return Object.values(state.allBookmarks).sort((left, right) => {
    const order = (left.order ?? 0) - (right.order ?? 0);
    if (order !== 0) return order;
    return String(left.created_at || '').localeCompare(String(right.created_at || ''));
  });
}

function bookmarkButton(action, label, title, disabled = false) {
  const button = document.createElement('button');
  button.type = 'button';
  button.className = 'bookmark-action' + (action === 'delete' ? ' delete' : '');
  button.dataset.action = action;
  button.textContent = label;
  button.title = title;
  button.setAttribute('aria-label', title);
  button.disabled = disabled;
  return button;
}

function historyButton(action, label) {
  const button = document.createElement('button');
  button.type = 'button';
  button.className = 'history-action' + (action === 'delete' ? ' delete' : '');
  button.dataset.historyAction = action;
  button.textContent = label;
  return button;
}

function metric(text) {
  const element = document.createElement('span');
  element.className = 'history-metric';
  element.textContent = text;
  return element;
}

function emptyElement(text) {
  const element = document.createElement('div');
  element.className = 'empty-mini';
  element.textContent = text;
  return element;
}

function projectName(path) {
  const parts = String(path || '').replace(/\\/g, '/').split('/').filter(Boolean);
  return parts.at(-1) || 'Project';
}
