# internal/filehandler

This directory contains the **file handler** — the component that manages all file movement into and out of the platform.

## What the file handler does

- Receives uploaded files in small chunks to avoid timeouts and memory pressure.
- Verifies that authenticated users own the target dataset, or are admins, before accepting registrations or chunks.
- Assembles chunks into final files on disk.
- Triggers the AI pipeline once **all** files for a dataset are fully uploaded.
- Streams downloads directly from disk so even multi-gigabyte files do not consume RAM.
- Packages multi-file datasets into ZIP archives on-the-fly.

## Files

| File | Purpose |
|---|---|
| `types.go` | Defines `AssembledFile` (one completed file) and `Handler` (the file I/O manager with the `OnFileAssembled` callback hook). |
| `handler_core.go` | Constructor `NewHandler` that creates the storage root and `_tmp` subfolder. |
| `helpers.go` | Shared utilities: JSON writing, filename sanitization, chunk assembly, cleanup, and database helpers (`allFilesComplete`, `loadAssembledFiles`). |
| `register.go` | `POST /api/files/register` handler. Pre-creates a `files` row so the client gets a `file_id` before sending chunks. |
| `upload.go` | `POST /api/files/chunk` handler. Receives one chunk at a time, writes it to temp, atomically claims final assembly, and fires `OnFileAssembled` when the whole dataset is complete. |
| `download.go` | `GET /api/datasets/:id/download` handler. Streams a single file directly or builds a ZIP archive for multi-file datasets. |

## Upload flow

1. Authenticated client calls `POST /api/datasets` → receives `dataset_id`.
2. Client calls `POST /api/files/register` for each file → receives `file_id`.
3. Client sends chunks via `POST /api/files/chunk` (`file_id`, `chunk_index`, `total_chunks`, binary chunk).
4. On the final chunk, the handler changes the file row to `assembling`, assembles the file, marks it `complete`, and checks if **all** files for that dataset are complete.
5. If yes, it calls `OnFileAssembled(datasetID, files)` in a goroutine to start the AI pipeline.

## Download behavior

- **Single-file dataset** — streams directly with `Content-Disposition` and `Content-Type` headers.
- **Multi-file dataset** — streams a ZIP archive created on-the-fly with `zip.Deflate` compression.
- In both cases, `io.Copy` with a 32 KB buffer is used so memory stays flat regardless of file size.

## Storage layout

- `storageDir` — completed files live here.
- `storageDir/_tmp` — in-progress chunks are staged here and cleaned up after assembly.
