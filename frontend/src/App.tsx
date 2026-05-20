import { BrowserRouter, Routes, Route, Navigate } from 'react-router-dom';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { AuthProvider } from './context/AuthContext';
import { ToastProvider } from './components/Toast';
import { ProtectedRoute } from './components/ProtectedRoute';
import { Layout } from './components/Layout';
import { LoginPage } from './pages/LoginPage';
import { EmployeesPage } from './pages/EmployeesPage';
import { MarkdownPage } from './pages/MarkdownPage';
import { LogsPage } from './pages/LogsPage';
import { IntegrityPage } from './pages/IntegrityPage';

const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      retry: 1,
      staleTime: 30_000,
    },
  },
});

export default function App() {
  return (
    <QueryClientProvider client={queryClient}>
      <AuthProvider>
        <ToastProvider>
          <BrowserRouter>
            <Routes>
              <Route path="/login" element={<LoginPage />} />
              <Route
                element={
                  <ProtectedRoute>
                    <Layout />
                  </ProtectedRoute>
                }
              >
                <Route index element={<Navigate to="/employees" replace />} />
                <Route path="/employees" element={<EmployeesPage />} />
                <Route path="/markdown" element={<MarkdownPage />} />
                <Route
                  path="/logs"
                  element={
                    <ProtectedRoute requireAdmin>
                      <LogsPage />
                    </ProtectedRoute>
                  }
                />
                <Route
                  path="/integrity"
                  element={
                    <ProtectedRoute requireAdmin>
                      <IntegrityPage />
                    </ProtectedRoute>
                  }
                />
              </Route>
              <Route path="*" element={<Navigate to="/employees" replace />} />
            </Routes>
          </BrowserRouter>
        </ToastProvider>
      </AuthProvider>
    </QueryClientProvider>
  );
}
