import { Navigate, Route, Routes } from 'react-router-dom'
import { AuthProvider, useAuth } from './AuthContext'
import Login from './pages/Login'
import DeviceList from './pages/DeviceList'
import DeviceDetail from './pages/DeviceDetail'
import Customers from './pages/Customers'
import Alerts from './pages/Alerts'
import Releases from './pages/Releases'
import Users from './pages/Users'
import Settings from './pages/Settings'
import Help from './pages/Help'

function RequireAuth({ children }: { children: React.ReactElement }) {
  const { user, loading } = useAuth()
  if (loading) return <div className="page">Loading...</div>
  if (!user) return <Navigate to="/login" replace />
  return children
}

function AppRoutes() {
  return (
    <Routes>
      <Route path="/login" element={<Login />} />
      <Route path="/" element={<RequireAuth><DeviceList /></RequireAuth>} />
      <Route path="/customers" element={<RequireAuth><Customers /></RequireAuth>} />
      <Route path="/alerts" element={<RequireAuth><Alerts /></RequireAuth>} />
      <Route path="/releases" element={<RequireAuth><Releases /></RequireAuth>} />
      <Route path="/users" element={<RequireAuth><Users /></RequireAuth>} />
      <Route path="/settings" element={<RequireAuth><Settings /></RequireAuth>} />
      <Route path="/help" element={<RequireAuth><Help /></RequireAuth>} />
      <Route path="/help/:slug" element={<RequireAuth><Help /></RequireAuth>} />
      <Route path="/devices/:id" element={<RequireAuth><DeviceDetail /></RequireAuth>} />
    </Routes>
  )
}

function Footer() {
  return (
    <footer className="app-footer">
      Powered by <a href="https://sonnyathome.online" target="_blank" rel="noopener noreferrer">sonnyathome.online</a>
    </footer>
  )
}

export default function App() {
  return (
    <AuthProvider>
      <AppRoutes />
      <Footer />
    </AuthProvider>
  )
}
