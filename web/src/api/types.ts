export interface ApiError {
  error: { code: string; message: string }
}

// Single-part upload
export interface UploadSession {
  id: string
  object_key: string
  upload_url: string
  upload_method: string
  upload_headers: Record<string, string>
  upload_expires_in_seconds: number
  original_name: string
  content_type: string
  expected_size: number
  status: string
  reused: boolean
}

export interface CompleteUploadResponse {
  id: string
  object_key: string
  original_name: string
  content_type: string
  expected_size: number
  status: string
}

// Multipart upload
export interface MultipartSession {
  id: string
  object_key: string
  original_name: string
  content_type: string
  expected_size: number
  part_size: number
  status: string
  reused: boolean
}

export interface PresignPartResponse {
  part_number: number
  upload_url: string
  upload_method: string
  upload_headers: Record<string, string>
  upload_expires_in_seconds: number
  size: number
}

export interface Part {
  part_number: number
  etag: string
  size: number
}

export interface ListPartsResponse {
  parts: Part[]
}

export interface CompleteMultipartResponse {
  file_id: string
  object_key: string
  original_name: string
  content_type: string
  expected_size: number
  status: string
}

// File listing
export interface FileRecord {
  id: string
  owner_principal_id: string
  object_key: string
  original_name: string
  content_type: string
  expected_size: number
  status: string
  created_at: string
  uploaded_at?: string
}

export interface ListFilesResponse {
  files: FileRecord[]
  next_cursor?: string
}

export interface DownloadResponse {
  id: string
  download_url: string
  download_method: string
  download_headers: Record<string, string>
  download_expires_in_seconds: number
  original_name: string
  content_type: string
  expected_size: number
  status: string
}
