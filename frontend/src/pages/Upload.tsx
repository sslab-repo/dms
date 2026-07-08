import { useState, useRef, useEffect, type FormEvent } from 'react'
import { Link } from 'react-router-dom'
import { BASE, createDataset, registerFile } from '../api/client'
import { UploadProgress } from '../components/UploadProgress'
import { useAuth } from '../context/AuthContext'
import { uploadFilesForDataset } from '../utils/uploadFiles'
import { formatBytes } from '../utils'
import type { CreateDatasetRequest } from '../types'

type Step = 'info' | 'uploading' | 'processing' | 'done' | 'error'

export default function Upload() {
  const { user } = useAuth()
  const [step, setStep] = useState<Step>('info')
  const [datasetId, setDatasetId] = useState<number | null>(null)
  const [errorMessage, setErrorMessage] = useState('')
  const [processingStage, setProcessingStage] = useState('')

  const [name, setName] = useState('')
  const [researcher, setResearcher] = useState('')
  const [uploaderEmail, setUploaderEmail] = useState('')
  const [description, setDescription] = useState('')
  const [tags, setTags] = useState('')
  const [labelColumn, setLabelColumn] = useState('')

  const [selectedFiles, setSelectedFiles] = useState<File[]>([])
  const [uploadLabel, setUploadLabel] = useState('Preparing upload')
  const [uploadedChunks, setUploadedChunks] = useState(0)
  const [totalChunks, setTotalChunks] = useState(0)
  const [dragOver, setDragOver] = useState(false)

  const fileInputRef = useRef<HTMLInputElement>(null)
  const pollRef = useRef<ReturnType<typeof setInterval> | null>(null)

  useEffect(() => {
    if (user) {
      if (!researcher) setResearcher(user.display_name)
    }
  }, [user]) // eslint-disable-line react-hooks/exhaustive-deps

  function handleFileSelect(e: React.ChangeEvent<HTMLInputElement>) {
    const files = Array.from(e.target.files || [])
    if (files.length > 0) {
      setSelectedFiles((prev) => [...prev, ...files])
    }
  }

  function removeFile(index: number) {
    setSelectedFiles((prev) => prev.filter((_, i) => i !== index))
  }

  function handleDragOver(e: React.DragEvent) {
    e.preventDefault()
    e.stopPropagation()
    setDragOver(true)
  }

  function handleDragLeave(e: React.DragEvent) {
    e.preventDefault()
    e.stopPropagation()
    setDragOver(false)
  }

  function handleDrop(e: React.DragEvent) {
    e.preventDefault()
    e.stopPropagation()
    setDragOver(false)
    const files = Array.from(e.dataTransfer.files)
    if (files.length > 0) {
      setSelectedFiles((prev) => [...prev, ...files])
    }
  }

  async function handleCreateDatasetAndUpload(e: FormEvent) {
    e.preventDefault()
    if (!name.trim() || !researcher.trim() || !uploaderEmail.trim() || selectedFiles.length === 0) return

    try {
      const body: CreateDatasetRequest = {
        name: name.trim(),
        researcher_name: researcher.trim(),
        uploader_email: uploaderEmail.trim(),
        user_description: description.trim(),
        tags: tags
          .split(',')
          .map((t) => t.trim())
          .filter(Boolean),
        total_files: selectedFiles.length,
        ...(labelColumn.trim() ? { label_column: labelColumn.trim() } : {}),
      }
      const res = await createDataset(body)
      setDatasetId(res.dataset_id)
      setStep('uploading')
      await uploadFiles(res.dataset_id, selectedFiles)
    } catch (err) {
      setErrorMessage(err instanceof Error ? err.message : 'Failed to create dataset')
      setStep('error')
    }
  }

  async function uploadFiles(datasetId: number, files: File[]) {
    await uploadFilesForDataset(
      datasetId,
      files,
      (id, file) => registerFile(id, file.name, file.type || ''),
      {
        onStart: (label, chunkTotal) => {
          setUploadLabel(label)
          setUploadedChunks(0)
          setTotalChunks(chunkTotal)
        },
        onChunkComplete: setUploadedChunks,
      },
    )
    setStep('processing')
    startPolling(datasetId)
  }

  function startPolling(datasetId: number) {
    pollRef.current = setInterval(async () => {
      try {
        const res = await fetch(`${BASE}/api/datasets/${datasetId}`)
        const data = await res.json()
        setProcessingStage(data.processing_stage || '')

        if (data.status === 'ready') {
          if (pollRef.current) clearInterval(pollRef.current)
          setStep('done')
        } else if (data.status === 'error') {
          if (pollRef.current) clearInterval(pollRef.current)
          setErrorMessage(data.error_message || 'AI pipeline failed')
          setStep('error')
        }
      } catch {
        // Keep polling through transient network errors.
      }
    }, 2000)
  }

  useEffect(() => {
    return () => {
      if (pollRef.current) clearInterval(pollRef.current)
    }
  }, [])

  return (
    <div className="upload-page">
      <h1>Upload Dataset</h1>

      <div className="upload-steps-indicator">
        <div className={`step-dot ${step === 'info' || step === 'uploading' || step === 'processing' || step === 'done' ? 'active' : ''} ${step === 'info' ? 'current' : ''}`}>1</div>
        <div className="step-line" />
        <div className={`step-dot ${step === 'uploading' || step === 'processing' || step === 'done' ? 'active' : ''} ${step === 'uploading' ? 'current' : ''}`}>2</div>
        <div className="step-line" />
        <div className={`step-dot ${step === 'processing' || step === 'done' ? 'active' : ''} ${step === 'processing' ? 'current' : ''}`}>3</div>
      </div>

      <main className="upload-main">
        {step === 'info' && (
          <form className="upload-form" onSubmit={handleCreateDatasetAndUpload}>
            <h2>Dataset Information</h2>

            <div className="form-group">
              <label htmlFor="ds-name">Title *</label>
              <input
                id="ds-name"
                type="text"
                value={name}
                onChange={(e) => setName(e.target.value)}
                placeholder="e.g. CIFAR-10 augmented"
                required
              />
            </div>

            <div className="form-group">
              <label htmlFor="ds-researcher">Your Name *</label>
              <input
                id="ds-researcher"
                type="text"
                value={researcher}
                onChange={(e) => setResearcher(e.target.value)}
                placeholder="e.g. Prof. Janeway"
                required
              />
            </div>

            <div className="form-group">
              <label htmlFor="ds-email">Your Email *</label>
              <input
                id="ds-email"
                type="email"
                value={uploaderEmail}
                onChange={(e) => setUploaderEmail(e.target.value)}
                placeholder="e.g. janeway@lewisu.edu"
                required
              />
            </div>

            <div className="form-group">
              <label htmlFor="ds-description">Description</label>
              <textarea
                id="ds-description"
                value={description}
                onChange={(e) => setDescription(e.target.value)}
                placeholder="Brief description of the dataset (optional - AI will generate a summary)"
                rows={3}
              />
            </div>

            <div className="form-group">
              <label htmlFor="ds-tags">Tags</label>
              <input
                id="ds-tags"
                type="text"
                value={tags}
                onChange={(e) => setTags(e.target.value)}
                placeholder="Comma-separated, e.g. computer-vision, classification"
              />
            </div>

            <div className="form-group">
              <label htmlFor="ds-label-column">Label Column Name</label>
              <input
                id="ds-label-column"
                type="text"
                value={labelColumn}
                onChange={(e) => setLabelColumn(e.target.value)}
                placeholder="e.g. Label, target, class (optional — helps for wide CSVs)"
              />
              <p className="form-hint">
                If your dataset has a label/target column that may not be in the first 20 columns, name it here so the system classifies the dataset correctly.
              </p>
            </div>

            <div className="form-group">
              <label htmlFor="ds-files">Files *</label>
              <div
                className={`file-drop-zone ${dragOver ? 'drag-over' : ''}`}
                onClick={() => fileInputRef.current?.click()}
                onDragOver={handleDragOver}
                onDragLeave={handleDragLeave}
                onDrop={handleDrop}
              >
                <input
                  ref={fileInputRef}
                  type="file"
                  multiple
                  onChange={handleFileSelect}
                  style={{ display: 'none' }}
                />
                {selectedFiles.length === 0 ? (
                  <div className="file-drop-prompt">
                    <span className="drop-icon">+</span>
                    <p>Click to select files or drag and drop here</p>
                  </div>
                ) : (
                  <div className="file-list">
                    {selectedFiles.map((f, i) => (
                      <div key={`${f.name}-${i}`} className="file-list-item">
                        <span className="file-list-name">{f.name}</span>
                        <span className="file-list-size">
                          {formatBytes(f.size)}
                        </span>
                        <button
                          type="button"
                          className="file-list-remove"
                          onClick={(e) => {
                            e.stopPropagation()
                            removeFile(i)
                          }}
                          title="Remove file"
                        >
                          &times;
                        </button>
                      </div>
                    ))}
                  </div>
                )}
                {selectedFiles.length > 0 && (
                  <p className="file-drop-add-more">Click or drop to add more files</p>
                )}
              </div>
            </div>

            <button type="submit" className="btn-primary" disabled={selectedFiles.length === 0 || !researcher.trim() || !uploaderEmail.trim()}>
              Upload {selectedFiles.length} file{selectedFiles.length !== 1 ? 's' : ''}
            </button>
          </form>
        )}

        {step === 'uploading' && (
          <UploadProgress
            label={uploadLabel}
            uploadedChunks={uploadedChunks}
            totalChunks={totalChunks}
          />
        )}

        {step === 'processing' && (
          <div className="processing-message">
            <p>
              {processingStage === 'profiling'
                ? 'Profiling files and sampling representative records...'
                : processingStage === 'embedding'
                  ? 'Indexing the profile-enriched summary for search...'
                  : 'All files uploaded. AI is generating dataset metadata...'}
            </p>
            <div className="spinner" />
          </div>
        )}

        {step === 'done' && (
          <div className="upload-done-step">
            <h2>Upload Complete</h2>
            <p>The dataset has been processed and is now searchable.</p>
            <div className="upload-done-actions">
              <Link to={`/datasets/${datasetId}`} className="btn-primary">View Dataset</Link>
              <Link to="/" className="btn-secondary">Back to Home</Link>
            </div>
          </div>
        )}

        {step === 'error' && (
          <div className="upload-error-step">
            <h2>Upload Failed</h2>
            <p className="error-message">{errorMessage}</p>
            <div className="upload-error-actions">
              <button onClick={() => window.location.reload()} className="btn-secondary">Try Again</button>
            </div>
          </div>
        )}
      </main>
    </div>
  )
}
