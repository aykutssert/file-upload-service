import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { Download, Trash2, RefreshCw, FileIcon } from 'lucide-react'
import { listFiles, getDownloadUrl, deleteFile } from '../api/client'
import type { FileRecord } from '../api/types'

function formatBytes(n: number): string {
  if (n < 1024 * 1024) return `${(n / 1024).toFixed(1)} KB`
  if (n < 1024 * 1024 * 1024) return `${(n / (1024 * 1024)).toFixed(1)} MB`
  return `${(n / (1024 * 1024 * 1024)).toFixed(2)} GB`
}

const STATUS_COLOR: Record<string, string> = {
  ready: 'text-emerald-400 bg-emerald-500/10',
  pending: 'text-yellow-400 bg-yellow-500/10',
  processing: 'text-violet-400 bg-violet-500/10',
  deleted: 'text-slate-500 bg-slate-700',
}

function StatusBadge({ status }: { status: string }) {
  const cls = STATUS_COLOR[status] ?? 'text-slate-400 bg-slate-700'
  return (
    <span className={`inline-flex items-center px-2 py-0.5 rounded text-xs font-medium ${cls}`}>
      {status}
    </span>
  )
}

function FileRow({ file }: { file: FileRecord }) {
  const qc = useQueryClient()

  const download = useMutation({
    mutationFn: () => getDownloadUrl(file.id),
    onSuccess: (res) => window.open(res.download_url, '_blank'),
  })

  const remove = useMutation({
    mutationFn: () => deleteFile(file.id),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['files'] }),
  })

  return (
    <tr className="border-t border-slate-700/50 hover:bg-slate-800/30 transition-colors">
      <td className="px-4 py-3">
        <div className="flex items-center gap-2 min-w-0">
          <FileIcon size={14} className="text-slate-500 shrink-0" />
          <span className="text-slate-200 text-sm truncate max-w-[240px]">{file.original_name}</span>
        </div>
      </td>
      <td className="px-4 py-3 text-slate-400 text-sm">{formatBytes(file.expected_size)}</td>
      <td className="px-4 py-3">
        <StatusBadge status={file.status} />
      </td>
      <td className="px-4 py-3 text-slate-500 text-xs">{file.content_type}</td>
      <td className="px-4 py-3">
        <div className="flex items-center gap-2">
          <button
            onClick={() => download.mutate()}
            disabled={file.status !== 'ready' || download.isPending}
            className="text-slate-500 hover:text-violet-400 disabled:opacity-30 transition-colors"
            title="Download"
          >
            <Download size={14} />
          </button>
          <button
            onClick={() => remove.mutate()}
            disabled={file.status !== 'ready' || remove.isPending}
            className="text-slate-500 hover:text-red-400 disabled:opacity-30 transition-colors"
            title="Delete"
          >
            <Trash2 size={14} />
          </button>
        </div>
      </td>
    </tr>
  )
}

export function FileList() {
  const qc = useQueryClient()
  const { data, isLoading, isError, isFetching } = useQuery({
    queryKey: ['files'],
    queryFn: () => listFiles(),
    refetchInterval: 5_000,
  })

  return (
    <div className="rounded-xl border border-slate-700 overflow-hidden">
      <div className="flex items-center justify-between px-4 py-3 border-b border-slate-700 bg-slate-800/50">
        <h2 className="text-slate-200 font-medium text-sm">Uploaded files</h2>
        <button
          onClick={() => qc.invalidateQueries({ queryKey: ['files'] })}
          className="text-slate-500 hover:text-slate-300 transition-colors"
          title="Refresh"
        >
          <RefreshCw size={14} className={isFetching ? 'animate-spin' : ''} />
        </button>
      </div>

      {isLoading && (
        <div className="px-4 py-8 text-center text-slate-500 text-sm">Loading…</div>
      )}
      {isError && (
        <div className="px-4 py-8 text-center text-red-400 text-sm">Failed to load files</div>
      )}
      {data && data.files.length === 0 && (
        <div className="px-4 py-8 text-center text-slate-500 text-sm">No files yet</div>
      )}
      {data && data.files.length > 0 && (
        <table className="w-full">
          <thead>
            <tr className="text-left text-xs text-slate-500 uppercase tracking-wide">
              <th className="px-4 py-2">Name</th>
              <th className="px-4 py-2">Size</th>
              <th className="px-4 py-2">Status</th>
              <th className="px-4 py-2">Type</th>
              <th className="px-4 py-2">Actions</th>
            </tr>
          </thead>
          <tbody>
            {data.files.map(f => <FileRow key={f.id} file={f} />)}
          </tbody>
        </table>
      )}
    </div>
  )
}
