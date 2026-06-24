import { Navigate, Route, Routes, useLocation } from 'react-router-dom'
import { useAuth } from '@/contexts/AuthContext'
import { LoadingSkeleton } from '@/components/ui/State'
import { LoginPage } from '@/pages/LoginPage'
import { SetupPage } from '@/pages/SetupPage'
import { DashboardPage } from '@/pages/DashboardPage'
import { AssetsPage, AssetDetailPage } from '@/pages/AssetsPage'
import { InstancesPage } from '@/pages/InstancesPage'
import { AlertsPage } from '@/pages/AlertsPage'
import { RulesPage } from '@/pages/RulesPage'
import { NotificationsPage } from '@/pages/NotificationsPage'
import { ScansPage } from '@/pages/ScansPage'
import { SettingsPage } from '@/pages/SettingsPage'

function RequireAuth({ children }: { children: React.ReactNode }) {
  const { token, isLoading, needsSetup } = useAuth()
  const location = useLocation()

  if (isLoading) {
    return (
      <div className="h-full flex items-center justify-center">
        <LoadingSkeleton rows={3} />
      </div>
    )
  }

  if (needsSetup) return <Navigate to="/setup" replace />
  if (!token) return <Navigate to="/login" state={{ from: location }} replace />

  return children
}

export function AppRoutes() {
  const { needsSetup, isLoading, token } = useAuth()

  if (isLoading) {
    return (
      <div className="h-full flex items-center justify-center">
        <LoadingSkeleton rows={3} />
      </div>
    )
  }

  return (
    <Routes>
      <Route
        path="/setup"
        element={needsSetup === false ? <Navigate to={token ? '/' : '/login'} replace /> : <SetupPage />}
      />
      <Route
        path="/login"
        element={token ? <Navigate to="/" replace /> : <LoginPage />}
      />
      <Route
        path="/"
        element={
          <RequireAuth>
            <DashboardPage />
          </RequireAuth>
        }
      />
      <Route
        path="/assets"
        element={
          <RequireAuth>
            <AssetsPage />
          </RequireAuth>
        }
      />
      <Route
        path="/assets/:id"
        element={
          <RequireAuth>
            <AssetDetailPage />
          </RequireAuth>
        }
      />
      <Route
        path="/instances"
        element={
          <RequireAuth>
            <InstancesPage />
          </RequireAuth>
        }
      />
      <Route
        path="/alerts"
        element={
          <RequireAuth>
            <AlertsPage />
          </RequireAuth>
        }
      />
      <Route
        path="/rules"
        element={
          <RequireAuth>
            <RulesPage />
          </RequireAuth>
        }
      />
      <Route
        path="/notifications"
        element={
          <RequireAuth>
            <NotificationsPage />
          </RequireAuth>
        }
      />
      <Route
        path="/scans"
        element={
          <RequireAuth>
            <ScansPage />
          </RequireAuth>
        }
      />
      <Route
        path="/settings"
        element={
          <RequireAuth>
            <SettingsPage />
          </RequireAuth>
        }
      />
      <Route path="*" element={<Navigate to="/" replace />} />
    </Routes>
  )
}
