import { useReducer, useRef, useState, useEffect } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import { DropZone } from './components/DropZone'
import { UploadItem } from './components/UploadItem'
import { FileList } from './components/FileList'
import { uploadReducer } from './upload/reducer'
import { runSinglePartUpload, runMultipartUpload, shouldUseMultipart } from './upload/executor'
import { setApiKey } from './api/client'

export default function App() {
  const [apiKeyInput, setApiKeyInput] = useState(() => localStorage.getItem('api_key') ?? '')
  const [keySet, setKeySet] = useState(() => !!localStorage.getItem('api_key'))
  const [uploads, dispatch] = useReducer(uploadReducer, [])
  const controllers = useRef<Map<string, AbortController>>(new Map())
  const qc = useQueryClient()

  useEffect(() => {
    if (keySet) setApiKey(apiKeyInput)
  }, [keySet, apiKeyInput])

  // Start uploads for any idle items not yet running.
  useEffect(() => {
    for (const item of uploads) {
      if (item.status !== 'idle') continue
      if (controllers.current.has(item.id)) continue

      const controller = new AbortController()
      controllers.current.set(item.id, controller)

      const run = shouldUseMultipart(item.file) ? runMultipartUpload : runSinglePartUpload

      run(item.id, item.file, dispatch, controller.signal)
        .then(() => qc.invalidateQueries({ queryKey: ['files'] }))
        .catch((err: Error) => {
          if (err.name !== 'AbortError') {
            dispatch({ type: 'SET_ERROR', id: item.id, error: err.message })
          }
        })
        .finally(() => controllers.current.delete(item.id))
    }
  })

  function handleFiles(files: File[]) {
    for (const file of files) {
      dispatch({ type: 'ADD', file })
    }
  }

  function handleAbort(id: string) {
    controllers.current.get(id)?.abort()
    dispatch({ type: 'SET_STATUS', id, status: 'aborted' })
  }

  function handleRemove(id: string) {
    controllers.current.get(id)?.abort()
    dispatch({ type: 'REMOVE', id })
  }

  function handleSaveKey() {
    localStorage.setItem('api_key', apiKeyInput)
    setApiKey(apiKeyInput)
    setKeySet(true)
  }

  if (!keySet) {
    return (
      <div className="min-h-screen flex items-center justify-center p-6">
        <div className="w-full max-w-md rounded-2xl border border-slate-700 bg-slate-800/80 p-8 flex flex-col gap-4">
          <h1 className="text-slate-100 text-xl font-semibold">File Upload Service</h1>
          <p className="text-slate-400 text-sm">Enter your API key to continue.</p>
          <input
            type="password"
            value={apiKeyInput}
            onChange={e => setApiKeyInput(e.target.value)}
            onKeyDown={e => e.key === 'Enter' && apiKeyInput && handleSaveKey()}
            placeholder="Enter API key"
            className="w-full rounded-lg bg-slate-900 border border-slate-600 px-4 py-2.5 text-slate-100 placeholder:text-slate-600 outline-none focus:border-violet-500 transition-colors text-sm font-mono"
          />
          <button
            onClick={handleSaveKey}
            disabled={!apiKeyInput}
            className="rounded-lg bg-violet-600 hover:bg-violet-500 disabled:opacity-40 px-4 py-2.5 text-white font-medium text-sm transition-colors"
          >
            Continue
          </button>
        </div>
      </div>
    )
  }

  return (
    <div className="min-h-screen p-6 max-w-4xl mx-auto flex flex-col gap-6">
      <div className="flex items-center justify-between">
        <h1 className="text-slate-100 text-lg font-semibold">File Upload Service</h1>
        <button
          onClick={() => {
            localStorage.removeItem('api_key')
            setApiKey('')
            setKeySet(false)
            setApiKeyInput('')
          }}
          className="text-xs text-slate-500 hover:text-slate-300 transition-colors"
        >
          Change API key
        </button>
      </div>

      <DropZone onFiles={handleFiles} />

      {uploads.length > 0 && (
        <div className="flex flex-col gap-2">
          <h2 className="text-slate-400 text-xs uppercase tracking-wide font-medium">
            Uploads ({uploads.length})
          </h2>
          {uploads.map(item => (
            <UploadItem
              key={item.id}
              item={item}
              onAbort={() => handleAbort(item.id)}
              onRemove={() => handleRemove(item.id)}
            />
          ))}
        </div>
      )}

      <FileList />
    </div>
  )
}
