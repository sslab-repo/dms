import { type FC, useState, useRef, useEffect } from 'react'
import type { SearchFilters } from '../types'

interface Props {
    filters: Partial<SearchFilters>
    onChange: (filters: Partial<SearchFilters>) => void
    onClearAll: () => void
}

type SizeUnit = 'MB' | 'GB'

const MODALITIES = ['text', 'image', 'tabular', 'audio', 'multimodal']
const DATASET_TYPES = ['supervised', 'unsupervised', 'semi-supervised', 'self-supervised']
const FORMATS = [
  'CSV', 'JSONL', 'JSON', 'Parquet',
  'COCO JSON', 'YOLO TXT', 'Pascal VOC XML', 'KITTI TXT',
  'CoNLL', 'HDF5', 'TFRecord', 'Arrow',
  'Alpaca JSON', 'ShareGPT JSON', 'OpenAI JSONL', 'DPO pairs', 'SQuAD JSON',
  'plain text', 'image files',
]
const FORMATS_INITIAL = 6

const sizeMultiplier = (unit: SizeUnit) => (
    unit === 'GB' ? 1024 * 1024 * 1024 : 1024 * 1024
)

export const FilterPanel: FC<Props> = ({ filters, onChange, onClearAll }) => {
    const safeFilters = filters || {}
    const [sizeUnit, setSizeUnit] = useState<SizeUnit>('MB')
    const [formatsExpanded, setFormatsExpanded] = useState(false)
    const formatChipsRef = useRef<HTMLDivElement>(null)
    const [datePreset, setDatePreset] = useState<'1m' | '6m' | '1y' | 'custom' | ''>('')
    const [customFrom, setCustomFrom] = useState('')
    const [customTo, setCustomTo] = useState('')

    // When a currently-hidden format is active, auto-expand so it's visible.
    useEffect(() => {
        const active = safeFilters.annotation_format
        if (!active) return
        const idx = FORMATS.indexOf(active)
        if (idx >= FORMATS_INITIAL) setFormatsExpanded(true)
    }, [safeFilters.annotation_format])

    // Sync local date state when filters are cleared externally.
    useEffect(() => {
        if (!safeFilters.uploaded_after && !safeFilters.uploaded_before) {
            setDatePreset('')
            setCustomFrom('')
            setCustomTo('')
        }
    }, [safeFilters.uploaded_after, safeFilters.uploaded_before])

    function applyPreset(preset: '1m' | '6m' | '1y') {
        const d = new Date()
        if (preset === '1m') d.setMonth(d.getMonth() - 1)
        else if (preset === '6m') d.setMonth(d.getMonth() - 6)
        else d.setFullYear(d.getFullYear() - 1)
        const after = d.toISOString().split('T')[0]
        setDatePreset(preset)
        const copy = { ...safeFilters }
        delete copy.uploaded_before
        onChange({ ...copy, uploaded_after: after })
    }

    function clearDateFilter() {
        setDatePreset('')
        setCustomFrom('')
        setCustomTo('')
        const copy = { ...safeFilters }
        delete copy.uploaded_after
        delete copy.uploaded_before
        onChange(copy)
    }

    function handleCustomFrom(val: string) {
        setCustomFrom(val)
        setDatePreset('custom')
        onChange({ ...safeFilters, uploaded_after: val })
    }

    function handleCustomTo(val: string) {
        setCustomTo(val)
        setDatePreset('custom')
        onChange({ ...safeFilters, uploaded_before: val })
    }

    const visibleFormats = formatsExpanded ? FORMATS : FORMATS.slice(0, FORMATS_INITIAL)
    const multiplier = sizeMultiplier(sizeUnit)
    const hasActive = Object.keys(safeFilters).length > 0

    const set = (key: keyof SearchFilters, value: string) => {
        onChange({ ...safeFilters, [key]: value })
    }

    const clear = (key: keyof SearchFilters) => {
        const copy = { ...safeFilters }
        delete copy[key]
        onChange(copy)
    }

    const sizeInputValue = (value?: string) => {
        if (!value) return ''
        const bytes = Number(value)
        if (!Number.isFinite(bytes)) return ''
        return String(Math.round((bytes / multiplier) * 10) / 10)
    }

    const setSize = (key: 'min_size' | 'max_size', value: string) => {
        if (!value) {
            clear(key)
            return
        }
        set(key, String(Math.round(Number(value) * multiplier)))
    }

    return (
        <div className="filter-panel">
            <div className="filter-body">
                <div className="filter-group">
                    <label>Modality</label>
                    <div className="filter-chips">
                        {MODALITIES.map((m) => (
                            <button
                                key={m}
                                className={`chip ${safeFilters.modality === m ? 'active' : ''}`}
                                onClick={() =>
                                    safeFilters.modality === m
                                        ? clear('modality')
                                        : set('modality', m)
                                }
                            >
                                {m}
                            </button>
                        ))}
                    </div>
                </div>

                <div className="filter-group">
                    <label>Dataset Type</label>
                    <div className="filter-chips">
                        {DATASET_TYPES.map((t) => (
                            <button
                                key={t}
                                className={`chip ${safeFilters.dataset_type === t ? 'active' : ''}`}
                                onClick={() =>
                                    safeFilters.dataset_type === t
                                        ? clear('dataset_type')
                                        : set('dataset_type', t)
                                }
                            >
                                {t}
                            </button>
                        ))}
                    </div>
                </div>

                <div className="filter-group">
                    <label>Annotation Format</label>
                    <div className="filter-chips" ref={formatChipsRef}>
                        {visibleFormats.map((f) => (
                            <button
                                key={f}
                                className={`chip ${safeFilters.annotation_format === f ? 'active' : ''}`}
                                onClick={() =>
                                    safeFilters.annotation_format === f
                                        ? clear('annotation_format')
                                        : set('annotation_format', f)
                                }
                            >
                                {f}
                            </button>
                        ))}
                        <button
                            className="chip chip-more"
                            onClick={() => setFormatsExpanded((v) => !v)}
                        >
                            {formatsExpanded ? '− less' : `+${FORMATS.length - FORMATS_INITIAL} more`}
                        </button>
                    </div>
                </div>

                <div className="filter-group">
                    <div className="filter-label-row">
                        <label>Uploaded Date</label>
                        {datePreset && (
                            <button className="filter-clear-inline" onClick={clearDateFilter}>clear</button>
                        )}
                    </div>
                    <div className="filter-chips">
                        {(['1m', '6m', '1y'] as const).map((p) => (
                            <button
                                key={p}
                                className={`chip ${datePreset === p ? 'active' : ''}`}
                                onClick={() => datePreset === p ? clearDateFilter() : applyPreset(p)}
                            >
                                {p === '1m' ? '1 month' : p === '6m' ? '6 months' : '1 year'}
                            </button>
                        ))}
                        <button
                            className={`chip ${datePreset === 'custom' ? 'active' : ''}`}
                            onClick={() => setDatePreset('custom')}
                        >
                            Custom
                        </button>
                    </div>
                    {datePreset === 'custom' && (
                        <div className="filter-date-range">
                            <input
                                type="date"
                                value={customFrom}
                                onChange={(e) => handleCustomFrom(e.target.value)}
                                placeholder="From"
                            />
                            <span>—</span>
                            <input
                                type="date"
                                value={customTo}
                                onChange={(e) => handleCustomTo(e.target.value)}
                                placeholder="To"
                            />
                        </div>
                    )}
                </div>

                <div className="filter-group-range">
                    <div className="filter-label-row">
                        <label>Size Range</label>
                        <div className="unit-toggle" aria-label="Size unit">
                            {(['MB', 'GB'] as SizeUnit[]).map((unit) => (
                                <button
                                    key={unit}
                                    type="button"
                                    className={sizeUnit === unit ? 'active' : ''}
                                    onClick={() => setSizeUnit(unit)}
                                >
                                    {unit}
                                </button>
                            ))}
                        </div>
                    </div>
                    <div className="filter-range">
                        <input
                            type="number"
                            min={0}
                            placeholder={`Min ${sizeUnit}`}
                            value={sizeInputValue(safeFilters.min_size)}
                            onChange={(e) => setSize('min_size', e.target.value)}
                        />
                        <span>—</span>
                        <input
                            type="number"
                            min={0}
                            placeholder={`Max ${sizeUnit}`}
                            value={sizeInputValue(safeFilters.max_size)}
                            onChange={(e) => setSize('max_size', e.target.value)}
                        />
                    </div>
                </div>

                <div className="filter-group-range">
                    <label>Label Completeness</label>
                    <div className="filter-range">
                        <input
                            type="number"
                            min={0}
                            max={100}
                            placeholder="Min %"
                            value={
                                safeFilters.min_label_completeness
                                    ? Math.round(Number(safeFilters.min_label_completeness) * 100)
                                    : ''
                            }
                            onChange={(e) => {
                                const v = e.target.value
                                    ? String(Number(e.target.value) / 100)
                                    : ''
                                if (v) set('min_label_completeness', v)
                                else clear('min_label_completeness')
                            }}
                        />
                        <span>—</span>
                        <input
                            type="number"
                            min={0}
                            max={100}
                            placeholder="Max %"
                            value={
                                safeFilters.max_label_completeness
                                    ? Math.round(Number(safeFilters.max_label_completeness) * 100)
                                    : ''
                            }
                            onChange={(e) => {
                                const v = e.target.value
                                    ? String(Number(e.target.value) / 100)
                                    : ''
                                if (v) set('max_label_completeness', v)
                                else clear('max_label_completeness')
                            }}
                        />
                    </div>
                </div>

            </div>

            {hasActive && (
                <div className="filter-footer">
                    <button className="filter-reset" onClick={onClearAll}>
                        Clear filters
                    </button>
                </div>
            )}
        </div>
    )
}