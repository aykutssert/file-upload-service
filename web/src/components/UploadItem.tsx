import { X, CheckCircle, AlertCircle, Loader, FileIcon } from 'lucide-react'
import type { UploadItem as TUploadItem } from '../upload/types'

interface Props {
  item: TUploadItem
  onAbort: () => void
  onRemove: () => void
}

function formatBytes(n: number): string {
  if (n < 1024) return `${n} B`
  if (n < 1024 * 1024) return `${(n / 1024).toFixed(1)} KB`
  if (n < 1024 * 1024 * 1024) return `${(n / (1024 * 1024)).toFixed(1)} MB`
  return `${(n / (1024 * 1024 * 1024)).toFixed(2)} GB`
}

function formatEta(s: number): string {
  if (s < 60) return `${Math.round(s)}s`
  if (s < 3600) return `${Math.floor(s / 60)}m ${Math.round(s % 60)}s`
  return `${Math.floor(s / 3600)}h ${Math.floor((s % 3600) / 60)}m`
}

const STATUS_LABEL: Record<string, string> = {
  idle: 'Queued',
  creating: 'Creating session…',
  uploading: 'Uploading',
  paused: 'Paused',
  completing: 'Finalizing…',
  done: 'Complete',
  error: 'Failed',
  aborted: 'Aborted',
}

export function UploadItem({ item, onAbort, onRemove }: Props) {
  const isActive = item.status === 'uploading' || item.status === 'creating' || item.status === 'completing'
  const isDone = item.status === 'done'
  const isFailed = item.status === 'error' || item.status === 'aborted'
  const progress = item.totalBytes > 0 ? (item.bytesUploaded / item.totalBytes) * 100 : 0

  return (
    <div className="rounded-lg bg-slate-800 border border-slate-700 p-4 flex flex-col gap-3">
      {/* Header */}
      <div className="flex items-start gap-3">
        <FileIcon size={20} className="text-slate-400 mt-0.5 shrink-0" />
        <div className="flex-1 min-w-0">
          <p className="text-slate-100 font-medium truncate">{item.file.name}</p>
          <p className="text-slate-500 text-xs mt-0.5">
            {formatBytes(item.file.size)} · {item.mode === 'multipart' ? 'multipart' : 'single-part'}
          </p>
        </div>
        <div className="flex items-center gap-2 shrink-0">
          {isDone && <CheckCircle size={16} className="text-emerald-400" />}
          {isFailed && <AlertCircle size={16} className="text-red-400" />}
          {isActive && <Loader size={16} className="text-violet-400 animate-spin" />}
          {isActive ? (
            <button
              onClick={onAbort}
              className="text-slate-500 hover:text-slate-200 transition-colors"
              title="Cancel"
            >
              <X size={16} />
            </button>
          ) : (
            <button
              onClick={onRemove}
              className="text-slate-500 hover:text-slate-200 transition-colors"
              title="Remove"
            >
              <X size={16} />
            </button>
          )}
        </div>
      </div>

      {/* Progress bar */}
      {!isDone && !isFailed && (
        <div className="h-1.5 w-full rounded-full bg-slate-700 overflow-hidden">
          <div
            className="h-full rounded-full bg-violet-500 transition-all duration-300"
            style={{ width: `${Math.min(progress, 100)}%` }}
          />
        </div>
      )}

      {/* Part bars for multipart */}
      {item.mode === 'multipart' && item.parts.length > 0 && !isDone && !isFailed && (
        <div className="flex gap-0.5 flex-wrap">
          {item.parts.map(p => (
            <div
              key={p.partNumber}
              title={`Part ${p.partNumber}: ${p.status}`}
              className={[
                'h-1 rounded-sm flex-1 min-w-[6px] max-w-[20px]',
                p.status === 'done' ? 'bg-emerald-500' :
                p.status === 'uploading' ? 'bg-violet-400 animate-pulse' :
                p.status === 'error' ? 'bg-red-500' :
                'bg-slate-600',
              ].join(' ')}
            />
          ))}
        </div>
      )}

      {/* Status line */}
      <div className="flex items-center justify-between text-xs">
        <span className={[
          isFailed ? 'text-red-400' : isDone ? 'text-emerald-400' : 'text-slate-400',
        ].join('')}>
          {STATUS_LABEL[item.status] ?? item.status}
          {isDone && item.mode === 'multipart' ? ` — multipart (${item.parts.length} parts)` : ''}
          {isDone && item.mode === 'single' ? ' — single-part' : ''}
          {item.error ? ` — ${item.error}` : ''}
        </span>
        {isActive && item.throughputBps > 0 && (
          <span className="text-slate-500">
            {formatBytes(item.throughputBps)}/s · {formatEta(item.etaSeconds)} left
          </span>
        )}
        {isActive && (
          <span className="text-slate-500">
            {formatBytes(item.bytesUploaded)} / {formatBytes(item.totalBytes)}
          </span>
        )}
      </div>
    </div>
  )
}
