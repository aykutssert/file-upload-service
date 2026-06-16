import { useRef, useState } from 'react'
import { Upload } from 'lucide-react'

interface Props {
  onFiles: (files: File[]) => void
  disabled?: boolean
}

export function DropZone({ onFiles, disabled }: Props) {
  const [dragging, setDragging] = useState(false)
  const inputRef = useRef<HTMLInputElement>(null)

  function handleDrop(e: React.DragEvent) {
    e.preventDefault()
    setDragging(false)
    if (disabled) return
    const files = Array.from(e.dataTransfer.files)
    if (files.length > 0) onFiles(files)
  }

  function handleChange(e: React.ChangeEvent<HTMLInputElement>) {
    const files = Array.from(e.target.files ?? [])
    if (files.length > 0) onFiles(files)
    e.target.value = ''
  }

  return (
    <div
      onClick={() => !disabled && inputRef.current?.click()}
      onDragOver={e => { e.preventDefault(); if (!disabled) setDragging(true) }}
      onDragLeave={() => setDragging(false)}
      onDrop={handleDrop}
      className={[
        'flex flex-col items-center justify-center gap-3 rounded-xl border-2 border-dashed p-12 cursor-pointer transition-colors',
        dragging
          ? 'border-violet-500 bg-violet-500/10'
          : 'border-slate-600 hover:border-slate-400 bg-slate-800/50',
        disabled ? 'opacity-50 cursor-not-allowed' : '',
      ].join(' ')}
    >
      <Upload size={32} className="text-slate-400" />
      <div className="text-center">
        <p className="text-slate-200 font-medium">Drop files here or click to browse</p>
        <p className="text-slate-500 text-sm mt-1">Files &lt; 20 MB use single-part · Larger files use multipart</p>
      </div>
      <input ref={inputRef} type="file" multiple className="hidden" onChange={handleChange} />
    </div>
  )
}
