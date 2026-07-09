import { useState, useEffect, useRef, type FormEvent } from 'react'
import { Link, useNavigate, useParams } from 'react-router-dom'
import {
  getDataset,
  downloadUrl,
  exportDownloadUrl,
  updateDataset,
  deleteDataset,
  registerAdditionalFile,
  deleteDatasetFile,
} from '../api/client'
import { StatusIndicator } from '../components/StatusIndicator'
import { LabelBar } from '../components/LabelBar'
import { UploadProgress } from '../components/UploadProgress'
import { ExpandableText } from '../components/ExpandableText'
import { useAuth } from '../context/AuthContext'
import { uploadFilesForDataset } from '../utils/uploadFiles'
import { formatBytes } from '../utils'
import type { Dataset, UpdateDatasetRequest } from '../types'

export default function DatasetDetail() {
  const { id } = useParams<{ id: string }>()
  const navigate = useNavigate()
  const { user } = useAuth()
  const [dataset, setDataset] = useState<Dataset | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [editOpen, setEditOpen] = useState(false)
  const [deleteConfirmOpen, setDeleteConfirmOpen] = useState(false)
  const [actionError, setActionError] = useState('')
  const [actionLoading, setActionLoading] = useState(false)
  const [editName, setEditName] = useState('')
  const [editResearcher, setEditResearcher] = useState('')
  const [editDescription, setEditDescription] = useState('')
  const [editTags, setEditTags] = useState('')
  const [fileUploadLabel, setFileUploadLabel] = useState('')
  const [fileUploadedChunks, setFileUploadedChunks] = useState(0)
  const [fileTotalChunks, setFileTotalChunks] = useState(0)
  const [fileUploadActive, setFileUploadActive] = useState(false)
  const addFileInputRef = useRef<HTMLInputElement>(null)

  useEffect(() => {
    if (!id) return
    loadDataset(Number(id))
  }, [id])

  useEffect(() => {
    // Poll while the dataset is still ingesting, and also while the ML
    // package is being built so the export button can flip to enabled.
    const ingesting = dataset && (dataset.status === 'pending' || dataset.status === 'processing')
    const exporting = dataset && dataset.status === 'ready' && dataset.export_status === 'building'
    if (!ingesting && !exporting) return
    const timer = setInterval(() => {
      loadDataset(dataset.id, true)
    }, 2000)
    return () => clearInterval(timer)
  }, [dataset?.id, dataset?.status, dataset?.export_status])

  async function loadDataset(datasetId: number, quiet = false) {
    if (!quiet) {
      setLoading(true)
      setError(null)
    }
    try {
      const data = await getDataset(datasetId)
      setDataset(data)
    } catch (err) {
      if (!quiet) setError(err instanceof Error ? err.message : 'Failed to load dataset')
    } finally {
      if (!quiet) setLoading(false)
    }
  }

  function openEditPanel() {
    if (!dataset) return
    setActionError('')
    setDeleteConfirmOpen(false)
    setEditName(dataset.name)
    setEditResearcher(dataset.researcher_name)
    setEditDescription(dataset.description)
    setEditTags((dataset.tags ?? []).join(', '))
    setEditOpen(true)
  }

  async function handleSaveEdit(e: FormEvent) {
    e.preventDefault()
    if (!dataset) return

    const body: UpdateDatasetRequest = {
      name: editName.trim(),
      researcher_name: editResearcher.trim(),
      user_description: editDescription.trim(),
      tags: editTags
        .split(',')
        .map((tag) => tag.trim())
        .filter(Boolean),
    }

    setActionError('')
    setActionLoading(true)
    try {
      const updated = await updateDataset(dataset.id, body)
      setDataset(updated)
      setEditOpen(false)
    } catch (err) {
      setActionError(err instanceof Error ? err.message : 'Failed to update dataset')
    } finally {
      setActionLoading(false)
    }
  }

  async function handleDeleteDataset() {
    if (!dataset) return
    setActionError('')
    setActionLoading(true)
    try {
      await deleteDataset(dataset.id)
      navigate('/')
    } catch (err) {
      setActionError(err instanceof Error ? err.message : 'Failed to delete dataset')
      setActionLoading(false)
    }
  }

  async function handleAddFiles(e: React.ChangeEvent<HTMLInputElement>) {
    if (!dataset) return
    const filesToAdd = Array.from(e.target.files || [])
    e.target.value = ''
    if (filesToAdd.length === 0) return

    setActionError('')
    setActionLoading(true)
    setFileUploadActive(true)
    try {
      await uploadFilesForDataset(
        dataset.id,
        filesToAdd,
        (datasetId, file) => registerAdditionalFile(datasetId, file.name, file.type || ''),
        {
          onStart: (label, chunkTotal) => {
            setFileUploadLabel(label)
            setFileUploadedChunks(0)
            setFileTotalChunks(chunkTotal)
          },
          onChunkComplete: setFileUploadedChunks,
        },
      )
      await loadDataset(dataset.id, true)
    } catch (err) {
      setActionError(err instanceof Error ? err.message : 'Failed to add files')
    } finally {
      setActionLoading(false)
      setFileUploadActive(false)
    }
  }

  async function handleDeleteFile(fileId: number) {
    if (!dataset) return
    if ((dataset.files ?? []).length <= 1) {
      setActionError('Use Delete dataset instead of deleting the last file.')
      return
    }
    const confirmed = window.confirm('Delete this file and reprocess the dataset?')
    if (!confirmed) return

    setActionError('')
    setActionLoading(true)
    try {
      await deleteDatasetFile(fileId)
      setDataset({
        ...dataset,
        status: 'pending',
        processing_stage: 'file_edit_upload',
        files: dataset.files.filter((file) => file.id !== fileId),
      })
      await loadDataset(dataset.id, true)
    } catch (err) {
      setActionError(err instanceof Error ? err.message : 'Failed to delete file')
    } finally {
      setActionLoading(false)
    }
  }

  function renderGroupSample(group: NonNullable<Dataset['profile']>['groups'][number]) {
    const example = group.representative_examples?.find((f) => f.sample_rows?.length || f.sample_text?.length)
    if (!example) return null

    if (example.sample_rows && example.sample_rows.length > 0) {
      const columns = Object.keys(example.sample_rows[0]).slice(0, 8)
      return (
        <div className="profile-sample">
          <span className="profile-sample-title">Sample from {example.original_name}</span>
          <div className="profile-table-wrap">
            <table className="profile-table">
              <thead>
                <tr>
                  {columns.map((col) => <th key={col}>{col}</th>)}
                </tr>
              </thead>
              <tbody>
                {example.sample_rows.slice(0, 3).map((row, i) => (
                  <tr key={i}>
                    {columns.map((col) => <td key={col}>{row[col]}</td>)}
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </div>
      )
    }

    if (example.sample_text && example.sample_text.length > 0) {
      return (
        <div className="profile-sample">
          <span className="profile-sample-title">Sample from {example.original_name}</span>
          <pre className="profile-text-sample">
            {example.sample_text.slice(0, 5).join('\n')}
          </pre>
        </div>
      )
    }

    return null
  }

  if (loading) return <div className="loading-page">Loading dataset...</div>
  if (error) return <div className="error-page"><p>{error}</p><Link to="/">Back</Link></div>
  if (!dataset) return <div className="error-page"><p>Dataset not found</p><Link to="/">Back</Link></div>

  const aiCaveats = dataset.ai_caveats ?? []
  const tags = dataset.tags ?? []
  const labels = dataset.labels ?? []
  const labelFields = dataset.label_fields ?? []
  const pseudoQueries = dataset.pseudo_queries ?? []
  const files = dataset.files ?? []
  const profile = dataset.profile
  const canModify = Boolean(user && (user.role === 'admin' || user.id === dataset.owner_id))
  const canEditMetadata = canModify && (dataset.status === 'ready' || dataset.status === 'error')
  const canEditFiles = canEditMetadata

  return (
    <div className="detail-page">
      <div className="detail-header-row">
        <h1>{dataset.name}</h1>
        <StatusIndicator status={dataset.status} />
      </div>

      <main className="detail-main">
        {/* Left column: metadata */}
        <div className="detail-left">
          <section className="detail-section">
            <h2>Overview</h2>
            <p className="detail-researcher">
              Uploaded by <strong>{dataset.researcher_name}</strong>
              {' · '}
              {new Date(dataset.uploaded_at).toLocaleDateString('en-US', {
                year: 'numeric',
                month: 'long',
                day: 'numeric',
              })}
            </p>

            {dataset.description && (
              <div className="detail-description">
                <h3>Description</h3>
                <ExpandableText text={dataset.description} />
              </div>
            )}

            {dataset.ai_summary && (
              <div className="detail-ai-summary">
                <h3>AI Summary</h3>
                <ExpandableText text={dataset.ai_summary} />
              </div>
            )}
          </section>

          <section className="detail-section">
            <h2>Classification</h2>
            <div className="detail-meta-grid">
              {dataset.modality && (
                <div className="meta-item">
                  <span className="meta-label">Modality</span>
                  <span className="meta-value">{dataset.modality}</span>
                </div>
              )}
              {dataset.dataset_type && (
                <div className="meta-item">
                  <span className="meta-label">Type</span>
                  <span className="meta-value">{dataset.dataset_type}</span>
                </div>
              )}
              {dataset.annotation_format && (
                <div className="meta-item">
                  <span className="meta-label">Annotation Format</span>
                  <span className="meta-value">{dataset.annotation_format}</span>
                </div>
              )}
              {tags.length > 0 && (
                <div className="meta-item meta-item-tags">
                  <span className="meta-label">Tags</span>
                  <div className="detail-tags">
                    {tags.map((tag) => (
                      <span key={tag} className="meta-tag tag">{tag}</span>
                    ))}
                  </div>
                </div>
              )}
              <div className="meta-item">
                <span className="meta-label">Size</span>
                <span className="meta-value">{formatBytes(dataset.total_size_bytes)}</span>
              </div>
              <div className="meta-item completeness-item">
                <span className="meta-label">Label Completeness</span>
                {dataset.label_completeness > 0 ? (
                  <div className="completeness-bar">
                    <div
                      className="completeness-fill"
                      style={{ width: `${dataset.label_completeness * 100}%` }}
                    />
                    <span className="completeness-label">
                      {(dataset.label_completeness * 100).toFixed(0)}%
                    </span>
                  </div>
                  ) : (
                    <span className="completeness-label unknown">Unknown</span>
                )}
              </div>
              {dataset.ai_confidence > 0 && (
                <div className="meta-item completeness-item">
                  <span className="meta-label">AI Metadata Confidence</span>
                  <div className="completeness-bar confidence">
                    <div
                      className="completeness-fill"
                      style={{ width: `${dataset.ai_confidence * 100}%` }}
                    />
                    <span className="completeness-label">
                      {(dataset.ai_confidence * 100).toFixed(0)}%
                    </span>
                  </div>
                </div>
              )}
            </div>

            {aiCaveats.length > 0 && (
              <div className="ai-caveats">
                <h3>Metadata Caveats</h3>
                <ul>
                  {aiCaveats.map((caveat, i) => (
                    <li key={i}>{caveat}</li>
                  ))}
                </ul>
              </div>
            )}
          </section>

          {profile && (
            <section className="detail-section">
              <h2>Data Profile</h2>
              {(profile.notes ?? []).length > 0 && (
                <p className="section-hint">{profile.notes[0]}</p>
              )}

              <div className="profile-type-row">
                {(profile.file_types ?? []).map((type) => (
                  <span key={type.detected_type} className="meta-tag tag">
                    {type.file_count} {type.detected_type}
                  </span>
                ))}
              </div>

              {(profile.detected_patterns ?? []).length > 0 && (
                <div className="profile-patterns">
                  {(profile.detected_patterns ?? []).map((pattern) => (
                    <span key={pattern} className="meta-tag type">{pattern}</span>
                  ))}
                </div>
              )}

              {(profile.annotations ?? []).length > 0 && (
                <div className="annotation-summaries">
                  {(profile.annotations ?? []).map((annotation) => (
                    <article key={annotation.format} className="annotation-summary">
                      <div className="annotation-summary-header">
                        <h3>{annotation.format}</h3>
                        <p>
                          {annotation.class_count} class{annotation.class_count !== 1 ? 'es' : ''}
                          {annotation.total_annotations > 0 && (
                            <> &middot; {annotation.total_annotations} annotation{annotation.total_annotations !== 1 ? 's' : ''}</>
                          )}
                        </p>
                      </div>
                      {annotation.classes && annotation.classes.length > 0 && (
                        <div className="annotation-classes">
                          {annotation.classes.slice(0, 12).map((klass) => (
                            <span key={`${klass.id}-${klass.name}`} className="annotation-class">
                              {klass.name}
                              {klass.count > 0 && <small>{klass.count}</small>}
                            </span>
                          ))}
                        </div>
                      )}
                      {annotation.notes && annotation.notes.length > 0 && (
                        <p className="annotation-note">{annotation.notes[0]}</p>
                      )}
                    </article>
                  ))}
                </div>
              )}

              <div className="profile-groups">
                {(profile.groups ?? []).map((group) => (
                  <article key={group.key} className="profile-group">
                    <div className="profile-group-header">
                      <div>
                        <h3>{group.role} &middot; {group.detected_type}</h3>
                        <p>
                          {group.file_count} file{group.file_count !== 1 ? 's' : ''} &middot; {formatBytes(group.total_size_bytes)}
                        </p>
                      </div>
                    </div>

                    {group.shared_columns && group.shared_columns.length > 0 && (
                      <div className="profile-columns">
                        {group.shared_columns.slice(0, 12).map((col) => (
                          <span key={col.name} className="profile-column">
                            {col.name}
                            <small>{col.inferred_type}</small>
                          </span>
                        ))}
                        {group.shared_columns.length > 12 && (
                          <span className="profile-column more">
                            +{group.shared_columns.length - 12} columns
                          </span>
                        )}
                      </div>
                    )}

                    {renderGroupSample(group)}
                  </article>
                ))}
              </div>
            </section>
          )}

          {files.length > 0 && (
            <section className="detail-section">
              <div className="files-section-header">
                <h2>Files</h2>
                {canEditFiles && (
                  <>
                    <input
                      ref={addFileInputRef}
                      type="file"
                      multiple
                      onChange={handleAddFiles}
                      style={{ display: 'none' }}
                    />
                    <button
                      type="button"
                      className="btn-secondary"
                      onClick={() => addFileInputRef.current?.click()}
                      disabled={actionLoading}
                    >
                      Add files
                    </button>
                  </>
                )}
              </div>
              <div className="files-list">
                {files.map((f) => (
                  <div key={f.id} className="file-row">
                    <span className="file-name">{f.original_name}</span>
                    <span className="file-size">{formatBytes(f.size_bytes)}</span>
                    <div className="file-row-actions">
                      {dataset.status === 'ready' && (
                        <a
                          href={downloadUrl(dataset.id)}
                          className="file-download-btn"
                          download
                        >
                          Download
                        </a>
                      )}
                      {canEditFiles && (
                        <button
                          type="button"
                          className="file-delete-btn"
                          onClick={() => handleDeleteFile(f.id)}
                          disabled={actionLoading || files.length <= 1}
                          title={files.length <= 1 ? 'Use Delete dataset instead' : 'Delete file'}
                        >
                          Delete
                        </button>
                      )}
                    </div>
                  </div>
                ))}
              </div>
              {fileUploadActive && (
                <div className="file-edit-progress">
                  <UploadProgress
                    label={fileUploadLabel}
                    uploadedChunks={fileUploadedChunks}
                    totalChunks={fileTotalChunks}
                  />
                </div>
              )}
              {canEditFiles && files.length <= 1 && (
                <p className="management-hint">Use Delete dataset instead of deleting the last file.</p>
              )}
            </section>
          )}

          {files.length === 0 && canEditFiles && (
            <section className="detail-section">
              <div className="files-section-header">
                <h2>Files</h2>
                <input
                  ref={addFileInputRef}
                  type="file"
                  multiple
                  onChange={handleAddFiles}
                  style={{ display: 'none' }}
                />
                <button
                  type="button"
                  className="btn-secondary"
                  onClick={() => addFileInputRef.current?.click()}
                  disabled={actionLoading}
                >
                  Add files
                </button>
              </div>
              {fileUploadActive && (
                <div className="file-edit-progress">
                  <UploadProgress
                    label={fileUploadLabel}
                    uploadedChunks={fileUploadedChunks}
                    totalChunks={fileTotalChunks}
                  />
                </div>
              )}
            </section>
          )}
        </div>

        {/* Right column: labels and pseudo-queries */}
        <div className="detail-right">
          {(labels.length > 0 || labelFields.length > 0) && (
            <section className="detail-section">
              <h2>Labels</h2>
              <LabelBar labels={labels} labelFields={labelFields} />
            </section>
          )}

          {pseudoQueries.length > 0 && (
            <section className="detail-section">
              <h2>Suggested Queries</h2>
              <p className="section-hint">
                Example searches that will find this dataset
              </p>
              <ul className="pseudo-queries-list">
                {pseudoQueries.map((q, i) => (
                  <li key={i}>{q}</li>
                ))}
              </ul>
            </section>
          )}

          {dataset.status === 'ready' && (
            <div className="detail-download-section">
              <a
                href={downloadUrl(dataset.id)}
                className="btn-primary btn-large"
                download
              >
                Download Original Files
              </a>
              {dataset.export_status === 'ready' ? (
                <a
                  href={exportDownloadUrl(dataset.id)}
                  className="btn-primary btn-large ml-package-btn"
                  download
                >
                  Download ML Package (.zip)
                </a>
              ) : dataset.export_status === 'error' ? (
                <button
                  type="button"
                  className="btn-primary btn-large ml-package-btn ml-package-disabled"
                  disabled
                  title="The ML package could not be built for this dataset."
                >
                  ML Package Unavailable
                </button>
              ) : (
                <button
                  type="button"
                  className="btn-primary btn-large ml-package-btn ml-package-disabled"
                  disabled
                  title="README datasheet, manifest, raw files, train/val/test splits, and a rebuild script."
                >
                  <span className="ml-package-spinner" />
                  Preparing ML Package… {Math.round((dataset.export_progress || 0) * 100)}%
                </button>
              )}
              {dataset.export_status !== 'error' && (
                <p className="ml-package-hint">
                  The ML package bundles a datasheet, manifest with checksums, raw files,
                  train/val/test splits, and a deterministic rebuild script.
                </p>
              )}
            </div>
          )}

          {dataset.status === 'pending' && (
            <div className="detail-status-msg">
              <p>Files are still being uploaded. The dataset will be available once all uploads complete.</p>
            </div>
          )}

          {dataset.status === 'processing' && (
            <div className="detail-status-msg">
              <p>
                {dataset.processing_stage === 'profiling'
                  ? 'Profiling files and sampling representative records.'
                  : dataset.processing_stage === 'embedding'
                    ? 'Indexing the profile-enriched summary for semantic search.'
                    : 'AI analysis in progress. Labels, summary, and search metadata are being generated.'}
              </p>
              <div className="processing-spinner" />
            </div>
          )}

          {dataset.status === 'error' && dataset.error_message && (
            <div className="detail-status-msg error">
              <p className="error-label">AI analysis failed</p>
              <p className="error-summary">
                The uploaded file could not be processed automatically.
                Please check the file format and try uploading again.
              </p>
              <details className="error-details">
                <summary>Technical details</summary>
                <pre className="error-detail">{dataset.error_message}</pre>
              </details>
            </div>
          )}

          {dataset.status === 'error' && !dataset.error_message && (
            <div className="detail-status-msg error">
              <p className="error-label">Processing failed</p>
              <p className="error-summary">
                The uploaded file could not be processed automatically.
                Please check the file format and try uploading again.
              </p>
            </div>
          )}

          {canModify && (
            <section className="detail-section dataset-management">
              <h2>Manage</h2>
              {actionError && <div className="error-banner">{actionError}</div>}
              <div className="management-actions">
                <button
                  type="button"
                  className="btn-secondary"
                  onClick={openEditPanel}
                  disabled={!canEditMetadata || actionLoading}
                >
                  Edit dataset
                </button>
                <button
                  type="button"
                  className="btn-danger"
                  onClick={() => {
                    setActionError('')
                    setEditOpen(false)
                    setDeleteConfirmOpen((open) => !open)
                  }}
                  disabled={actionLoading}
                >
                  Delete dataset
                </button>
              </div>

              {!canEditMetadata && (
                <p className="management-hint">Metadata can be edited after upload processing finishes.</p>
              )}

              {editOpen && (
                <form className="dataset-edit-panel" onSubmit={handleSaveEdit}>
                  <div className="form-group">
                    <label htmlFor="edit-ds-name">Name</label>
                    <input
                      id="edit-ds-name"
                      type="text"
                      value={editName}
                      onChange={(e) => setEditName(e.target.value)}
                      required
                    />
                  </div>
                  <div className="form-group">
                    <label htmlFor="edit-ds-researcher">Researcher Name</label>
                    <input
                      id="edit-ds-researcher"
                      type="text"
                      value={editResearcher}
                      onChange={(e) => setEditResearcher(e.target.value)}
                      required
                    />
                  </div>
                  <div className="form-group">
                    <label htmlFor="edit-ds-description">Description</label>
                    <textarea
                      id="edit-ds-description"
                      value={editDescription}
                      onChange={(e) => setEditDescription(e.target.value)}
                      rows={4}
                    />
                  </div>
                  <div className="form-group">
                    <label htmlFor="edit-ds-tags">Tags</label>
                    <input
                      id="edit-ds-tags"
                      type="text"
                      value={editTags}
                      onChange={(e) => setEditTags(e.target.value)}
                    />
                  </div>
                  <div className="panel-actions">
                    <button type="submit" className="btn-primary" disabled={actionLoading}>
                      {actionLoading ? 'Saving...' : 'Save'}
                    </button>
                    <button
                      type="button"
                      className="btn-secondary"
                      onClick={() => setEditOpen(false)}
                      disabled={actionLoading}
                    >
                      Cancel
                    </button>
                  </div>
                </form>
              )}

              {deleteConfirmOpen && (
                <div className="delete-confirm-panel">
                  <p>Are you sure? This cannot be undone.</p>
                  <div className="panel-actions">
                    <button
                      type="button"
                      className="btn-danger solid"
                      onClick={handleDeleteDataset}
                      disabled={actionLoading}
                    >
                      {actionLoading ? 'Deleting...' : 'Delete'}
                    </button>
                    <button
                      type="button"
                      className="btn-secondary"
                      onClick={() => setDeleteConfirmOpen(false)}
                      disabled={actionLoading}
                    >
                      Cancel
                    </button>
                  </div>
                </div>
              )}
            </section>
          )}
        </div>
      </main>
    </div>
  )
}
