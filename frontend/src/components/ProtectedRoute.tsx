import { Navigate } from 'react-router-dom';
import { useAuth } from '../context/AuthContext';
import { LoadingSpinner } from './LoadingSpinner';

interface Props {
  children: React.ReactNode;
  requireAdmin?: boolean;
}

export function ProtectedRoute({ children, requireAdmin = false }: Props) {
  const { token, perms, isLoading } = useAuth();

  if (isLoading) {
    return (
      <div className="flex h-screen items-center justify-center bg-slate-900">
        <LoadingSpinner size="lg" className="text-blue-500" />
      </div>
    );
  }

  if (!token) {
    return <Navigate to="/login" replace />;
  }

  if (requireAdmin && !perms.isAdmin) {
    return (
      <div className="flex h-full min-h-64 items-center justify-center">
        <div className="text-center">
          <p className="text-2xl font-semibold text-slate-100">Access Denied</p>
          <p className="mt-2 text-slate-400">You do not have admin privileges.</p>
        </div>
      </div>
    );
  }

  return <>{children}</>;
}
