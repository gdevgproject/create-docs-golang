import {
  dom,
  formatNumber,
  openInfoModal,
  requestJSON,
  showToast,
  state
} from './core.js';

export function initTools() {
  dom.btnCounter.addEventListener('click', openTokenCounter);
  dom.btnExclusions.addEventListener('click', openExclusions);
}

function openTokenCounter() {
  const layout = document.createElement('div');
  layout.className = 'counter-layout';
  const stats = document.createElement('div');
  stats.className = 'counter-stats';
  const tokens = counterStat('Tokens');
  const characters = counterStat('Characters');
  const lines = counterStat('Lines');
  const context = counterStat('128K context');
  stats.append(tokens.root, characters.root, lines.root, context.root);

  const tools = document.createElement('div');
  tools.className = 'counter-tools';
  const mode = document.createElement('span');
  mode.className = 'section-hint';
  mode.textContent = 'o200k_base';
  const actions = document.createElement('div');
  actions.className = 'inline-tools';
  const paste = smallButton('Paste');
  const clear = smallButton('Clear');
  actions.append(paste, clear);
  tools.append(mode, actions);

  const input = document.createElement('textarea');
  input.className = 'counter-input';
  input.placeholder = 'Paste text or code';
  input.spellcheck = false;
  layout.append(stats, tools, input);
  openInfoModal('Token counter', layout);

  let timer = null;
  const reset = () => {
    tokens.value.textContent = '0';
    characters.value.textContent = '0';
    lines.value.textContent = '0';
    context.value.textContent = '0.0%';
  };
  const count = async () => {
    const text = input.value;
    if (!text) {
      reset();
      return;
    }
    if (new Blob([text]).size > 7.5 * 1024 * 1024) {
      showToast('Token input is too large', { error: true });
      return;
    }
    if (state.tokenController) state.tokenController.abort();
    const controller = new AbortController();
    state.tokenController = controller;
    try {
      const result = await requestJSON('api/count-tokens', {
        method: 'POST',
        signal: controller.signal,
        body: JSON.stringify({ text })
      });
      if (state.tokenController !== controller) return;
      tokens.value.textContent = (result.token_mode === 'exact' ? '' : '~') + formatNumber(result.tokens);
      characters.value.textContent = formatNumber(result.characters);
      lines.value.textContent = formatNumber(result.lines);
      context.value.textContent = ((Number(result.tokens) || 0) / 1280).toFixed(1) + '%';
      mode.textContent = result.token_mode === 'exact' ? 'o200k_base · exact' : 'o200k_base · estimate';
    } catch (error) {
      if (error.name !== 'AbortError') showToast(error.message || 'Unable to count tokens', { error: true });
    } finally {
      if (state.tokenController === controller) state.tokenController = null;
    }
  };

  input.addEventListener('input', () => {
    clearTimeout(timer);
    timer = setTimeout(count, 220);
  });
  paste.addEventListener('click', async () => {
    try {
      input.value = await navigator.clipboard.readText();
      await count();
    } catch {
      showToast('Clipboard access was denied', { error: true });
    }
  });
  clear.addEventListener('click', () => {
    input.value = '';
    reset();
    input.focus();
  });
  queueMicrotask(() => input.focus());
}

async function openExclusions() {
  const loading = document.createElement('div');
  loading.className = 'empty-mini';
  loading.textContent = 'Loading…';
  openInfoModal('Ignored paths', loading);
  try {
    const data = await requestJSON('api/exclusions');
    const content = document.createElement('div');
    content.append(
      tagSection('Directories', data.dirs || [], 'directory'),
      tagSection('Files', data.files || [], 'file'),
      tagSection('Binary extensions', (data.extensions || []).map((value) => '.' + value), 'binary')
    );
    if (loading.isConnected) loading.replaceWith(content);
  } catch (error) {
    if (loading.isConnected) loading.textContent = error.message || 'Unable to load ignored paths';
  }
}

function counterStat(label) {
  const root = document.createElement('div');
  root.className = 'counter-stat';
  const caption = document.createElement('span');
  caption.textContent = label;
  const value = document.createElement('strong');
  value.textContent = '0';
  root.append(caption, value);
  return { root, value };
}

function smallButton(label) {
  const button = document.createElement('button');
  button.type = 'button';
  button.className = 'text-button';
  button.textContent = label;
  return button;
}

function tagSection(title, values, className) {
  const section = document.createElement('section');
  section.className = 'tag-section';
  const heading = document.createElement('h3');
  heading.textContent = title + ' · ' + values.length;
  const grid = document.createElement('div');
  grid.className = 'tag-grid';
  values.forEach((value) => {
    const tag = document.createElement('span');
    tag.className = 'tag ' + className;
    tag.textContent = value;
    grid.appendChild(tag);
  });
  section.append(heading, grid);
  return section;
}
