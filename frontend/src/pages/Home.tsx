import { useState, useEffect, useRef } from 'react'
import { Link } from 'react-router-dom'
import { search, listDatasets } from '../api/client'
import { SearchBar } from '../components/SearchBar'
import { FilterPanel } from '../components/FilterPanel'
import { DatasetCard } from '../components/DatasetCard'
import type { SearchResult, DatasetSummary, SearchFilters } from '../types'

const PAGE_SIZE = 20

export default function Home() {
    const [query, setQuery] = useState('')
    const [results, setResults] = useState<SearchResult[]>([])
    const [datasets, setDatasets] = useState<DatasetSummary[]>([])
    const [filters, setFilters] = useState<Partial<SearchFilters>>({})
    const [visibleCount, setVisibleCount] = useState(PAGE_SIZE)
    const [loading, setLoading] = useState(false)
    const [error, setError] = useState<string | null>(null)
    const searchIdRef = useRef(0)

    const safeResults = results ?? []
    const safeDatasets = datasets ?? []
    const hasActiveFilters = Object.keys(filters || {}).length > 0
    const hasSearchQuery = query.trim().length > 0
    const isSearching = hasSearchQuery || hasActiveFilters

    // Load all datasets on mount
    useEffect(() => {
        loadDatasets()
    }, [])

    // Reset visible count when switching between search/browse
    useEffect(() => {
        setVisibleCount(PAGE_SIZE)
    }, [isSearching])

    async function loadDatasets() {
        try {
            const data = await listDatasets()
            setDatasets(Array.isArray(data) ? data : [])
        } catch (err) {
            setError(err instanceof Error ? err.message : 'Failed to load datasets')
        }
    }

    async function executeSearch(q: string, f: Partial<SearchFilters>) {
        const id = ++searchIdRef.current
        setLoading(true)
        setError(null)
        try {
            const res = await search(q, f)
            if (id !== searchIdRef.current) return
            setResults(res.results || [])
        } catch (err) {
            if (id !== searchIdRef.current) return
            setError(err instanceof Error ? err.message : 'Search failed')
            setResults([])
        } finally {
            if (id === searchIdRef.current) setLoading(false)
        }
    }

    function handleSearch(q: string) {
        setQuery(q)
        // Pass explicit filter object to avoid stale closure
        executeSearch(q, filters)
    }

    function handleFilterChange(newFilters: Partial<SearchFilters>) {
        // Normalize null/undefined filters to empty object
        const normalizedFilters = newFilters || {}
        setFilters(normalizedFilters)
        // Always re-search when filters change, passing explicit values
        // This avoids the stale-closure bug
        executeSearch(query, normalizedFilters)
    }

    function handleReset() {
        setQuery('')
        setFilters({})
        setResults([])
        setError(null)
    }

    function handleClearFilters() {
        setFilters({})
        if (hasSearchQuery) {
            executeSearch(query, {})
        } else {
            setResults([])
        }
    }

    function handleLoadMore() {
        setVisibleCount((prev) => prev + PAGE_SIZE)
    }

    return (
        <div className="home-page">
            <section className="home-notice">
                <p>
                    An open dataset repository operated by the{' '}
                    <a href="https://www.lewisu.edu" target="_blank" rel="noreferrer">
                        Lewis University
                    </a>{' '}
                    Security Science Lab for education and research purposes.{' '}
                    <strong>No login required</strong> to browse, upload, or download datasets.
                    Account login is only needed for dataset management (edit&nbsp;/ delete).
                </p>
            </section>

            <section className="home-search-section">
                <div className="search-zone">
                    <SearchBar
                        initialQuery={query}
                        onSearch={handleSearch}
                    />
                    <FilterPanel
                        filters={filters}
                        onChange={handleFilterChange}
                        onClearAll={handleClearFilters}
                    />
                </div>
                {isSearching && (
                    <div className="home-search-actions">
                        <button className="clear-all-search" onClick={handleReset}>
                            Clear search and filters
                        </button>
                    </div>
                )}
            </section>

            <main className="home-main">
                {error && <div className="error-banner">{error}</div>}

                {loading && <div className="loading">Searching...</div>}

                {!loading && isSearching && safeResults.length === 0 && (
                    <div className="no-results">
                        <p>No datasets found matching your criteria.</p>
                        <p>Try different terms or adjust your filters.</p>
                        <button className="upload-cta" onClick={handleReset}>
                            Clear search & filters
                        </button>
                    </div>
                )}

                {!loading && isSearching && safeResults.length > 0 && (
                    <div className="results-section">
                        <div className="results-header">
                            <span className="results-count">
                                {safeResults.length} result{safeResults.length !== 1 ? 's' : ''}
                                {hasSearchQuery ? ` for "${query}"` : ''}
                            </span>
                            <button className="results-back" onClick={handleReset}>
                                Clear search & filters
                            </button>
                        </div>
                        <div className="results-grid">
                            {safeResults.map((r) => (
                                <DatasetCard
                                    key={r.dataset_id}
                                    dataset={r}
                                    citation={r.citation}
                                    fusionScore={r.fusion_score}
                                />
                            ))}
                        </div>
                    </div>
                )}

                {!loading && !isSearching && safeDatasets.length === 0 && (
                    <div className="no-results">
                        <p>No datasets uploaded yet.</p>
                        <Link to="/upload" className="upload-cta">
                            Upload the first dataset
                        </Link>
                    </div>
                )}

                {!loading && !isSearching && safeDatasets.length > 0 && (
                    <div className="browse-section">
                        <div className="browse-header">
                            <h2>All Datasets</h2>
                            <span className="browse-count">
                                {safeDatasets.length} dataset{safeDatasets.length !== 1 ? 's' : ''}
                            </span>
                        </div>
                        <div className="results-grid">
                            {safeDatasets.slice(0, visibleCount).map((d) => (
                                <DatasetCard
                                    key={d.id}
                                    dataset={d}
                                />
                            ))}
                        </div>
                        {visibleCount < safeDatasets.length && (
                            <div className="load-more-container">
                                <button
                                    className="load-more-btn"
                                    onClick={handleLoadMore}
                                >
                                    Load More ({safeDatasets.length - visibleCount} remaining)
                                </button>
                            </div>
                        )}
                    </div>
                )}
            </main>
        </div>
    )
}
