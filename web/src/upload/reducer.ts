import type { UploadAction, UploadItem, PartProgress } from './types'

let counter = 0

function makeId(): string {
  return `upload-${++counter}-${Date.now()}`
}

export function uploadReducer(state: UploadItem[], action: UploadAction): UploadItem[] {
  switch (action.type) {
    case 'ADD':
      return [
        ...state,
        {
          id: makeId(),
          file: action.file,
          mode: 'single',
          status: 'idle',
          bytesUploaded: 0,
          totalBytes: action.file.size,
          parts: [],
          throughputBps: 0,
          etaSeconds: 0,
        },
      ]

    case 'SET_STATUS':
      return state.map(u =>
        u.id === action.id ? { ...u, status: action.status } : u,
      )

    case 'SET_SESSION':
      return state.map(u =>
        u.id === action.id
          ? { ...u, sessionId: action.sessionId, mode: action.mode, startedAt: Date.now() }
          : u,
      )

    case 'SET_FILE_ID':
      return state.map(u =>
        u.id === action.id ? { ...u, fileId: action.fileId } : u,
      )

    case 'SET_PROGRESS':
      return state.map(u =>
        u.id === action.id ? { ...u, bytesUploaded: action.bytesUploaded } : u,
      )

    case 'INIT_PARTS': {
      const parts: PartProgress[] = []
      for (let i = 1; i <= action.count; i++) {
        const isLast = i === action.count
        const size = isLast
          ? action.totalBytes - (action.count - 1) * action.partSize
          : action.partSize
        parts.push({ partNumber: i, loaded: 0, total: size, status: 'pending' })
      }
      return state.map(u =>
        u.id === action.id ? { ...u, parts, totalBytes: action.totalBytes } : u,
      )
    }

    case 'SET_PART_STATUS':
      return state.map(u => {
        if (u.id !== action.id) return u
        return {
          ...u,
          parts: u.parts.map(p =>
            p.partNumber === action.partNumber
              ? { ...p, status: action.status, loaded: action.loaded ?? p.loaded }
              : p,
          ),
        }
      })

    case 'COMPLETE_PART':
      return state.map(u => {
        if (u.id !== action.id) return u
        const parts = u.parts.map(p =>
          p.partNumber === action.partNumber
            ? { ...p, status: 'done' as const, loaded: p.total }
            : p,
        )
        const bytesUploaded = parts.reduce((s, p) => s + (p.status === 'done' ? p.total : 0), 0)
        return { ...u, parts, bytesUploaded }
      })

    case 'SET_THROUGHPUT':
      return state.map(u =>
        u.id === action.id
          ? { ...u, throughputBps: action.throughputBps, etaSeconds: action.etaSeconds }
          : u,
      )

    case 'SET_ERROR':
      return state.map(u =>
        u.id === action.id ? { ...u, status: 'error', error: action.error } : u,
      )

    case 'REMOVE':
      return state.filter(u => u.id !== action.id)

    default:
      return state
  }
}
