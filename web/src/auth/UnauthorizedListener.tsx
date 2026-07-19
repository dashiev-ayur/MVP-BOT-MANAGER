import { useEffect } from 'react'
import { useNavigate } from 'react-router'
import { setUnauthorizedHandler } from '../api/client'

/**
 * Подписывает api client на 401: очистка сессии уже в client,
 * здесь — переход на /login с понятным сообщением.
 */
export function UnauthorizedListener() {
  const navigate = useNavigate()

  useEffect(() => {
    setUnauthorizedHandler(() => {
      navigate('/login', {
        replace: true,
        state: { reason: 'unauthorized' },
      })
    })
    return () => setUnauthorizedHandler(null)
  }, [navigate])

  return null
}
