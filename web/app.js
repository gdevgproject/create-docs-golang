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
  };

  let toastTimer = null;
  let lastGeneratedFile = null;
  let cachedFullText = null;
  let contentLoaded = false;
  let isFullRendered = false;

  // Initialize Bookmarks
  loadBookmarks();

  // Event Listeners
  dom.btnAddBm.addEventListener('click', addBookmark);
  dom.btnStruct.addEventListener('click', getStructure);
  dom.btnGen.addEventListener('click', startGenerate);
  dom.btnExclusions.addEventListener('click', showExclusions);
  dom.btnLoad.addEventListener('click', loadFullContent);
  dom.btnCopy.addEventListener('click', copyContent);
  dom.btnCopyModal.addEventListener('click', copyModalContent);
  dom.btnCloseModal.addEventListener('click', closeModal);
  dom.btnRenderAll.addEventListener('click', renderRemainingTextAsync);

  window.addEventListener('click', (e) => {
    if (e.target === dom.modal) closeModal();
  });

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

  // ── BOOKMARKS ──
  async function loadBookmarks() {
    try {
      const res = await fetch('api/bookmarks');
      const data = await res.json();
      const items = Object.values(data).sort((a, b) => b.created_at.localeCompare(a.created_at));

      dom.bmCount.textContent = items.length;

      if (items.length === 0) {
        dom.bmList.innerHTML = '<div class="bm-empty">No bookmarks saved yet</div>';
        return;
      }

      const frag = document.createDocumentFragment();
      items.forEach(bm => {
        const div = document.createElement('div');
        div.className = 'bm-item';
        div.innerHTML = `
          <div class="bm-name">${escHtml(bm.note || 'Project')}</div>
          <div class="bm-path" title="${escHtml(bm.path)}">${escHtml(bm.path)}</div>
          <button class="bm-del" title="Delete bookmark">×</button>
        `;
        div.addEventListener('click', (e) => {
          if (!e.target.classList.contains('bm-del')) {
            dom.pathInput.value = bm.path;
            showToast(`Loaded bookmark: ${bm.note || bm.path}`);
          }
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
          <div class="ex-title">Excluded Directories</div>
          <div>${d.dirs.map(x => `<span class="ex-tag dir">${x}</span>`).join('')}</div>
        </div>
        <div class="ex-section">
          <div class="ex-title">Excluded Files</div>
          <div>${d.files.map(x => `<span class="ex-tag file">${x}</span>`).join('')}</div>
        </div>
        <div class="ex-section">
          <div class="ex-title">Binary Extensions (Structure only, Content skipped)</div>
          <div>${d.extensions.map(x => `<span class="ex-tag bin">${x}</span>`).join('')}</div>
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
        // High-Performance Virtual Preview: Render initial 300KB slice immediately (0ms UI lag)
        const initialSlice = text.slice(0, PREVIEW_LIMIT);
        dom.editor.value = initialSlice + `\n\n... [⚡ FAST PREVIEW MODE: Showing initial 300 KB of ${formatBytes(text.length)}. Click "Load All Lines" above to render remaining content] ...`;
        
        dom.vBannerText.textContent = `⚡ High-Performance Fast Preview Mode: Loaded initial 300 KB of ${formatBytes(text.length)} (0ms UI lag).`;
        dom.virtualBanner.style.display = 'flex';
        dom.btnRenderAll.style.display = '';
        dom.btnRenderAll.disabled = false;
        dom.btnRenderAll.textContent = '📄 Load All Lines (Async)';
        isFullRendered = false;
      }

      setTimeout(() => dom.statusPanel.style.display = 'none', 1000);
      showToast(`✅ Content loaded (${formatBytes(text.length)})`);
    } catch {
      showToast('❌ Error loading content');
    }
  }

  function renderRemainingTextAsync() {
    if (!cachedFullText || isFullRendered) return;

    dom.btnRenderAll.disabled = true;
    dom.btnRenderAll.textContent = '⏳ Rendering...';

    const fullText = cachedFullText;
    const totalLength = fullText.length;
    const chunkSize = 250 * 1024; // 250KB per animation frame
    let offset = dom.editor.value.indexOf('\n\n... [⚡ FAST PREVIEW MODE');
    if (offset < 0) offset = 300 * 1024;

    dom.editor.value = fullText.slice(0, offset);

    function step() {
      if (offset >= totalLength) {
        dom.editor.value = fullText;
        isFullRendered = true;
        dom.virtualBanner.style.display = 'none';
        showToast('✅ Full document rendered with 0ms UI lag!');
        return;
      }

      const nextOffset = Math.min(offset + chunkSize, totalLength);
      dom.editor.value += fullText.slice(offset, nextOffset);
      offset = nextOffset;

      const pct = Math.round((offset / totalLength) * 100);
      dom.vBannerText.textContent = `⚡ Rendering Full Text: ${pct}% (${formatBytes(offset)} / ${formatBytes(totalLength)})...`;

      // Non-blocking UI frame dispatch
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
      // Zero-DOM Copying: Copies directly from memory buffer, 0ms UI lag!
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
