import { type FC } from 'react'
import type { Label, LabelField } from '../types'

interface Props {
    labels: Label[]
    labelFields?: LabelField[]
}

const PALETTE = [
    '#2d6a4f',
    '#40916c',
    '#52b788',
    '#74c69d',
    '#95d5b2',
    '#b7e4c7',
    '#1b4332',
    '#344e41',
    '#588157',
    '#a3b18a',
    '#3a5a40',
    '#6b705c',
]

function formatCount(value: number): string {
    return value.toLocaleString()
}

function formatPercent(value: number): string {
    return `${Math.round(value * 100)}%`
}

export const LabelBar: FC<Props> = ({ labels, labelFields = [] }) => {
    const distributionLabels = labels.filter(
        (label) => label.proportion !== null && label.proportion > 0,
    )
    const classOnlyLabels = labels.filter(
        (label) => label.proportion === null || label.proportion <= 0,
    )

    if (distributionLabels.length === 0 && classOnlyLabels.length === 0 && labelFields.length === 0) {
        return <p className="label-bar-empty">No labels detected</p>
    }

    return (
        <div className="label-bar">
            {distributionLabels.length > 0 && (
                <>
                    <div className="label-bar-track">
                        {distributionLabels.map((label, i) => (
                            <div
                                key={label.name}
                                className="label-bar-segment"
                                style={{
                                    width: `${Math.max((label.proportion ?? 0) * 100, 1)}%`,
                                    background: PALETTE[i % PALETTE.length],
                                }}
                                title={`${label.name}: ${((label.proportion ?? 0) * 100).toFixed(1)}%${
                                    label.sample_count >= 0 ? ` (${formatCount(label.sample_count)} samples)` : ''
                                }`}
                            />
                        ))}
                    </div>
                    <div className="label-bar-legend">
                        {distributionLabels.map((label, i) => (
                            <div key={label.name} className="label-bar-legend-item">
                                <span
                                    className="label-bar-legend-dot"
                                    style={{ background: PALETTE[i % PALETTE.length] }}
                                />
                                <span className="label-bar-legend-name">{label.name}</span>
                                <span className="label-bar-legend-pct">
                                    {((label.proportion ?? 0) * 100).toFixed(1)}%
                                </span>
                            </div>
                        ))}
                    </div>
                </>
            )}

            {classOnlyLabels.length > 0 && (
                <div className="label-class-list">
                    {classOnlyLabels.map((label) => (
                        <div key={label.name} className="label-class-item">
                            <span>{label.name}</span>
                            {label.sample_count >= 0 && (
                                <small>{formatCount(label.sample_count)} sample{label.sample_count !== 1 ? 's' : ''}</small>
                            )}
                        </div>
                    ))}
                </div>
            )}

            {labelFields.length > 0 && (
                <div className="label-field-list">
                    {labelFields.map((field) => (
                        <div key={field.name} className="label-field-item">
                            <div>
                                <span className="label-field-name">Label column: {field.name}</span>
                                <small>
                                    {formatPercent(field.completeness)} populated
                                    {field.non_empty_count > 0 && ` (${formatCount(field.non_empty_count)} observed)`}
                                </small>
                            </div>
                            {field.examples.length > 0 && (
                                <div className="label-field-examples">
                                    {field.examples.slice(0, 4).map((example) => (
                                        <span key={example}>{example}</span>
                                    ))}
                                </div>
                            )}
                        </div>
                    ))}
                </div>
            )}
        </div>
    )
}
