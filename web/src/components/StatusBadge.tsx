import { useTranslation } from 'react-i18next'

const STATUS_COLORS: Record<string, string> = {
  online: 'green',
  connection_lost: 'yellow',
  offline: 'gray',
  unknown: 'gray',
  revoked: 'gray',
  healthy: 'green',
  warning: 'yellow',
  critical: 'red',
}

export default function StatusBadge({ kind, value }: { kind: 'status' | 'health'; value: string }) {
  const { t } = useTranslation()
  const color = STATUS_COLORS[value] ?? 'gray'
  const label = t(`${kind}Values.${value}`, { defaultValue: value })
  return <span className={`badge badge-${color}`} title={`${kind}: ${value}`}>{label}</span>
}
