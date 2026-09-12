/*
  API client — thin fetch wrappers around every backend endpoint.

  All functions return typed responses and throw on non-2xx status
  so callers can use try/catch uniformly.

  The base URL is configurable via import.meta.env.VITE_API_URL
  and defaults to http://localhost:8080 for local development.
*/

import type {
  Dataset,
  DatasetSummary,
  SearchResponse,
  SearchFilters,
  CreateDatasetRequest,
  CreateDatasetResponse,
  UpdateDatasetRequest,
  RegisterFileResponse,
  ChunkUploadResponse,
  AuthUser,
  LoginResponse,
} from '../types'

export const BASE = import.meta.env.VITE_API_URL || `${window.location.protocol}//${window.location.hostname}:8081`

const TOKEN_KEY = 'labdatasets_token'

export function getAuthToken(): string | null {
  return localStorage.getItem(TOKEN_KEY)
}

export function setAuthToken(token: string): void {
  localStorage.setItem(TOKEN_KEY, token)
}

export function clearAuthToken(): void {
  localStorage.removeItem(TOKEN_KEY)
}

function authHeader(): Record<string, string> {
  const token = getAuthToken()
  return token ? { Authorization: `Bearer ${token}` } : {}
}

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await fetch(`${BASE}${path}`, {
    ...init,
    headers: {
      'Content-Type': 'application/json',
      ...authHeader(),
      ...init?.headers,
    },
  })
  if (!res.ok) {
    if (res.status === 401) clearAuthToken()
    const body = await res.text()
    throw new Error(`API ${res.status}: ${body}`)
  }
  if (res.status === 204) {
    return undefined as T
  }
  return res.json()
}

// ── Health ────────────────────────────────────────────────────────────────────

export async function healthCheck(): Promise<{ status: string; time: string }> {
  return request('/api/health')
}

// ── Datasets ──────────────────────────────────────────────────────────────────

export async function listDatasets(): Promise<DatasetSummary[]> {
  const datasets = await request<DatasetSummary[] | null>('/api/datasets')
  return Array.isArray(datasets) ? datasets.map(normalizeDatasetSummary) : []
}

export async function createDataset(
  body: CreateDatasetRequest,
): Promise<CreateDatasetResponse> {
  return request('/api/datasets', {
    method: 'POST',
    body: JSON.stringify(body),
  })
}

export async function getDataset(id: number): Promise<Dataset> {
  const dataset = await request<Dataset>(`/api/datasets/${id}`)
  return normalizeDataset(dataset)
}

export async function updateDataset(
  id: number,
  body: UpdateDatasetRequest,
): Promise<Dataset> {
  const dataset = await request<Dataset>(`/api/datasets/${id}`, {
    method: 'PUT',
    body: JSON.stringify(body),
  })
  return normalizeDataset(dataset)
}

export async function deleteDataset(id: number): Promise<void> {
  await request<void>(`/api/datasets/${id}`, { method: 'DELETE' })
}

export async function deleteDatasetFile(fileId: number): Promise<void> {
  await request<void>(`/api/files/${fileId}`, { method: 'DELETE' })
}

// ── Files ─────────────────────────────────────────────────────────────────────

export async function registerFile(
  datasetId: number,
  originalName: string,
  mimeType: string = '',
): Promise<RegisterFileResponse> {
  return request('/api/files/register', {
    method: 'POST',
    body: JSON.stringify({
      dataset_id: datasetId,
      original_name: originalName,
      mime_type: mimeType,
    }),
  })
}

export async function registerAdditionalFile(
  datasetId: number,
  originalName: string,
  mimeType: string = '',
): Promise<RegisterFileResponse> {
  return request(`/api/datasets/${datasetId}/files/register`, {
    method: 'POST',
    body: JSON.stringify({
      original_name: originalName,
      mime_type: mimeType,
    }),
  })
}

export async function uploadChunk(
  fileId: number,
  chunkIndex: number,
  totalChunks: number,
  chunkData: Blob,
): Promise<ChunkUploadResponse> {
  const form = new FormData()
  form.append('file_id', String(fileId))
  form.append('chunk_index', String(chunkIndex))
  form.append('total_chunks', String(totalChunks))
  form.append('chunk', chunkData)

  const res = await fetch(`${BASE}/api/files/chunk`, {
    method: 'POST',
    headers: authHeader(),
    body: form,
  })
  if (!res.ok) {
    if (res.status === 401) clearAuthToken()
    const body = await res.text()
    throw new Error(`Chunk upload ${res.status}: ${body}`)
  }
  return res.json()
}

export function downloadUrl(datasetId: number): string {
  return `${BASE}/api/datasets/${datasetId}/download`
}

// URL of the prebuilt ML package zip (README datasheet + manifest + raw/ +
// processed splits + build script). Only valid once export_status === 'ready'.
export function exportDownloadUrl(datasetId: number): string {
  return `${BASE}/api/datasets/${datasetId}/export`
}

// Auth

export async function login(username: string, password: string): Promise<LoginResponse> {
  return request('/api/auth/login', {
    method: 'POST',
    body: JSON.stringify({ username, password }),
  })
}

export async function getCurrentUser(): Promise<AuthUser> {
  return request('/api/auth/me')
}

export async function createUser(body: {
  username: string
  display_name: string
  password: string
  role: AuthUser['role']
}): Promise<AuthUser> {
  return request('/api/admin/users', {
    method: 'POST',
    body: JSON.stringify(body),
  })
}

export async function listUsers(): Promise<AuthUser[]> {
  return request('/api/admin/users')
}

export async function deleteUser(id: string): Promise<void> {
  await request<void>(`/api/admin/users/${id}`, { method: 'DELETE' })
}

// ── Search ────────────────────────────────────────────────────────────────────

export async function search(
  query: string,
  filters: Partial<SearchFilters> = {},
): Promise<SearchResponse> {
  const params = new URLSearchParams()

  // Query is optional - user can filter by metadata without keyword search
  if (query) params.set('q', query)

  if (filters.modality) params.set('modality', filters.modality)
  if (filters.dataset_type) params.set('dataset_type', filters.dataset_type)
  if (filters.annotation_format) params.set('annotation_format', filters.annotation_format)
  if (filters.min_size) params.set('min_size', filters.min_size)
  if (filters.max_size) params.set('max_size', filters.max_size)
  if (filters.min_label_completeness) params.set('min_label_completeness', filters.min_label_completeness)
  if (filters.max_label_completeness) params.set('max_label_completeness', filters.max_label_completeness)
  if (filters.uploaded_after) params.set('uploaded_after', filters.uploaded_after)
  if (filters.uploaded_before) params.set('uploaded_before', filters.uploaded_before)

  const response = await request<SearchResponse | null>(`/api/search?${params.toString()}`)
  return {
    query: response?.query ?? query,
    count: response?.count ?? 0,
    results: Array.isArray(response?.results)
      ? response.results.map(normalizeSearchResult)
      : [],
  }
}

function normalizeDatasetSummary(dataset: DatasetSummary): DatasetSummary {
  return {
    ...dataset,
    owner_id: dataset.owner_id ?? null,
    owner_display_name: dataset.owner_display_name ?? '',
    tags: dataset.tags ?? [],
    label_completeness: Number(dataset.label_completeness) || 0,
    total_size_bytes: Number(dataset.total_size_bytes) || 0,
  }
}

function normalizeSearchResult(result: SearchResponse['results'][number]): SearchResponse['results'][number] {
  return {
    ...result,
    tags: result.tags ?? [],
    label_completeness: Number(result.label_completeness) || 0,
    total_size_bytes: Number(result.total_size_bytes) || 0,
    keyword_score: result.keyword_score === undefined ? undefined : Number(result.keyword_score) || 0,
    semantic_score: result.semantic_score === undefined ? undefined : Number(result.semantic_score) || 0,
  }
}

function normalizeDataset(dataset: Dataset): Dataset {
  return {
    ...dataset,
    owner_id: dataset.owner_id ?? null,
    owner_display_name: dataset.owner_display_name ?? '',
    ai_caveats: dataset.ai_caveats ?? [],
    tags: dataset.tags ?? [],
    labels: dataset.labels ?? [],
    label_fields: dataset.label_fields ?? [],
    pseudo_queries: dataset.pseudo_queries ?? [],
    files: dataset.files ?? [],
    label_completeness: Number(dataset.label_completeness) || 0,
    ai_confidence: Number(dataset.ai_confidence) || 0,
    total_size_bytes: Number(dataset.total_size_bytes) || 0,
    profile: dataset.profile
      ? {
          ...dataset.profile,
          file_types: dataset.profile.file_types ?? [],
          groups: dataset.profile.groups ?? [],
          files: dataset.profile.files ?? [],
          annotations: dataset.profile.annotations ?? [],
          notes: dataset.profile.notes ?? [],
          detected_patterns: dataset.profile.detected_patterns ?? [],
        }
      : null,
  }
}
