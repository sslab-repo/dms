import { type FC } from 'react'
import { Link } from 'react-router-dom'
import { CitationBadge } from './CitationBadge'
import { StatusIndicator } from './StatusIndicator'
import { formatBytes } from '../utils'
import { downloadUrl } from '../api/client'
import type { SearchResult, DatasetSummary } from '../types'

interface Props {
    dataset: SearchResult | DatasetSummary
    citation?: 'keyword' | 'semantic' | 'hybrid'
    fusionScore?: number
}

export const DatasetCard: FC<Props> = ({ dataset, citation, fusionScore }) => {
    const isSearchResult = 'citation' in dataset
    const id = isSearchResult ? (dataset as SearchResult).dataset_id : (dataset as DatasetSummary).id
    const status = isSearchResult ? 'ready' : (dataset as DatasetSummary).status
    const tags = 'tags' in dataset ? (dataset as SearchResult).tags : (dataset as DatasetSummary).tags

    const handleDownload = (e: React.MouseEvent) => {
        e.preventDefault()
        e.stopPropagation()
        window.open(downloadUrl(id), '_blank')
    }

    return (
        <Link to={`/datasets/${id}`} className="dataset-card">
            <div className="dataset-card-header">
                <h3 className="dataset-card-title">{dataset.name}</h3>
                <div className="dataset-card-badges">
                    {citation && <CitationBadge source={citation} />}
                    <StatusIndicator status={status} />
                </div>
            </div>

            <p className="dataset-card-researcher">
                by {dataset.researcher_name}
            </p>

            {dataset.ai_summary && (
                <p className="dataset-card-summary">{dataset.ai_summary}</p>
            )}

            <div className="dataset-card-meta">
                {dataset.modality && (
                    <span className="meta-tag modality">{dataset.modality}</span>
                )}
                {dataset.dataset_type && (
                    <span className="meta-tag type">{dataset.dataset_type}</span>
                )}
                <span className="meta-tag size">{formatBytes(dataset.total_size_bytes)}</span>
                {dataset.label_completeness > 0 && (
                    <span className="meta-tag completeness">
                        {(dataset.label_completeness * 100).toFixed(0)}% labeled
                    </span>
                )}
            </div>

            {tags && tags.length > 0 && (
                <div className="dataset-card-tags">
                    {tags.slice(0, 4).map((tag) => (
                        <span key={tag} className="meta-tag tag">{tag}</span>
                    ))}
                    {tags.length > 4 && (
                        <span className="meta-tag tag">+{tags.length - 4}</span>
                    )}
                </div>
            )}

            {fusionScore !== undefined && (
                <div className="dataset-card-score">
                    Relevance: {fusionScore.toFixed(4)}
                </div>
            )}

            {status === 'ready' && (
                <button className="dataset-card-download" onClick={handleDownload}>
                    Download
                </button>
            )}
        </Link>
    )
}
