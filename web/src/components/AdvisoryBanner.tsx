import { useAuth } from '../AuthContext'

const SEVERITY_STYLE: Record<string, { bg: string; fg: string }> = {
  critical: { bg: '#4a1414', fg: '#ff8a8a' },
  warning: { bg: '#4a3814', fg: '#ffcf80' },
  info: { bg: '#14304a', fg: '#8ac4ff' },
}

// Shown on every dashboard page (not gated per-route) for admins who can
// act on it — see internal/config.ServerConfig.SecurityAdvisories. Not
// dismissible: these are ongoing deployment issues, not one-time
// notices, so they should keep showing until actually fixed.
export default function AdvisoryBanner() {
  const { user } = useAuth()
  if (!user?.advisories || user.advisories.length === 0) return null

  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: '1px' }}>
      {user.advisories.map((a) => {
        const style = SEVERITY_STYLE[a.severity] ?? SEVERITY_STYLE.info
        return (
          <div
            key={a.code}
            style={{
              background: style.bg,
              color: style.fg,
              padding: '0.5rem 1rem',
              fontSize: '0.85rem',
            }}
          >
            {a.message}
          </div>
        )
      })}
    </div>
  )
}
