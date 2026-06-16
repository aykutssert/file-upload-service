import {
  confirmPart,
  confirmUpload,
  completeMultipartSession,
  createMultipartSession,
  createUploadSession,
  DEFAULT_PART_SIZE,
  listParts,
  MULTIPART_THRESHOLD,
  presignPart,
} from '../api/client'
import { Semaphore } from './semaphore'
import type { UploadAction } from './types'

const CONCURRENT_PARTS = 3

function generateIdempotencyKey(): string {
  return `${Date.now()}-${Math.random().toString(36).slice(2)}`
}

function getMimeType(file: File): string {
  return file.type || 'application/octet-stream'
}

export async function runSinglePartUpload(
  localId: string,
  file: File,
  dispatch: (a: UploadAction) => void,
  signal: AbortSignal,
): Promise<void> {
  dispatch({ type: 'SET_STATUS', id: localId, status: 'creating' })

  const idempotencyKey = generateIdempotencyKey()
  const session = await createUploadSession(
    file.name,
    getMimeType(file),
    file.size,
    idempotencyKey,
    signal,
  )

  dispatch({ type: 'SET_SESSION', id: localId, sessionId: session.id, mode: 'single' })
  dispatch({ type: 'SET_STATUS', id: localId, status: 'uploading' })

  const startedAt = Date.now()
  let lastBytes = 0
  let lastTime = startedAt

  await uploadWithProgress(session.upload_url, session.upload_method, session.upload_headers, file, signal, (loaded) => {
    dispatch({ type: 'SET_PROGRESS', id: localId, bytesUploaded: loaded })

    const now = Date.now()
    const elapsed = (now - lastTime) / 1000
    if (elapsed > 0.5) {
      const bps = (loaded - lastBytes) / elapsed
      const remaining = file.size - loaded
      const eta = bps > 0 ? remaining / bps : 0
      dispatch({ type: 'SET_THROUGHPUT', id: localId, throughputBps: bps, etaSeconds: eta })
      lastBytes = loaded
      lastTime = now
    }
  })

  dispatch({ type: 'SET_STATUS', id: localId, status: 'completing' })
  const result = await confirmUpload(session.id, signal)
  dispatch({ type: 'SET_FILE_ID', id: localId, fileId: result.id })
  dispatch({ type: 'SET_PROGRESS', id: localId, bytesUploaded: file.size })
  dispatch({ type: 'SET_STATUS', id: localId, status: 'done' })
}

export async function runMultipartUpload(
  localId: string,
  file: File,
  dispatch: (a: UploadAction) => void,
  signal: AbortSignal,
): Promise<void> {
  dispatch({ type: 'SET_STATUS', id: localId, status: 'creating' })

  const partSize = DEFAULT_PART_SIZE
  const totalParts = Math.ceil(file.size / partSize)
  const idempotencyKey = generateIdempotencyKey()

  const session = await createMultipartSession(
    file.name,
    getMimeType(file),
    file.size,
    partSize,
    idempotencyKey,
    signal,
  )

  dispatch({ type: 'SET_SESSION', id: localId, sessionId: session.id, mode: 'multipart' })
  dispatch({ type: 'INIT_PARTS', id: localId, count: totalParts, partSize, totalBytes: file.size })
  dispatch({ type: 'SET_STATUS', id: localId, status: 'uploading' })

  // Resume: find already uploaded parts.
  const { parts: confirmedParts } = await listParts(session.id, signal)
  const confirmedSet = new Set(confirmedParts.map(p => p.part_number))

  const sem = new Semaphore(CONCURRENT_PARTS)
  const startedAt = Date.now()

  const tasks = Array.from({ length: totalParts }, (_, i) => {
    const partNumber = i + 1
    return sem.run(async () => {
      if (signal.aborted) throw new DOMException('Aborted', 'AbortError')
      if (confirmedSet.has(partNumber)) {
        dispatch({ type: 'COMPLETE_PART', id: localId, partNumber })
        return
      }

      const offset = (partNumber - 1) * partSize
      const size = Math.min(partSize, file.size - offset)
      const chunk = file.slice(offset, offset + size)

      dispatch({ type: 'SET_PART_STATUS', id: localId, partNumber, status: 'uploading' })

      const { upload_url, upload_method, upload_headers } = await presignPart(
        session.id,
        partNumber,
        size,
        signal,
      )

      const etag = await uploadPartWithProgress(
        upload_url,
        upload_method,
        upload_headers,
        chunk,
        signal,
        (loaded) => {
          dispatch({ type: 'SET_PART_STATUS', id: localId, partNumber, status: 'uploading', loaded })
          const elapsed = (Date.now() - startedAt) / 1000
          const totalDone = confirmedSet.size * partSize
          const bps = elapsed > 0 ? totalDone / elapsed : 0
          const remaining = file.size - totalDone
          const eta = bps > 0 ? remaining / bps : 0
          dispatch({ type: 'SET_THROUGHPUT', id: localId, throughputBps: bps, etaSeconds: eta })
        },
      )

      await confirmPart(session.id, partNumber, etag, size, signal)
      confirmedSet.add(partNumber)
      dispatch({ type: 'COMPLETE_PART', id: localId, partNumber })
    })
  })

  await Promise.all(tasks)

  dispatch({ type: 'SET_STATUS', id: localId, status: 'completing' })
  const result = await completeMultipartSession(session.id, signal)
  dispatch({ type: 'SET_FILE_ID', id: localId, fileId: result.file_id })
  dispatch({ type: 'SET_PROGRESS', id: localId, bytesUploaded: file.size })
  dispatch({ type: 'SET_STATUS', id: localId, status: 'done' })
}

export function shouldUseMultipart(file: File): boolean {
  return file.size >= MULTIPART_THRESHOLD
}

// Upload via presigned URL and report progress via XHR.
function uploadWithProgress(
  url: string,
  method: string,
  headers: Record<string, string>,
  body: Blob,
  signal: AbortSignal,
  onProgress: (loaded: number) => void,
): Promise<void> {
  return new Promise((resolve, reject) => {
    const xhr = new XMLHttpRequest()
    xhr.open(method, url)
    for (const [k, v] of Object.entries(headers)) xhr.setRequestHeader(k, v)

    xhr.upload.onprogress = e => { if (e.lengthComputable) onProgress(e.loaded) }
    xhr.onload = () => {
      if (xhr.status >= 200 && xhr.status < 300) resolve()
      else reject(new Error(`Upload failed: ${xhr.status}`))
    }
    xhr.onerror = () => reject(new Error('Network error'))
    xhr.onabort = () => reject(new DOMException('Aborted', 'AbortError'))
    signal.addEventListener('abort', () => xhr.abort(), { once: true })
    xhr.send(body)
  })
}

// Upload a part and return the ETag from the response header.
function uploadPartWithProgress(
  url: string,
  method: string,
  headers: Record<string, string>,
  body: Blob,
  signal: AbortSignal,
  onProgress: (loaded: number) => void,
): Promise<string> {
  return new Promise((resolve, reject) => {
    const xhr = new XMLHttpRequest()
    xhr.open(method, url)
    for (const [k, v] of Object.entries(headers)) xhr.setRequestHeader(k, v)

    xhr.upload.onprogress = e => { if (e.lengthComputable) onProgress(e.loaded) }
    xhr.onload = () => {
      if (xhr.status >= 200 && xhr.status < 300) {
        const etag = xhr.getResponseHeader('ETag') ?? ''
        resolve(etag)
      } else {
        reject(new Error(`Part upload failed: ${xhr.status}`))
      }
    }
    xhr.onerror = () => reject(new Error('Network error'))
    xhr.onabort = () => reject(new DOMException('Aborted', 'AbortError'))
    signal.addEventListener('abort', () => xhr.abort(), { once: true })
    xhr.send(body)
  })
}
