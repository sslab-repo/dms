import { uploadChunk } from '../api/client'
import type { RegisterFileResponse } from '../types'

const BYTES_PER_MB = 1024 * 1024
const DEFAULT_CHUNK_SIZE_MB = 10
const DEFAULT_MAX_CONCURRENT_CHUNKS = 4
const DEFAULT_MAX_CONCURRENT_FILES = 2
const MAX_CHUNK_SIZE_MB = 64
const CHUNK_UPLOAD_ATTEMPTS = 3

interface UploadConfig {
  chunkSizeBytes: number
  maxConcurrentChunks: number
  maxConcurrentFiles: number
}

interface FileUploadPlan {
  file: File
  fileId: number
  totalChunks: number
}

export type RegisterDatasetFile = (
  datasetId: number,
  file: File,
) => Promise<RegisterFileResponse>

export interface UploadFilesCallbacks {
  onStart?: (label: string, totalChunks: number) => void
  onChunkComplete?: (completedChunks: number) => void
}

function readPositiveInt(value: unknown, fallback: number): number {
  const parsed = Number(value)
  if (!Number.isFinite(parsed) || parsed < 1) return fallback
  return Math.floor(parsed)
}

function clamp(value: number, min: number, max: number): number {
  return Math.min(Math.max(value, min), max)
}

function readUploadConfig(): UploadConfig {
  const chunkSizeMB = clamp(
    readPositiveInt(import.meta.env.VITE_UPLOAD_CHUNK_SIZE_MB, DEFAULT_CHUNK_SIZE_MB),
    1,
    MAX_CHUNK_SIZE_MB,
  )

  return {
    chunkSizeBytes: chunkSizeMB * BYTES_PER_MB,
    maxConcurrentChunks: readPositiveInt(
      import.meta.env.VITE_UPLOAD_MAX_CONCURRENT_CHUNKS,
      DEFAULT_MAX_CONCURRENT_CHUNKS,
    ),
    maxConcurrentFiles: readPositiveInt(
      import.meta.env.VITE_UPLOAD_MAX_CONCURRENT_FILES,
      DEFAULT_MAX_CONCURRENT_FILES,
    ),
  }
}

const uploadConfig = readUploadConfig()

async function runPool<T>(
  items: T[],
  concurrency: number,
  worker: (item: T) => Promise<void>,
): Promise<void> {
  let nextIndex = 0
  let firstError: unknown = null
  const workerCount = Math.min(Math.max(1, concurrency), items.length)

  async function runWorker() {
    while (firstError === null) {
      const itemIndex = nextIndex
      nextIndex += 1
      if (itemIndex >= items.length) return

      try {
        await worker(items[itemIndex])
      } catch (err) {
        firstError = err
        return
      }
    }
  }

  await Promise.all(Array.from({ length: workerCount }, runWorker))
  if (firstError !== null) throw firstError
}

export async function uploadFilesForDataset(
  datasetId: number,
  files: File[],
  registerFileForDataset: RegisterDatasetFile,
  callbacks: UploadFilesCallbacks = {},
): Promise<void> {
  const prepared = files.map((file) => ({
    file,
    totalChunks: Math.max(1, Math.ceil(file.size / uploadConfig.chunkSizeBytes)),
  }))
  const chunkTotal = prepared.reduce((sum, plan) => sum + plan.totalChunks, 0)
  const label = files.length === 1 ? `Uploading ${files[0].name}` : `Uploading ${files.length} files`
  callbacks.onStart?.(label, chunkTotal)

  const plans: FileUploadPlan[] = []
  for (const plan of prepared) {
    const res = await registerFileForDataset(datasetId, plan.file)
    plans.push({ ...plan, fileId: res.file_id })
  }

  let completedChunks = 0
  const markChunkComplete = () => {
    completedChunks += 1
    callbacks.onChunkComplete?.(completedChunks)
  }

  await runPool(plans, uploadConfig.maxConcurrentFiles, async (plan) => {
    await uploadSingleFile(plan, markChunkComplete)
  })
}

async function uploadSingleFile(
  plan: FileUploadPlan,
  markChunkComplete: () => void,
) {
  const { file, fileId, totalChunks } = plan

  if (totalChunks === 1) {
    await uploadChunkWithRetry(file, fileId, 0, totalChunks)
    markChunkComplete()
    return
  }

  const nonFinalChunkIndexes = Array.from(
    { length: totalChunks - 1 },
    (_, index) => index,
  )

  await runPool(nonFinalChunkIndexes, uploadConfig.maxConcurrentChunks, async (chunkIndex) => {
    await uploadChunkWithRetry(file, fileId, chunkIndex, totalChunks)
    markChunkComplete()
  })

  await uploadChunkWithRetry(file, fileId, totalChunks - 1, totalChunks)
  markChunkComplete()
}

async function uploadChunkWithRetry(
  file: File,
  fileId: number,
  chunkIndex: number,
  totalChunks: number,
) {
  const start = chunkIndex * uploadConfig.chunkSizeBytes
  const end = Math.min(start + uploadConfig.chunkSizeBytes, file.size)
  const chunkBlob = file.slice(start, end)

  for (let attempt = 1; attempt <= CHUNK_UPLOAD_ATTEMPTS; attempt++) {
    try {
      await uploadChunk(fileId, chunkIndex, totalChunks, chunkBlob)
      return
    } catch (err) {
      if (attempt === CHUNK_UPLOAD_ATTEMPTS) {
        const detail = err instanceof Error ? err.message : 'unknown error'
        throw new Error(`Chunk ${chunkIndex + 1} of ${file.name} failed after ${CHUNK_UPLOAD_ATTEMPTS} attempts: ${detail}`)
      }
    }
  }
}
