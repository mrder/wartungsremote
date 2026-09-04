import { useEffect, useRef, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { useAuth } from '../AuthContext'

const INACTIVITY_LIMIT_SECONDS = 15 * 60
const WARNING_THRESHOLD_SECONDS = 60
const ACTIVITY_EVENTS = ['mousedown', 'mousemove', 'keydown', 'scroll', 'touchstart', 'wheel'] as const

export default function InactivityLogout() {
  const { t } = useTranslation()
  const { logout } = useAuth()
  const navigate = useNavigate()
  const [remaining, setRemaining] = useState(INACTIVITY_LIMIT_SECONDS)
  const lastActivityRef = useRef(Date.now())

  useEffect(() => {
    const resetActivity = () => {
      lastActivityRef.current = Date.now()
    }
    ACTIVITY_EVENTS.forEach((ev) => window.addEventListener(ev, resetActivity, { passive: true }))

    const interval = setInterval(async () => {
      const elapsed = Math.floor((Date.now() - lastActivityRef.current) / 1000)
      const left = INACTIVITY_LIMIT_SECONDS - elapsed
      if (left <= 0) {
        await logout()
        navigate('/login', { state: { reason: 'inactivity' }, replace: true })
      } else {
        setRemaining(left)
      }
    }, 1000)

    return () => {
      ACTIVITY_EVENTS.forEach((ev) => window.removeEventListener(ev, resetActivity))
      clearInterval(interval)
    }
  }, [logout, navigate])

  const minutes = Math.floor(remaining / 60)
  const seconds = remaining % 60
  const display = `${minutes}:${seconds.toString().padStart(2, '0')}`

  return (
    <div
      className={`inactivity-timer${remaining <= WARNING_THRESHOLD_SECONDS ? ' inactivity-timer-warn' : ''}`}
      title={t('common.inactivityLogoutHint')}
    >
      {t('common.autoLogoutIn', { time: display })}
    </div>
  )
}
