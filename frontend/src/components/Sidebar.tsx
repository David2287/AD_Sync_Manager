import { NavLink } from 'react-router-dom';
import { useAuth } from '../context/AuthContext';

interface NavItem {
  to: string;
  label: string;
  icon: string;
  adminOnly?: boolean;
}

const navItems: NavItem[] = [
  { to: '/employees', label: 'Employees', icon: '👥' },
  { to: '/markdown',  label: 'MD Corrections', icon: '📝' },
  { to: '/logs',      label: 'Audit Logs', icon: '📋', adminOnly: true },
  { to: '/integrity', label: 'Integrity', icon: '🔍', adminOnly: true },
];

export function Sidebar() {
  const { perms } = useAuth();

  return (
    <aside className="flex h-full w-56 flex-shrink-0 flex-col border-r border-slate-700 bg-slate-800">
      <div className="flex h-14 items-center gap-2 border-b border-slate-700 px-4">
        <span className="text-xl">🔐</span>
        <span className="text-sm font-semibold text-slate-100">AD Sync Manager</span>
      </div>

      <nav className="flex-1 overflow-y-auto p-2">
        {navItems
          .filter((item) => !item.adminOnly || perms.isAdmin)
          .map((item) => (
            <NavLink
              key={item.to}
              to={item.to}
              className={({ isActive }) =>
                `flex items-center gap-3 rounded-md px-3 py-2 text-sm transition-colors mb-0.5 ${
                  isActive
                    ? 'bg-blue-600 text-white'
                    : 'text-slate-300 hover:bg-slate-700 hover:text-slate-100'
                }`
              }
            >
              <span>{item.icon}</span>
              <span>{item.label}</span>
            </NavLink>
          ))}
      </nav>

      <div className="border-t border-slate-700 p-2">
        {perms.isAdmin && (
          <div className="mb-1 flex items-center gap-1.5 px-3 py-1">
            <span className="h-1.5 w-1.5 rounded-full bg-blue-400" />
            <span className="text-xs text-slate-400">Admin</span>
          </div>
        )}
        {perms.isEditor && (
          <div className="flex items-center gap-1.5 px-3 py-1">
            <span className="h-1.5 w-1.5 rounded-full bg-green-400" />
            <span className="text-xs text-slate-400">Editor</span>
          </div>
        )}
      </div>
    </aside>
  );
}
