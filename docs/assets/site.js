(() => {
  const key = 'hitch:theme';
  const root = document.documentElement;
  const button = document.querySelector('[data-theme-toggle]');
  const prefersDark = window.matchMedia('(prefers-color-scheme: dark)');

  const resolve = (mode) => (mode === 'system' ? (prefersDark.matches ? 'dark' : 'light') : mode);
  const readMode = () => {
    const stored = localStorage.getItem(key);
    return stored === 'light' || stored === 'dark' || stored === 'system' ? stored : 'system';
  };
  const apply = (mode) => {
    const theme = resolve(mode);
    root.dataset.theme = theme;
    root.dataset.themeMode = mode;
    root.style.colorScheme = theme;
    if (button) {
      button.textContent = mode === 'dark' ? 'Dark' : mode === 'light' ? 'Light' : 'System';
      button.setAttribute('aria-label', `Theme preference: ${mode}. Activate to cycle theme.`);
    }
  };

  apply(readMode());
  prefersDark.addEventListener('change', () => apply(readMode()));
  button?.addEventListener('click', () => {
    const next = readMode() === 'system' ? 'dark' : readMode() === 'dark' ? 'light' : 'system';
    localStorage.setItem(key, next);
    apply(next);
  });
})();
