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
  const color = STATUS_COLORS[value] ?? 'gray'
  return <span className={`badge badge-${color}`} title={`${kind}: ${value}`}>{value}</span>
}
