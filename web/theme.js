/* theme.js — light/dark toggle, shared by every page.
   The <head> inline script already applies the saved theme before paint
   to avoid a flash; this only handles the toggle + label sync. */
(function () {
  function apply(t) {
    document.documentElement.classList.toggle('dark', t === 'dark');
    var dark = t === 'dark';
    document.querySelectorAll('[data-theme-ico]').forEach(function (e) { e.textContent = dark ? '☾' : '☀'; });
    document.querySelectorAll('[data-theme-label]').forEach(function (e) { e.textContent = dark ? 'Dark' : 'Light'; });
  }
  window.__toggleTheme = function () {
    var t = document.documentElement.classList.contains('dark') ? 'light' : 'dark';
    try { localStorage.setItem('molt-theme', t); } catch (e) {}
    apply(t);
  };
  apply(document.documentElement.classList.contains('dark') ? 'dark' : 'light');
})();
