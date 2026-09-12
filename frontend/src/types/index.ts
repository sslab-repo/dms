/*
  Shared TypeScript types that mirror the backend JSON responses.
  Every API wrapper returns these types so the rest of the app
  works with strongly-typed data throughout.
*/

export interface Dataset {
    id: number
    name: string
    researcher_name: string
    owner_id: string | null
    owner_display_name: string
    description: string
    ai_summary: string
    modality: string
    dataset_type: string
    annotation_format: string
    label_completeness: number
    ai_confidence: number
    ai_caveats: string[]
    total_size_bytes: number
    status: 'pending' | 'processing' | 'ready' | 'error'
    processing_stage: string
    export_status: 'none' | 'building' | 'ready' | 'error'
    export_progress: number
    uploaded_at: string
    error_message: string
    tags: string[]
    labels: Label[]
    label_fields: LabelField[]
    pseudo_queries: string[]
    files: FileInfo[]
    profile: DatasetProfile | null
}

export interface DatasetSummary {
    id: number
    name: string
    researcher_name: string
    owner_id: string | null
    owner_display_name: string
    ai_summary: string
    modality: string
    dataset_type: string
    label_completeness: number
    total_size_bytes: number
    status: string
    uploaded_at: string
    tags: string[]
}

export interface Label {
  name: string
  proportion: number | null
  sample_count: number
}

export interface LabelField {
  name: string
  non_empty_count: number
  empty_count: number
  completeness: number
  examples: string[]
}

export interface FileInfo {
  id: number
  original_name: string
  size_bytes: number
  mime_type: string
  upload_status: string
}

export interface SearchResult {
  dataset_id: number
  name: string
  researcher_name: string
  ai_summary: string
  modality: string
  dataset_type: string
  annotation_format: string
  label_completeness: number
  total_size_bytes: number
  tags: string[]
  fusion_score: number
  keyword_score?: number
  semantic_score?: number
  citation: 'keyword' | 'semantic' | 'hybrid'
  uploaded_at: string
}

export interface SearchResponse {
  query: string
  count: number
  results: SearchResult[]
}

export interface SearchFilters {
  modality: string
  dataset_type: string
  annotation_format: string
  min_size: string
  max_size: string
  min_label_completeness: string
  max_label_completeness: string
  uploaded_after: string   // YYYY-MM-DD
  uploaded_before: string  // YYYY-MM-DD
}

export interface CreateDatasetRequest {
	name: string
	researcher_name: string
	uploader_email: string
	user_description: string
	tags: string[]
	total_files: number
	label_column?: string
}

export interface CreateDatasetResponse {
  dataset_id: number
  status: string
  message: string
}

export interface UpdateDatasetRequest {
  name: string
  researcher_name: string
  user_description: string
  tags: string[]
  label_column?: string
}

export interface AuthUser {
  id: string
  username: string
  display_name: string
  role: 'admin' | 'researcher'
}

export interface LoginResponse {
  token: string
  user: AuthUser
}

export interface RegisterFileResponse {
  file_id: number
  message: string
}

export interface ChunkUploadResponse {
  file_id: number
  chunk_index: number
  total_chunks: number
  done: boolean
  all_done: boolean
  error?: string
}

export interface DatasetProfile {
  version: string
  generated_at: string
  total_files: number
  total_size_bytes: number
  file_types: TypeSummary[]
  groups: FileGroup[]
  files: FileProfile[]
  annotations?: AnnotationProfile[]
  notes: string[]
  detected_patterns: string[]
}

export interface TypeSummary {
  detected_type: string
  file_count: number
  total_size_bytes: number
}

export interface FileGroup {
  key: string
  detected_type: string
  role: string
  file_count: number
  total_size_bytes: number
  shared_columns?: ColumnProfile[]
  representative_file_ids: number[]
  representative_examples: FileProfile[]
}

export interface FileProfile {
  file_id: number
  original_name: string
  extension: string
  size_bytes: number
  mime_type?: string
  detected_type: string
  role: string
  sampled_rows?: number
  columns?: ColumnProfile[]
  sample_rows?: Record<string, string>[]
  sample_text?: string[]
  annotation?: AnnotationProfile
  warnings?: string[]
}

export interface ColumnProfile {
  name: string
  inferred_type: string
  non_empty_count: number
  empty_count: number
  example_values?: string[]
}

export interface AnnotationProfile {
  format: string
  source_files: string[]
  class_count: number
  total_annotations: number
  classes?: ClassProfile[]
  notes?: string[]
}

export interface ClassProfile {
  id?: string
  name: string
  count: number
  proportion?: number
}
