import { useEffect, useState } from 'react'

type Agent = {
  id: string
  name: string
  mode: string
  enabled: boolean
  live_confirmed: boolean
  exchange_id: string
  ai_model_id: string
}

export function AgentDashboardPage() {
  const [agents, setAgents] = useState<Agent[]>([])
  const [error, setError] = useState('')
  const [busy, setBusy] = useState('')

  const authHeaders = (): Record<string, string> => {
    const token = localStorage.getItem('auth_token')
    return token ? { Authorization: `Bearer ${token}` } : {}
  }

  const load = async () => {
    const response = await fetch('/api/agents', { headers: authHeaders() })
    if (!response.ok) throw new Error('Не удалось загрузить агентов')
    const payload = (await response.json()) as { agents: Agent[] }
    setAgents(payload.agents ?? [])
  }

  useEffect(() => {
    load().catch((reason: unknown) =>
      setError(reason instanceof Error ? reason.message : 'Ошибка загрузки')
    )
  }, [])

  const action = async (id: string, path: string, body?: object) => {
    setBusy(id + path)
    setError('')
    try {
      const response = await fetch(`/api/agents/${id}/${path}`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json', ...authHeaders() },
        body: body ? JSON.stringify(body) : undefined,
      })
      if (!response.ok) {
        const payload = (await response.json().catch(() => ({}))) as {
          error?: string
        }
        throw new Error(payload.error ?? 'Операция не выполнена')
      }
      await load()
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : 'Ошибка операции')
    } finally {
      setBusy('')
    }
  }

  return (
    <main className="min-h-screen bg-[#11130f] px-6 py-10 text-[#f1ece2]">
      <div className="mx-auto max-w-6xl">
        <div className="mb-8 flex items-end justify-between gap-4">
          <div>
            <p className="mb-2 text-xs uppercase tracking-[0.25em] text-emerald-400">
              Private AI Control
            </p>
            <h1 className="text-4xl font-semibold">AI Agents</h1>
            <p className="mt-2 max-w-2xl text-sm text-[#a9afa5]">
              Shadow, Paper и Live с отдельным risk gate для каждого агента.
            </p>
          </div>
          <button
            className="rounded-lg border border-emerald-500/40 px-4 py-2 text-sm text-emerald-300"
            onClick={() => load().catch(() => setError('Ошибка обновления'))}
          >
            Обновить
          </button>
        </div>
        {error && (
          <div
            role="alert"
            className="mb-5 rounded-lg border border-red-500/40 bg-red-950/40 p-3 text-sm text-red-200"
          >
            {error}
          </div>
        )}
        <div className="grid gap-4 md:grid-cols-2">
          {agents.map((item) => (
            <section
              key={item.id}
              className="rounded-2xl border border-white/10 bg-white/[0.04] p-5 shadow-xl"
            >
              <div className="flex items-start justify-between gap-4">
                <div>
                  <h2 className="text-xl font-medium">{item.name}</h2>
                  <p className="mt-1 text-xs text-[#899187]">{item.id}</p>
                </div>
                <span className="rounded-full bg-emerald-400/10 px-3 py-1 text-xs uppercase text-emerald-300">
                  {item.mode}
                </span>
              </div>
              <dl className="mt-5 grid grid-cols-2 gap-3 text-sm">
                <div>
                  <dt className="text-[#899187]">Exchange</dt>
                  <dd>{item.exchange_id}</dd>
                </div>
                <div>
                  <dt className="text-[#899187]">Model</dt>
                  <dd>{item.ai_model_id}</dd>
                </div>
                <div>
                  <dt className="text-[#899187]">Live gate</dt>
                  <dd>{item.live_confirmed ? 'Confirmed' : 'Locked'}</dd>
                </div>
                <div>
                  <dt className="text-[#899187]">Enabled</dt>
                  <dd>{item.enabled ? 'Yes' : 'No'}</dd>
                </div>
              </dl>
              <div className="mt-6 flex flex-wrap gap-2">
                <button
                  disabled={!!busy}
                  className="rounded-md bg-emerald-400 px-3 py-2 text-sm font-medium text-black disabled:opacity-50"
                  onClick={() => action(item.id, 'start')}
                >
                  Новый цикл
                </button>
                <button
                  disabled={!!busy}
                  className="rounded-md border border-amber-400/50 px-3 py-2 text-sm text-amber-200 disabled:opacity-50"
                  onClick={() => action(item.id, 'stop')}
                >
                  Kill switch
                </button>
                {!item.live_confirmed && (
                  <button
                    disabled={!!busy}
                    className="rounded-md border border-red-400/50 px-3 py-2 text-sm text-red-200 disabled:opacity-50"
                    onClick={() =>
                      action(item.id, 'live-confirmation', {
                        confirmation: `ENABLE LIVE ${item.id}`,
                      })
                    }
                  >
                    Enable Live
                  </button>
                )}
              </div>
            </section>
          ))}
          {agents.length === 0 && !error && (
            <div className="rounded-2xl border border-dashed border-white/15 p-10 text-center text-[#a9afa5]">
              Агентов пока нет. Создайте первого через API или настройки.
            </div>
          )}
        </div>
      </div>
    </main>
  )
}
