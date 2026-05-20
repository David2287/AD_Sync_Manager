import { useAuth } from '../context/AuthContext';
import { useTheme } from '../hooks/useTheme';

export function Topbar() {
  const { user, logout } = useAuth();
  const [theme, toggleTheme] = useTheme();

  return (
    <header className="flex h-14 flex-shrink-0 items-center justify-between border-b border-slate-700 bg-slate-800 px-4">
      <div />
      <div className="flex items-center gap-3">
        <button
          onClick={toggleTheme}
          className="rounded-md p-1.5 text-slate-400 hover:bg-slate-700 hover:text-slate-200 transition-colors"
          title={theme === 'dark' ? 'Switch to light mode' : 'Switch to dark mode'}
        >
          {theme === 'dark' ? '☀️' : '🌙'}
        </button>

        {user && (
          <div className="flex items-center gap-2">
            <div className="flex h-7 w-7 items-center justify-center rounded-full bg-blue-600 text-xs font-semibold text-white">
              {user.username.slice(0, 1).toUpperCase()}
            </div>
            <span className="text-sm text-slate-300">{user.username}</span>
          </div>
        )}

        <button
          onClick={() => logout()}
          className="rounded-md border border-slate-600 px-3 py-1 text-xs text-slate-300
            hover:border-slate-500 hover:text-slate-100 transition-colors"
        >
          Sign out
        </button>
      </div>
    </header>
  );
}
