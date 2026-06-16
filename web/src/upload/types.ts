export type UploadMode = 'single' | 'multipart'

export type UploadStatus =
  | 'idle'
  | 'creating'
  | 'uploading'
  | 'paused'
  | 'completing'
  | 'done'
  | 'error'
  | 'aborted'

export interface PartProgress {
  partNumber: number
  loaded: number
  total: number
  status: 'pending' | 'uploading' | 'done' | 'error'
}

export interface UploadItem {
  id: string               // local ID (uuid)
  file: File
  mode: UploadMode
  status: UploadStatus
  // server IDs
  sessionId?: string       // multipart session ID or single-part upload ID
  fileId?: string          // ID in files table after completion
  // progress
  bytesUploaded: number
  totalBytes: number
  parts: PartProgress[]
  // throughput
  startedAt?: number
  throughputBps: number
  etaSeconds: number
  // error
  error?: string
}

export type UploadAction =
  | { type: 'ADD'; file: File }
  | { type: 'SET_STATUS'; id: string; status: UploadStatus }
  | { type: 'SET_SESSION'; id: string; sessionId: string; mode: UploadMode }
  | { type: 'SET_FILE_ID'; id: string; fileId: string }
  | { type: 'SET_PROGRESS'; id: string; bytesUploaded: number }
  | { type: 'SET_PART_STATUS'; id: string; partNumber: number; status: PartProgress['status']; loaded?: number }
  | { type: 'INIT_PARTS'; id: string; count: number; partSize: number; totalBytes: number }
  | { type: 'COMPLETE_PART'; id: string; partNumber: number }
  | { type: 'SET_THROUGHPUT'; id: string; throughputBps: number; etaSeconds: number }
  | { type: 'SET_ERROR'; id: string; error: string }
  | { type: 'REMOVE'; id: string }
