// components/Chrome.tsx — spine + topnav shared chrome, plus theme toggle.
import { Link } from 'react-router-dom';

export function Spine({ middle }: { middle?: string }) {
  return (
    <div className="spine">
      <span>+ MOLTNET</span>
      <span>
        {middle || 'TRUST LAYER'} <span className="s">//</span> AGENT ECONOMY · MMXXVI
      </span>
      <span>◇</span>
    </div>
  );
}

export function ThemeToggle() {
  function toggle() {
    const dark = !document.documentElement.classList.contains('dark');
    document.documentElement.classList.toggle('dark', dark);
    try {
      localStorage.setItem('molt-theme', dark ? 'dark' : 'light');
    } catch {
      /* ignore */
    }
    syncToggle();
  }
  function syncToggle() {
    const dark = document.documentElement.classList.contains('dark');
    document.querySelectorAll('[data-theme-ico]').forEach((e) => {
      e.textContent = dark ? '☾' : '☀';
    });
    document.querySelectorAll('[data-theme-label]').forEach((e) => {
      e.textContent = dark ? 'Dark' : 'Light';
    });
  }
  return (
    <button
      className="btn btn--ghost btn--sm"
      onClick={() => {
        toggle();
      }}
      title="Toggle light / dark"
    >
      <span data-theme-ico>☀</span> <span data-theme-label>Light</span>
    </button>
  );
}

export function Mark() {
  return (
    <Link className="mark" to="/">
      <span className="gx">▚▞</span> <span className="nm">MoltNet</span>
    </Link>
  );
}