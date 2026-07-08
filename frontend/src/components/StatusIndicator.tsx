import { type FC } from 'react'

interface Props {
  status: string
}

const CONFIG: Record<string, { color: string; label: string }> = {
  pending: { color: '#b8860b', label: 'Pending' },
  processing: { color: '#2e86ab', label: 'Processing' },
  ready: { color: '#2d6a4f', label: 'Ready' },
  error: { color: '#c1121f', label: 'Error' },
}

export const StatusIndicator: FC<Props> = ({ status }) => {
  const c = CONFIG[status] || { color: '#666', label: status }
  return (
    <span className="status-indicator" style={{ color: c.color }}>
      <span className="status-dot" style={{ background: c.color }} />
      {c.label}
    </span>
  )
}
