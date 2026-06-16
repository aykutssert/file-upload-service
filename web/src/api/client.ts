import type {
  CompleteMultipartResponse,
  CompleteUploadResponse,
  DownloadResponse,
  ListFilesResponse,
  ListPartsResponse,
  MultipartSession,
  Part,
  PresignPartResponse,
  UploadSession,
} from './types'

export const MULTIPART_THRESHOLD = 20 * 1024 * 1024  // 20 MB
export const DEFAULT_PART_SIZE = 10 * 1024 * 1024    // 10 MB

let apiKey = ''

export function setApiKey(key: string) {
  apiKey = key
}

export function getApiKey(): string {
  return apiKey
}

async function request<T>(
  method: string,
  path: string,
  options: {
    body?: unknown
    headers?: Record<string, string>
    signal?: AbortSignal
  } = {},
): Promise<T> {
  const res = await fetch(path, {
    method,
    headers: {
      'Content-Type': 'application/json',
      Authorization: `Bearer ${apiKey}`,
      ...options.headers,
    },
    body: options.body !== undefined ? JSON.stringify(options.body) : undefined,
    signal: options.signal,
  })
  if (!res.ok) {
    const payload = await res.json().catch(() => ({}))
    const code = payload?.error?.code ?? 'unknown_error'
    const message = payload?.error?.message ?? `HTTP ${res.status}`
    throw Object.assign(new Error(message), { code, status: res.status })
  }
  if (res.status === 204) return undefined as T
  return res.json() as Promise<T>
}

// Single-part
export function createUploadSession(
  originalName: string,
  contentType: string,
  expectedSize: number,
  idempotencyKey: string,
  signal?: AbortSignal,
): Promise<UploadSession> {
  return request<UploadSession>('POST', '/v1/upload-sessions', {
    body: { original_name: originalName, content_type: contentType, expected_size: expectedSize },
    headers: { 'Idempotency-Key': idempotencyKey },
    signal,
  })
}

export function confirmUpload(
  id: string,
  signal?: AbortSignal,
): Promise<CompleteUploadResponse> {
  return request<CompleteUploadResponse>('POST', `/v1/files/${id}/complete`, { signal })
}

// Multipart
export function createMultipartSession(
  originalName: string,
  contentType: string,
  expectedSize: number,
  partSize: number,
  idempotencyKey: string,
  signal?: AbortSignal,
): Promise<MultipartSession> {
  return request<MultipartSession>('POST', '/v1/multipart-sessions', {
    body: {
      original_name: originalName,
      content_type: contentType,
      expected_size: expectedSize,
      part_size: partSize,
    },
    headers: { 'Idempotency-Key': idempotencyKey },
    signal,
  })
}

export function presignPart(
  sessionId: string,
  partNumber: number,
  size: number,
  signal?: AbortSignal,
): Promise<PresignPartResponse> {
  return request<PresignPartResponse>(
    'GET',
    `/v1/multipart-sessions/${sessionId}/parts/${partNumber}?size=${size}`,
    { signal },
  )
}

export function confirmPart(
  sessionId: string,
  partNumber: number,
  etag: string,
  size: number,
  signal?: AbortSignal,
): Promise<Part> {
  return request<Part>('POST', `/v1/multipart-sessions/${sessionId}/parts/${partNumber}`, {
    body: { etag, size },
    signal,
  })
}

export function listParts(
  sessionId: string,
  signal?: AbortSignal,
): Promise<ListPartsResponse> {
  return request<ListPartsResponse>('GET', `/v1/multipart-sessions/${sessionId}/parts`, { signal })
}

export function completeMultipartSession(
  sessionId: string,
  signal?: AbortSignal,
): Promise<CompleteMultipartResponse> {
  return request<CompleteMultipartResponse>(
    'POST',
    `/v1/multipart-sessions/${sessionId}/complete`,
    { signal },
  )
}

export function abortMultipartSession(sessionId: string): Promise<void> {
  return request<void>('DELETE', `/v1/multipart-sessions/${sessionId}`)
}

// Files
export function listFiles(cursor?: string, signal?: AbortSignal): Promise<ListFilesResponse> {
  const url = cursor ? `/v1/files?cursor=${encodeURIComponent(cursor)}` : '/v1/files'
  return request<ListFilesResponse>('GET', url, { signal })
}

export function getDownloadUrl(fileId: string, signal?: AbortSignal): Promise<DownloadResponse> {
  return request<DownloadResponse>('GET', `/v1/files/${fileId}/download`, { signal })
}

export function deleteFile(fileId: string, signal?: AbortSignal): Promise<void> {
  return request<void>('DELETE', `/v1/files/${fileId}`, { signal })
}
