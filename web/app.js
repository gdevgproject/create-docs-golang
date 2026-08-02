'use strict';

document.addEventListener('DOMContentLoaded', () => {
  const dom = {
    pathInput: document.getElementById('path-input'),
    bmNote: document.getElementById('bm-note'),
    bmList: document.getElementById('bm-list'),
    bmCount: document.getElementById('bm-count'),
    btnAddBm: document.getElementById('btn-add-bm'),
    btnStruct: document.getElementById('btn-struct'),
    btnGen: document.getElementById('btn-gen'),
    btnExclusions: document.getElementById('btn-exclusions'),
    btnCounter: document.getElementById('btn-counter'),
    editor: document.getElementById('editor'),
    fileBadge: document.getElementById('file-badge'),
    statsCards: document.getElementById('stats-cards'),
    statFiles: document.getElementById('stat-files'),
    statLines: document.getElementById('stat-lines'),
    statTokens: document.getElementById('stat-tokens'),
    cardTokens: document.getElementById('card-tokens'),
    statSize: document.getElementById('stat-size'),
    statTime: document.getElementById('stat-time'),
    statDate: document.getElementById('stat-date'),
    btnLoad: document.getElementById('btn-load'),
    btnCopy: document.getElementById('btn-copy'),
    btnDownload: document.getElementById('btn-download'),
    statusPanel: document.getElementById('status-panel'),
    progressFill: document.getElementById('progress-fill'),
    percentText: document.getElementById('percent-text'),
    speedText: document.getElementById('speed-text'),
    logText: document.getElementById('log-text'),
    toast: document.getElementById('toast'),
    modal: document.getElementById('modal-dialog'),
    modalTitle: document.getElementById('modal-title'),
    modalContent: document.getElementById('modal-content'),
    btnCopyModal: document.getElementById('btn-copy-modal'),
    btnCloseModal: document.getElementById('btn-close-modal'),
    virtualBanner: document.getElementById('virtual-banner'),
    vBannerText: document.getElementById('v-banner-text'),
    btnRenderAll: document.getElementById('btn-render-all'),
    btnStopRender: document.getElementById('btn-stop-render'),
    appVersion: document.getElementById('app-version'),
    verInfo: document.getElementById('ver-info'),
    btnCheckUpdate: document.getElementById('btn-check-update'),
    btnManualCheck: document.getElementById('btn-manual-check'),
  };

  let toastTimer = null;
  let lastGeneratedFile = null;
  let cachedFullText = null;
  let contentLoaded = false;
  let isFullRendered = false;
  let cachedUpdateInfo = null;
  let progressPollTimer = null;
  let updateState = 'idle'; // 'idle', 'downloading', 'ready'
  let targetVerStr = '';
  let isRenderingActive = false;
  let abortRendering = false;

  // Initialize
  loadVersion();
  loadBookmarks();
  silentCheckUpdate(false);

  // Event Listeners
  dom.btnAddBm.addEventListener('click', addBookmark);
  dom.btnStruct.addEventListener('click', getStructure);
  dom.btnGen.addEventListener('click', startGenerate);
  dom.btnExclusions.addEventListener('click', showExclusions);
  if (dom.btnCounter) dom.btnCounter.addEventListener('click', showTokenCounterModal);
  dom.btnLoad.addEventListener('click', loadFullContent);
  dom.btnCopy.addEventListener('click', copyContent);
  dom.btnCopyModal.addEventListener('click', copyModalContent);
  dom.btnCloseModal.addEventListener('click', closeModal);
  dom.btnRenderAll.addEventListener('click', renderRemainingTextAsync);
  if (dom.btnStopRender) dom.btnStopRender.addEventListener('click', stopRendering);
  if (dom.btnCheckUpdate) dom.btnCheckUpdate.addEventListener('click', handleUpdateButtonClick);
  if (dom.btnManualCheck) dom.btnManualCheck.addEventListener('click', () => silentCheckUpdate(true));

  window.addEventListener('click', (e) => {
    if (e.target === dom.modal) closeModal();
  });

  // Heartbeat ping to keep backend alive while window is open
  function sendPing() {
    fetch('api/ping').catch(() => {});
  }
  sendPing();
  setInterval(sendPing, 3000);

  function getSelectedMode() {
    const el = document.querySelector('input[name="gen-mode"]:checked');
    return el ? el.value : 'stats';
  }

  function cleanPath(str) {
    return (str || '').replace(/^["'\s]+|["'\s]+$/g, '').trim();
  }

  function showToast(msg, duration = 2400) {
    if (toastTimer) clearTimeout(toastTimer);
    dom.toast.textContent = msg;
    dom.toast.classList.add('show');
    toastTimer = setTimeout(() => dom.toast.classList.remove('show'), duration);
  }

  function formatBytes(bytes) {
    if (bytes < 1024) return bytes + ' B';
    if (bytes < 1048576) return (bytes / 1024).toFixed(1) + ' KB';
    return (bytes / 1048576).toFixed(2) + ' MB';
  }

  function escHtml(str) {
    return String(str)
      .replace(/&/g, '&amp;')
      .replace(/</g, '&lt;')
      .replace(/>/g, '&gt;')
      .replace(/"/g, '&quot;');
  }

  // ── QUICK TOKEN COUNTER ──
  function showTokenCounterModal() {
    dom.modalTitle.textContent = '🧮 Instant Token Counter (o200k_base Tiktoken)';
    dom.btnCopyModal.style.display = 'none';

    dom.modalContent.innerHTML = `
      <div style="display:flex; flex-direction:column; gap:12px; height:100%;">
        <div style="display:flex; gap:10px; flex-wrap:wrap;">
          <div class="stat-card" style="flex:1; min-width:110px;">
            <span class="stat-lbl">TOKENS (EXACT)</span>
            <span class="stat-val" id="tc-tokens" style="font-size:1.1rem; color:var(--primary);">0</span>
          </div>
          <div class="stat-card" style="flex:1; min-width:110px;">
            <span class="stat-lbl">CHARACTERS</span>
            <span class="stat-val" id="tc-chars" style="font-size:1.1rem;">0</span>
          </div>
          <div class="stat-card" style="flex:1; min-width:110px;">
            <span class="stat-lbl">LINES</span>
            <span class="stat-val" id="tc-lines" style="font-size:1.1rem;">0</span>
          </div>
          <div class="stat-card" style="flex:1; min-width:110px;">
            <span class="stat-lbl">CONTEXT (128K)</span>
            <span class="stat-val" id="tc-context" style="font-size:1.1rem; color:var(--accent-emerald);">0.0%</span>
          </div>
        </div>

        <div style="display:flex; gap:8px;">
          <button class="action-btn primary sm" id="btn-tc-paste">📋 Paste Clipboard & Count</button>
          <button class="action-btn ghost sm" id="btn-tc-clear">🧹 Clear</button>
        </div>

        <textarea id="tc-input" class="code-editor" style="flex:1; height:320px; border:1px solid var(--border); border-radius:4px; padding:12px;" placeholder="Paste any text, prompt, XML output, or source code snippet here to calculate exact o200k_base tokens..."></textarea>
      </div>
    `;

    dom.modal.style.display = 'flex';

    const tcInput = document.getElementById('tc-input');
    const tcTokens = document.getElementById('tc-tokens');
    const tcChars = document.getElementById('tc-chars');
    const tcLines = document.getElementById('tc-lines');
    const tcContext = document.getElementById('tc-context');
    const btnPaste = document.getElementById('btn-tc-paste');
    const btnClear = document.getElementById('btn-tc-clear');

    let calcTimer = null;

    async function runCount() {
      const text = tcInput.value;
      if (!text) {
        tcTokens.textContent = '0';
        tcChars.textContent = '0';
        tcLines.textContent = '0';
        tcContext.textContent = '0.0%';
        return;
      }

      tcChars.textContent = text.length.toLocaleString();
      const lineCount = (text.match(/\n/g) || []).length + 1;
      tcLines.textContent = lineCount.toLocaleString();

      try {
        const res = await fetch('api/count-tokens', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ text }),
        });
        const d = await res.json();
        const tokens = d.tokens || 0;
        tcTokens.textContent = (d.token_mode === 'exact' ? '' : '~') + tokens.toLocaleString();
        
        const pct128 = ((tokens / 128000) * 100).toFixed(1);
        tcContext.textContent = pct128 + '%';
      } catch {
        // ignore
      }
    }

    tcInput.addEventListener('input', () => {
      if (calcTimer) clearTimeout(calcTimer);
      calcTimer = setTimeout(runCount, 150);
    });

    btnPaste.addEventListener('click', async () => {
      try {
        const clipboardText = await navigator.clipboard.readText();
        if (clipboardText) {
          tcInput.value = clipboardText;
          runCount();
          showToast('📋 Pasted & calculated tokens!');
        }
      } catch {
        showToast('⚠️ Please paste text into the box manually');
      }
    });

    btnClear.addEventListener('click', () => {
      tcInput.value = '';
      runCount();
    });

    tcInput.focus();
  }

  // ── SEAMLESS IN-PLACE BUTTON UPDATES ──
  async function loadVersion() {
    try {
      const res = await fetch('api/version');
      const data = await res.json();
      if (data.version) {
        let v = data.version;
        if (!v.startsWith('v')) v = 'v' + v;
        if (dom.appVersion) dom.appVersion.textContent = v;
        if (dom.verInfo) dom.verInfo.textContent = `CodePulse AI ${v}`;
      }
    } catch {
      // ignore
    }
  }

  // Check update with GitHub API cache buster & user-controlled manual trigger
  async function silentCheckUpdate(manual = false) {
    if (manual) {
      if (dom.btnManualCheck) {
        dom.btnManualCheck.disabled = true;
        dom.btnManualCheck.textContent = '⏳ Checking...';
      }
      showToast('🔍 Checking GitHub for updates...');
    }

    try {
      const res = await fetch(`api/check-update?t=${Date.now()}`);
      const info = await res.json();

      if (info.has_update) {
        cachedUpdateInfo = info;
        let ver = info.latest_version || '';
        if (ver.startsWith('v')) ver = ver.substring(1);
        targetVerStr = 'v' + ver;

        if (dom.btnManualCheck) dom.btnManualCheck.style.display = 'none';

        if (dom.btnCheckUpdate) {
          dom.btnCheckUpdate.textContent = `✨ Update ${targetVerStr}`;
          dom.btnCheckUpdate.className = 'btn-update-badge';
          dom.btnCheckUpdate.style.display = 'inline-block';
          updateState = 'idle';
        }
        if (manual) showToast(`✨ Update available: ${targetVerStr}`);
      } else {
        if (dom.btnCheckUpdate) dom.btnCheckUpdate.style.display = 'none';
        if (dom.btnManualCheck) {
          dom.btnManualCheck.style.display = 'inline-block';
          dom.btnManualCheck.disabled = false;
          dom.btnManualCheck.textContent = '🔄 Check Update';
        }
        if (manual) showToast(`✅ You are on the latest version (${info.current_version})`);
      }
    } catch {
      if (dom.btnManualCheck) {
        dom.btnManualCheck.disabled = false;
        dom.btnManualCheck.textContent = '🔄 Check Update';
      }
      if (manual) showToast('❌ Unable to check GitHub updates');
    }
  }

  function handleUpdateButtonClick() {
    if (updateState === 'idle') {
      if (cachedUpdateInfo && cachedUpdateInfo.download_url) {
        startInPlaceDownload(cachedUpdateInfo.download_url);
      }
    } else if (updateState === 'ready') {
      applyUpdate();
    }
  }

  async function startInPlaceDownload(downloadUrl) {
    updateState = 'downloading';
    if (dom.btnCheckUpdate) {
      dom.btnCheckUpdate.className = 'btn-update-badge downloading';
      dom.btnCheckUpdate.innerHTML = `<span class="spin-ring"></span> 0%`;
    }

    try {
      await fetch('api/download-update', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ download_url: downloadUrl }),
      });

      if (progressPollTimer) clearInterval(progressPollTimer);
      progressPollTimer = setInterval(pollInPlaceProgress, 400);
      showToast(`⬇ Downloading update ${targetVerStr} in background...`);
    } catch {
      showToast('❌ Failed to start update download');
      updateState = 'idle';
      if (dom.btnCheckUpdate) {
        dom.btnCheckUpdate.className = 'btn-update-badge';
        dom.btnCheckUpdate.textContent = `✨ Update ${targetVerStr}`;
      }
    }
  }

  async function pollInPlaceProgress() {
    try {
      const res = await fetch('api/update-progress');
      const p = await res.json();

      if (p.state === 'downloading') {
        if (dom.btnCheckUpdate) {
          dom.btnCheckUpdate.innerHTML = `<span class="spin-ring"></span> ${p.percent}%`;
        }
      } else if (p.state === 'ready') {
        if (progressPollTimer) clearInterval(progressPollTimer);
        updateState = 'ready';

        if (dom.btnCheckUpdate) {
          dom.btnCheckUpdate.className = 'btn-update-badge restart-ready';
          dom.btnCheckUpdate.textContent = `🚀 Restart ${targetVerStr}`;
        }
        showToast(`✅ Update ${targetVerStr} downloaded! Click button to restart.`);
      } else if (p.state === 'error') {
        if (progressPollTimer) clearInterval(progressPollTimer);
        updateState = 'idle';
        if (dom.btnCheckUpdate) {
          dom.btnCheckUpdate.className = 'btn-update-badge';
          dom.btnCheckUpdate.textContent = `✨ Update ${targetVerStr}`;
        }
        showToast('❌ Download error: ' + (p.error || 'Failed'));
      }
    } catch {
      // ignore
    }
  }

  async function applyUpdate() {
    if (dom.btnCheckUpdate) {
      dom.btnCheckUpdate.disabled = true;
      dom.btnCheckUpdate.textContent = '⏳ Restarting...';
    }

    try {
      await fetch('api/apply-update', { method: 'POST' });
      showToast(`🚀 Restarting application to apply ${targetVerStr}...`);
      setTimeout(() => {
        location.reload();
      }, 1500);
    } catch {
      showToast('🚀 Restarting...');
      setTimeout(() => location.reload(), 1500);
    }
  }

  // ── BOOKMARKS ──
  async function loadBookmarks() {
    try {
      const res = await fetch('api/bookmarks');
      const data = await res.json();
      const items = Object.values(data).sort((a, b) => (a.order ?? 0) - (b.order ?? 0) || b.created_at.localeCompare(a.created_at));

      dom.bmCount.textContent = items.length;

      if (items.length === 0) {
        dom.bmList.innerHTML = '<div class="bm-empty">No bookmarks saved yet</div>';
        return;
      }

      const frag = document.createDocumentFragment();
      items.forEach((bm, index) => {
        const div = document.createElement('div');
        div.className = 'bm-item';
        div.innerHTML = `
          <div class="bm-content">
            <div class="bm-name" title="Click edit icon to rename">${escHtml(bm.note || 'Project')}</div>
            <div class="bm-path" title="${escHtml(bm.path)}">${escHtml(bm.path)}</div>
          </div>
          <div class="bm-actions">
            <button class="bm-btn bm-move-up" title="Move Up" ${index === 0 ? 'disabled' : ''}>▲</button>
            <button class="bm-btn bm-move-down" title="Move Down" ${index === items.length - 1 ? 'disabled' : ''}>▼</button>
            <button class="bm-btn bm-rename" title="Rename bookmark">✏️</button>
            <button class="bm-btn bm-del" title="Delete bookmark">×</button>
          </div>
        `;

        div.addEventListener('click', (e) => {
          if (!e.target.closest('.bm-actions')) {
            dom.pathInput.value = bm.path;
            showToast(`Loaded bookmark: ${bm.note || bm.path}`);
          }
        });

        div.querySelector('.bm-move-up')?.addEventListener('click', (e) => {
          e.stopPropagation();
          moveBookmark(items, index, -1);
        });

        div.querySelector('.bm-move-down')?.addEventListener('click', (e) => {
          e.stopPropagation();
          moveBookmark(items, index, 1);
        });

        div.querySelector('.bm-rename')?.addEventListener('click', (e) => {
          e.stopPropagation();
          renameBookmark(bm);
        });

        div.querySelector('.bm-del').addEventListener('click', (e) => {
          e.stopPropagation();
          deleteBookmark(bm.id);
        });

        frag.appendChild(div);
      });

      dom.bmList.innerHTML = '';
      dom.bmList.appendChild(frag);
    } catch {
      dom.bmList.innerHTML = '<div class="bm-empty">Failed to load bookmarks</div>';
    }
  }

  async function moveBookmark(items, currentIndex, direction) {
    const targetIndex = currentIndex + direction;
    if (targetIndex < 0 || targetIndex >= items.length) return;

    const newItems = [...items];
    const [moved] = newItems.splice(currentIndex, 1);
    newItems.splice(targetIndex, 0, moved);

    const orderedIDs = newItems.map(x => x.id);

    try {
      const res = await fetch('api/bookmarks', {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ action: 'reorder', ordered_ids: orderedIDs }),
      });
      if (res.ok) {
        loadBookmarks();
      }
    } catch {
      showToast('❌ Error reordering bookmarks');
    }
  }

  async function renameBookmark(bm) {
    const newNote = prompt('Edit Bookmark Label:', bm.note || bm.path);
    if (newNote === null) return;
    const cleanNote = newNote.trim();

    try {
      const res = await fetch('api/bookmarks', {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ id: bm.id, note: cleanNote }),
      });
      if (res.ok) {
        loadBookmarks();
        showToast('✏️ Bookmark label updated!');
      }
    } catch {
      showToast('❌ Error renaming bookmark');
    }
  }

  async function addBookmark() {
    const path = cleanPath(dom.pathInput.value);
    if (!path) return showToast('⚠️ Please enter a project path first!');

    try {
      const res = await fetch('api/bookmarks', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          path: path,
          note: dom.bmNote.value.trim(),
        }),
      });

      if (res.ok) {
        dom.bmNote.value = '';
        loadBookmarks();
        showToast('🔖 Bookmark saved successfully!');
      }
    } catch {
      showToast('❌ Error saving bookmark');
    }
  }

  async function deleteBookmark(id) {
    try {
      const res = await fetch('api/bookmarks', {
        method: 'DELETE',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ id }),
      });
      if (res.ok) {
        loadBookmarks();
        showToast('Deleted bookmark');
      }
    } catch {
      showToast('❌ Error deleting bookmark');
    }
  }

  // ── STRUCTURE & EXCLUSIONS ──
  async function getStructure() {
    const path = cleanPath(dom.pathInput.value);
    if (!path) return showToast('⚠️ Please enter a project path!');

    dom.modalTitle.textContent = '🌳 Directory Tree Preview';
    dom.modalContent.textContent = 'Scanning project structure...';
    dom.btnCopyModal.style.display = '';
    dom.modal.style.display = 'flex';

    try {
      const res = await fetch(`api/structure?path=${encodeURIComponent(path)}`);
      const json = await res.json();
      if (json.status === 'success') {
        dom.modalContent.textContent = json.data;
      } else {
        dom.modalContent.textContent = 'Error: ' + json.message;
      }
    } catch {
      dom.modalContent.textContent = 'Error connecting to server.';
    }
  }

  async function showExclusions() {
    dom.modalTitle.textContent = '🚫 Ignored Settings & Extensions';
    dom.modalContent.innerHTML = 'Loading ignored lists...';
    dom.btnCopyModal.style.display = 'none';
    dom.modal.style.display = 'flex';

    try {
      const res = await fetch('api/exclusions');
      const d = await res.json();
      dom.modalContent.innerHTML = `
        <div class="ex-section">
          <div class="ex-title">📂 Excluded Directories (${d.dirs.length})</div>
          <div class="ex-tag-grid">${d.dirs.map(x => `<span class="ex-tag dir">${escHtml(x)}</span>`).join('')}</div>
        </div>
        <div class="ex-section">
          <div class="ex-title">📄 Excluded Files & Lockfiles (${d.files.length})</div>
          <div class="ex-tag-grid">${d.files.map(x => `<span class="ex-tag file">${escHtml(x)}</span>`).join('')}</div>
        </div>
        <div class="ex-section">
          <div class="ex-title">🎮 Binary & Asset Extensions (${d.extensions.length})</div>
          <div class="ex-tag-grid">${d.extensions.map(x => `<span class="ex-tag bin">.${escHtml(x)}</span>`).join('')}</div>
        </div>
      `;
    } catch {
      dom.modalContent.textContent = 'Error loading exclusion lists.';
    }
  }

  function copyModalContent() {
    navigator.clipboard.writeText(dom.modalContent.textContent);
    showToast('✅ Copied structure to clipboard!');
  }

  function closeModal() {
    dom.modal.style.display = 'none';
  }

  // ── GENERATE DOCS (SSE) ──
  function startGenerate() {
    const path = cleanPath(dom.pathInput.value);
    if (!path) return showToast('⚠️ Please enter a project path!');

    const mode = getSelectedMode();
    lastGeneratedFile = null;
    cachedFullText = null;
    contentLoaded = false;
    isFullRendered = false;
    abortRendering = false;
    isRenderingActive = false;

    dom.virtualBanner.style.display = 'none';
    dom.editor.value = '';
    dom.statsCards.style.display = 'none';
    dom.statusPanel.style.display = 'block';
    dom.btnGen.disabled = true;
    dom.btnCopy.disabled = true;
    dom.btnLoad.style.display = 'none';
    dom.btnDownload.setAttribute('disabled', '');
    dom.fileBadge.className = 'status-badge badge-progress';
    dom.fileBadge.textContent = 'Generating...';
    dom.progressFill.style.width = '0%';
    dom.percentText.textContent = '0%';
    dom.speedText.textContent = '';

    const es = new EventSource(`api/generate?path=${encodeURIComponent(path)}&mode=${mode}`);

    es.addEventListener('log', e => {
      dom.logText.textContent = JSON.parse(e.data).message;
    });

    es.addEventListener('progress', e => {
      const d = JSON.parse(e.data);
      dom.progressFill.style.width = d.percent + '%';
      dom.percentText.textContent = d.percent + '%';
      dom.logText.textContent = d.message;
      if (d.speed > 0) dom.speedText.textContent = d.speed + ' files/s';
    });

    es.addEventListener('complete', e => {
      const d = JSON.parse(e.data);
      dom.progressFill.style.width = '100%';
      dom.percentText.textContent = '100%';
      dom.speedText.textContent = '';

      lastGeneratedFile = d.message;

      dom.fileBadge.className = 'status-badge badge-success';
      dom.fileBadge.textContent = d.total + ' files';
      dom.statsCards.style.display = 'flex';
      dom.statFiles.textContent = (d.total || 0).toLocaleString();
      dom.statLines.textContent = (d.lines || 0).toLocaleString();
      dom.statTokens.textContent = (d.token_mode === 'exact' ? '' : '~') + (d.tokens || 0).toLocaleString();
      dom.cardTokens.title = d.token_mode === 'exact'
        ? 'Exact token count using official o200k_base tiktoken BPE'
        : 'Estimated token count (~10% variance)';
      dom.statSize.textContent = formatBytes(d.size || 0);
      dom.statTime.textContent = (d.elapsed || 0) + 's';
      dom.statDate.textContent = d.generated_at || new Date().toLocaleString();

      dom.btnDownload.href = `api/download?file=${d.message}`;
      dom.btnDownload.removeAttribute('disabled');

      if (mode === 'stats') {
        dom.editor.value = `📊 "Stats Only" Mode — Thống kê đã sẵn sàng.\n\nFile output đã được ghi tại server.\nBấm nút "📄 Load Content" (hoặc "📋 Copy Docs") để tải nội dung chi tiết.`;
        dom.btnLoad.style.display = '';
        dom.btnCopy.disabled = false;
        dom.logText.textContent = '✅ Generated successfully (Stats Only mode)!';
        es.close();
        dom.btnGen.disabled = false;
        setTimeout(() => dom.statusPanel.style.display = 'none', 1000);
        showToast(`✅ Generated ${d.total} files in ${d.elapsed}s`);
      } else {
        dom.logText.textContent = '✅ Completed! Loading output...';
        fetchAndDisplayContent(d.message);
        es.close();
        dom.btnGen.disabled = false;
      }
    });

    es.addEventListener('error', e => {
      try {
        const d = JSON.parse(e.data);
        showToast('❌ Error: ' + d.message);
      } catch {
        showToast('❌ Error generating documentation');
      }
      es.close();
      dom.btnGen.disabled = false;
      dom.statusPanel.style.display = 'none';
      dom.fileBadge.className = 'status-badge';
      dom.fileBadge.textContent = 'Error';
    });

    es.onerror = () => {
      es.close();
      dom.btnGen.disabled = false;
      dom.statusPanel.style.display = 'none';
    };
  }

  // ── HIGH PERFORMANCE NON-BLOCKING CONTENT DISPLAY ──
  async function fetchAndDisplayContent(filename) {
    try {
      const res = await fetch(`api/content?file=${filename}`);
      const text = await res.text();
      cachedFullText = text;
      contentLoaded = true;
      dom.btnCopy.disabled = false;
      dom.btnLoad.style.display = 'none';

      const PREVIEW_LIMIT = 300 * 1024; // 300KB Fast Preview Threshold

      if (text.length <= PREVIEW_LIMIT) {
        dom.virtualBanner.style.display = 'none';
        dom.editor.value = text;
        isFullRendered = true;
      } else {
        const initialSlice = text.slice(0, PREVIEW_LIMIT);
        dom.editor.value = initialSlice + `\n\n... [⚡ FAST PREVIEW MODE: Showing initial 300 KB of ${formatBytes(text.length)}. Click "Load All Lines" above to render remaining content] ...`;
        
        dom.vBannerText.textContent = `⚡ Fast Preview Mode: Showing initial 300 KB of ${formatBytes(text.length)} (0ms UI lag). Click Copy Docs anytime for 100% full text!`;
        dom.virtualBanner.style.display = 'flex';
        dom.btnRenderAll.style.display = '';
        dom.btnRenderAll.disabled = false;
        dom.btnRenderAll.textContent = '📄 Load All Lines (Async)';
        if (dom.btnStopRender) dom.btnStopRender.style.display = 'none';
        isFullRendered = false;
      }

      setTimeout(() => dom.statusPanel.style.display = 'none', 1000);
      showToast(`✅ Content loaded (${formatBytes(text.length)})`);
    } catch {
      showToast('❌ Error loading content');
    }
  }

  function stopRendering() {
    if (isRenderingActive) {
      abortRendering = true;
      isRenderingActive = false;
      if (dom.btnStopRender) dom.btnStopRender.style.display = 'none';
      if (dom.btnRenderAll) {
        dom.btnRenderAll.disabled = false;
        dom.btnRenderAll.style.display = '';
        dom.btnRenderAll.textContent = '📄 Resume / Render All Lines';
      }
      showToast('⏸ Rendering paused. Click "Copy Docs" to get full text directly!');
    }
  }

  function renderRemainingTextAsync() {
    if (!cachedFullText || isFullRendered) return;

    isRenderingActive = true;
    abortRendering = false;

    if (dom.btnRenderAll) dom.btnRenderAll.style.display = 'none';
    if (dom.btnStopRender) dom.btnStopRender.style.display = 'inline-block';

    const fullText = cachedFullText;
    const totalLength = fullText.length;
    const chunkSize = 500 * 1024; // 500KB fast chunk size
    let offset = dom.editor.value.indexOf('\n\n... [⚡ FAST PREVIEW MODE');
    if (offset < 0) offset = 300 * 1024;

    dom.editor.value = fullText.slice(0, offset);

    function step() {
      if (abortRendering) {
        isRenderingActive = false;
        return;
      }

      if (offset >= totalLength) {
        dom.editor.value = fullText;
        isFullRendered = true;
        isRenderingActive = false;
        dom.virtualBanner.style.display = 'none';
        if (dom.btnStopRender) dom.btnStopRender.style.display = 'none';
        showToast('✅ Full document rendered successfully!');
        return;
      }

      const nextOffset = Math.min(offset + chunkSize, totalLength);
      dom.editor.value += fullText.slice(offset, nextOffset);
      offset = nextOffset;

      const pct = Math.round((offset / totalLength) * 100);
      dom.vBannerText.textContent = `⚡ Rendering Full Text: ${pct}% (${formatBytes(offset)} / ${formatBytes(totalLength)})...`;

      requestAnimationFrame(() => {
        setTimeout(step, 0);
      });
    }

    step();
  }

  async function loadFullContent() {
    if (!lastGeneratedFile) return showToast('⚠️ No file generated yet.');
    if (contentLoaded && isFullRendered) return;

    dom.btnLoad.disabled = true;
    dom.btnLoad.textContent = '⏳ Loading...';
    await fetchAndDisplayContent(lastGeneratedFile);
    dom.btnLoad.disabled = false;
    dom.btnLoad.textContent = '📄 Load Content';
  }

  async function copyContent() {
    let textToCopy = cachedFullText || dom.editor.value;

    if (!cachedFullText && lastGeneratedFile) {
      try {
        const res = await fetch(`api/content?file=${lastGeneratedFile}`);
        textToCopy = await res.text();
        cachedFullText = textToCopy;
      } catch {
        return showToast('❌ Error fetching content for copy');
      }
    }

    if (!textToCopy) return;

    try {
      await navigator.clipboard.writeText(textToCopy);
      showToast(`✅ Copied full docs (${formatBytes(textToCopy.length)}) to clipboard!`);
    } catch {
      dom.editor.value = textToCopy;
      dom.editor.select();
      document.execCommand('copy');
      showToast('✅ Copied to clipboard!');
    }
  }
});
