import { type FC } from 'react'

interface Props {
  source: 'keyword' | 'semantic' | 'hybrid'
}

const COLORS: Record<string, { bg: string; text: string; label: string }> = {
  keyword: { bg: '#f0ebe3', text: '#5c4033', label: 'Keyword' },
  semantic: { bg: '#e3edf0', text: '#2c5f6b', label: 'Semantic' },
  hybrid: { bg: '#e8e3f0', text: '#4a2c6b', label: 'Hybrid' },
}

export const CitationBadge: FC<Props> = ({ source }) => {
  const c = COLORS[source] || COLORS.keyword
  return (
    <span
      className="citation-badge"
      style={{
        background: c.bg,
        color: c.text,
      }}
      title={`Found via ${c.label} retrieval`}
    >
      {c.label}
    </span>
  )
}
