import { type FC } from 'react'

interface Props {
  label: string
  uploadedChunks: number
  totalChunks: number
}

export const UploadProgress: FC<Props> = ({
  label,
  uploadedChunks,
  totalChunks,
}) => {
  const pct = totalChunks > 0 ? (uploadedChunks / totalChunks) * 100 : 0

  return (
    <div className="upload-progress">
      <div className="upload-progress-header">
        <span className="upload-filename">{label}</span>
        <span className="upload-pct">{pct.toFixed(0)}%</span>
      </div>
      <div className="upload-progress-track">
        <div
          className="upload-progress-fill"
          style={{ width: `${pct}%` }}
        />
      </div>
      <div className="upload-progress-detail">
        {uploadedChunks} / {totalChunks} chunks
      </div>
    </div>
  )
}
