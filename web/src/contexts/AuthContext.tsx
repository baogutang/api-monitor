import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useState,
  type ReactNode,
} from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { authApi } from '@/api/services'
import { getToken, setToken, setUnauthorizedHandler } from '@/lib/api-client'
import type { User } from '@/lib/types'

type AuthContextValue = {
  user: User | null
  token: string | null
  isLoading: boolean
  needsSetup: boolean | null
  login: (email: string, password: string) => Promise<void>
  setup: (email: string, password: string, name?: string) => Promise<void>
  logout: () => void
  refetchSetup: () => void
}

const AuthContext = createContext<AuthContextValue | null>(null)

export function AuthProvider({ children }: { children: ReactNode }) {
  const qc = useQueryClient()
  const [token, setTokenState] = useState<string | null>(() => getToken())

  const setupQuery = useQuery({
    queryKey: ['setup-status'],
    queryFn: () => authApi.setupStatus(),
    retry: 1,
  })

  const meQuery = useQuery({
    queryKey: ['auth-me'],
    queryFn: () => authApi.me(),
    enabled: !!token,
    retry: false,
  })

  const logout = useCallback(() => {
    setToken(null)
    setTokenState(null)
    qc.clear()
  }, [qc])

  useEffect(() => {
    setUnauthorizedHandler(logout)
    return () => setUnauthorizedHandler(() => {})
  }, [logout])

  const login = useCallback(async (email: string, password: string) => {
    const res = await authApi.login({ email, password })
    setToken(res.token)
    setTokenState(res.token)
    await qc.invalidateQueries({ queryKey: ['auth-me'] })
  }, [qc])

  const setup = useCallback(async (email: string, password: string, name?: string) => {
    await authApi.setup({ email, password, name })
    setToken(null)
    setTokenState(null)
    qc.setQueryData(['setup-status'], { needsSetup: false })
    qc.removeQueries({ queryKey: ['auth-me'] })
  }, [qc])

  const value = useMemo<AuthContextValue>(
    () => ({
      user: meQuery.data ?? null,
      token,
      isLoading: setupQuery.isLoading || (!!token && meQuery.isLoading),
      needsSetup: setupQuery.data?.needsSetup ?? null,
      login,
      setup,
      logout,
      refetchSetup: () => void setupQuery.refetch(),
    }),
    [meQuery.data, meQuery.isLoading, token, setupQuery, login, setup, logout],
  )

  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>
}

export function useAuth() {
  const ctx = useContext(AuthContext)
  if (!ctx) throw new Error('useAuth must be used within AuthProvider')
  return ctx
}
