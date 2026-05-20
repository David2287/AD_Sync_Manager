import { useState, type FormEvent } from 'react';
import { useNavigate } from 'react-router-dom';
import { useAuth } from '../context/AuthContext';
import { authApi } from '../services/api';
import { LoadingSpinner } from '../components/LoadingSpinner';

export function LoginPage() {
  const [username, setUsername] = useState('');
  const [password, setPassword] = useState('');
  const [error, setError] = useState('');
  const [isLoading, setIsLoading] = useState(false);
  const { login } = useAuth();
  const navigate = useNavigate();

  async function handleSubmit(e: FormEvent) {
    e.preventDefault();
    setError('');
    setIsLoading(true);
    try {
      const res = await authApi.login(username, password);
      await login(res.data.token);
      navigate('/employees', { replace: true });
    } catch (err: unknown) {
      const msg =
        (err as { response?: { data?: { error?: string } } })?.response?.data?.error ??
        'Login failed. Please try again.';
      setError(msg);
    } finally {
      setIsLoading(false);
    }
  }

  return (
    <div className="flex min-h-screen items-center justify-center bg-slate-900 px-4">
      <div className="w-full max-w-sm">
        <div className="mb-8 text-center">
          <div className="mb-3 text-4xl">🔐</div>
          <h1 className="text-2xl font-bold text-slate-100">AD Sync Manager</h1>
          <p className="mt-1 text-sm text-slate-400">Sign in with your AD credentials</p>
        </div>

        <form
          onSubmit={handleSubmit}
          className="rounded-xl border border-slate-700 bg-slate-800 p-6 shadow-2xl"
        >
          <div className="mb-4">
            <label className="mb-1.5 block text-xs font-medium text-slate-300" htmlFor="username">
              Username
            </label>
            <input
              id="username"
              type="text"
              autoComplete="username"
              value={username}
              onChange={(e) => setUsername(e.target.value)}
              required
              className="w-full rounded-md border border-slate-600 bg-slate-900 px-3 py-2 text-sm
                text-slate-100 placeholder-slate-500 outline-none transition
                focus:border-blue-500 focus:ring-1 focus:ring-blue-500"
              placeholder="jdoe"
            />
          </div>

          <div className="mb-6">
            <label className="mb-1.5 block text-xs font-medium text-slate-300" htmlFor="password">
              Password
            </label>
            <input
              id="password"
              type="password"
              autoComplete="current-password"
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              required
              className="w-full rounded-md border border-slate-600 bg-slate-900 px-3 py-2 text-sm
                text-slate-100 placeholder-slate-500 outline-none transition
                focus:border-blue-500 focus:ring-1 focus:ring-blue-500"
              placeholder="••••••••"
            />
          </div>

          {error && (
            <div className="mb-4 rounded-md border border-red-500/30 bg-red-950/50 px-3 py-2 text-sm text-red-300">
              {error}
            </div>
          )}

          <button
            type="submit"
            disabled={isLoading}
            className="flex w-full items-center justify-center gap-2 rounded-md bg-blue-600 px-4 py-2
              text-sm font-medium text-white transition hover:bg-blue-500
              disabled:cursor-not-allowed disabled:opacity-60"
          >
            {isLoading && <LoadingSpinner size="sm" />}
            {isLoading ? 'Signing in…' : 'Sign in'}
          </button>
        </form>
      </div>
    </div>
  );
}
