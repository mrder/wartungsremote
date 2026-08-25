import { useEffect, useRef, useState } from 'react'
import { Terminal } from '@xterm/xterm'
import { FitAddon } from '@xterm/addon-fit'
import '@xterm/xterm/css/xterm.css'
import { SessionApi, ReauthApi, ApiError } from '../api'

interface Props {
  deviceId: string
}

export default function TerminalView({ deviceId }: Props) {
  const containerRef = useRef<HTMLDivElement>(null)
  const [error, setError] = useState('')
  const [connecting, setConnecting] = useState(false)
  const [connected, setConnected] = useState(false)
  const [privilegeUntil, setPrivilegeUntil] = useState<Date | null>(null)
  const [remainingSec, setRemainingSec] = useState(0)
  const sessionIdRef = useRef<string | null>(null)
  const wsRef = useRef<WebSocket | null>(null)
  const termRef = useRef<Terminal | null>(null)

  useEffect(() => {
    if (!privilegeUntil) return
    const interval = setInterval(() => {
      const secs = Math.max(0, Math.round((privilegeUntil.getTime() - Date.now()) / 1000))
      setRemainingSec(secs)
      if (secs === 0) setPrivilegeUntil(null)
    }, 1000)
    return () => clearInterval(interval)
  }, [privilegeUntil])

  async function requestPrivilege() {
    const sessionId = sessionIdRef.current
    if (!sessionId) return
    const password = prompt('Re-enter your password to request elevated rights:')
    if (!password) return
    const code = prompt('Enter your current 6-digit authenticator code:')
    if (!code) return
    try {
      const reauth = await ReauthApi.reauth(password, code)
      const grant = await SessionApi.grantPrivilege(sessionId, reauth.reauth_id, 15 * 60)
      setPrivilegeUntil(new Date(grant.valid_until))
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Failed to elevate privileges')
    }
  }

  async function revokePrivilege() {
    const sessionId = sessionIdRef.current
    if (!sessionId) return
    try {
      await SessionApi.revokePrivilege(sessionId)
      setPrivilegeUntil(null)
    } catch {
      // best effort
    }
  }

  async function connect() {
    if (!containerRef.current) return
    setError('')
    setConnecting(true)
    try {
      const sess = await SessionApi.createTerminal(deviceId)
      sessionIdRef.current = sess.session_id

      const term = new Terminal({ convertEol: true, fontSize: 13, theme: { background: '#0f1115' } })
      const fit = new FitAddon()
      term.loadAddon(fit)
      containerRef.current.innerHTML = ''
      term.open(containerRef.current)
      fit.fit()
      termRef.current = term

      const proto = location.protocol === 'https:' ? 'wss' : 'ws'
      const ws = new WebSocket(`${proto}://${location.host}/api/v1/sessions/${sess.session_id}/stream`)
      ws.binaryType = 'arraybuffer'
      wsRef.current = ws

      ws.onopen = () => {
        setConnected(true)
        setConnecting(false)
        ws.send(JSON.stringify({ cols: term.cols, rows: term.rows }))
      }
      ws.onmessage = (ev) => {
        if (ev.data instanceof ArrayBuffer) {
          term.write(new Uint8Array(ev.data))
        }
      }
      ws.onclose = () => {
        setConnected(false)
        term.write('\r\n\x1b[33m[session closed]\x1b[0m\r\n')
      }
      ws.onerror = () => setError('Connection error')

      term.onData((data) => {
        if (ws.readyState === WebSocket.OPEN) {
          ws.send(new TextEncoder().encode(data))
        }
      })
      term.onResize(({ cols, rows }) => {
        if (ws.readyState === WebSocket.OPEN) {
          ws.send(JSON.stringify({ cols, rows }))
        }
      })

      const resizeObserver = new ResizeObserver(() => fit.fit())
      resizeObserver.observe(containerRef.current)
    } catch (err) {
      setConnecting(false)
      setError(err instanceof ApiError ? err.message : 'Failed to open terminal session')
    }
  }

  function disconnect() {
    wsRef.current?.close()
    if (sessionIdRef.current) {
      SessionApi.close(sessionIdRef.current).catch(() => {})
    }
    termRef.current?.dispose()
  }

  useEffect(() => {
    return () => disconnect()
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  return (
    <div className="terminal-panel">
      {!connected && !connecting && (
        <button onClick={connect}>Open Terminal</button>
      )}
      {connecting && <p>Connecting...</p>}
      {error && <p className="error">{error}</p>}
      {connected && (
        <>
          <button onClick={disconnect}>Close Session</button>{' '}
          {privilegeUntil ? (
            <>
              <span className="badge badge-yellow">privileged: {remainingSec}s</span>{' '}
              <button onClick={revokePrivilege}>Revoke</button>
            </>
          ) : (
            <button onClick={requestPrivilege}>Request Admin Rights</button>
          )}
        </>
      )}
      <div ref={containerRef} className="terminal-container" style={{ display: connecting || connected ? 'block' : 'none' }} />
    </div>
  )
}
