import { render, screen, waitFor } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { AgentDashboardPage } from './AgentDashboardPage'

describe('AgentDashboardPage', () => {
  beforeEach(() => {
    localStorage.setItem('auth_token', 'test-token')
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue({
        ok: true,
        json: async () => ({
          agents: [
            {
              id: 'a1',
              name: 'Flash',
              mode: 'shadow',
              enabled: true,
              live_confirmed: false,
              exchange_id: 'bybit',
              ai_model_id: 'deepseek',
            },
          ],
        }),
      })
    )
  })

  it('shows agent mode and keeps live action explicit', async () => {
    render(<AgentDashboardPage />)
    expect(await screen.findByText('Flash')).toBeInTheDocument()
    expect(screen.getByText('shadow')).toBeInTheDocument()
    expect(
      screen.getByRole('button', { name: 'Enable Live' })
    ).toBeInTheDocument()
    await waitFor(() =>
      expect(fetch).toHaveBeenCalledWith('/api/agents', {
        headers: { Authorization: 'Bearer test-token' },
      })
    )
  })
})
