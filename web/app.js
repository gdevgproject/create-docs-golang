import { dom, initCore, setEditor, setOutputStatus } from './js/core.js';
import { initBookmarks } from './js/bookmarks.js';
import { initGenerator } from './js/generator.js';
import { initTools } from './js/tools.js';
import { initUpdater } from './js/updater.js';

async function bootstrap() {
  try {
    initCore();
    initGenerator();
    initTools();
    initUpdater();
    await initBookmarks();
  } catch (error) {
    console.error('CodeDocs failed to initialize', error);
    if (dom.editor) setEditor('CodeDocs could not initialize.\n\n' + (error.message || String(error)));
    if (dom.fileBadge) setOutputStatus('Startup error', 'error');
  }
}

if (document.readyState === 'loading') {
  document.addEventListener('DOMContentLoaded', bootstrap, { once: true });
} else {
  bootstrap();
}
