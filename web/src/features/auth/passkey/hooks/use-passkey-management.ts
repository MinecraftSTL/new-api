/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import { useCallback, useEffect, useRef, useState } from 'react'

import {
  buildRegistrationResult,
  createCredential,
  isPasskeySupported,
  prepareCredentialCreationOptions,
} from '@/lib/passkey'
import { AuthOperationError } from '@/lib/secure-verification'

import {
  beginPasskeyRegistration,
  deletePasskey,
  finishPasskeyRegistration,
  getPasskeyStatus,
  renamePasskey,
} from '../api'
import type { PasskeyStatus } from '../types'

export const MAX_PASSKEYS_PER_USER = 16
export const MAX_PASSKEY_NAME_LENGTH = 64

export function usePasskeyManagement() {
  const [status, setStatus] = useState<PasskeyStatus | null>(null)
  const [statusError, setStatusError] = useState<string | null>(null)
  const [loading, setLoading] = useState(true)
  const [registering, setRegistering] = useState(false)
  const [renamingId, setRenamingId] = useState<number | null>(null)
  const [removingId, setRemovingId] = useState<number | null>(null)
  const [supported, setSupported] = useState(false)
  const operation = useRef<AbortController | null>(null)
  const mounted = useRef(true)

  const fetchStatus = useCallback(async () => {
    setLoading(true)
    try {
      const response = await getPasskeyStatus()
      if (!response.success || !response.data) {
        throw new AuthOperationError(
          response.message || 'Failed to load Passkey status'
        )
      }
      if (!mounted.current) return
      setStatus(response.data)
      setStatusError(null)
    } catch (error) {
      if (mounted.current) {
        setStatusError(AuthOperationError.from(error).message)
      }
    } finally {
      if (mounted.current) setLoading(false)
    }
  }, [])

  useEffect(() => {
    mounted.current = true
    void fetchStatus()
    void isPasskeySupported().then((value) => {
      if (mounted.current) setSupported(value)
    })
    return () => {
      mounted.current = false
      operation.current?.abort()
    }
  }, [fetchStatus])

  const register = useCallback(
    async (name: string, proofToken: string) => {
      if (!supported || !navigator.credentials) {
        throw new AuthOperationError('This device does not support Passkey')
      }
      if (operation.current) {
        throw new AuthOperationError(
          'A security operation is already in progress.'
        )
      }
      const controller = new AbortController()
      operation.current = controller
      setRegistering(true)
      try {
        const begin = await beginPasskeyRegistration(
          name,
          proofToken,
          controller.signal
        )
        if (!begin.flow_token) {
          throw new AuthOperationError(
            'Registration flow expired. Please try again.'
          )
        }
        const credential = (await createCredential(
          prepareCredentialCreationOptions(begin.options ?? begin),
          controller.signal
        )) as PublicKeyCredential | null
        controller.signal.throwIfAborted()
        if (!credential) {
          throw new AuthOperationError(
            'Passkey registration was cancelled',
            'AUTH_CANCELLED'
          )
        }
        const attestation = buildRegistrationResult(credential)
        if (!attestation) {
          throw new AuthOperationError('Invalid Passkey registration response')
        }
        await finishPasskeyRegistration(
          begin.flow_token,
          attestation,
          controller.signal
        )
        controller.signal.throwIfAborted()
        await fetchStatus()
      } catch (error) {
        if (mounted.current && !controller.signal.aborted) await fetchStatus()
        if (
          controller.signal.aborted ||
          (error instanceof DOMException && error.name === 'NotAllowedError')
        ) {
          throw new AuthOperationError(
            'Passkey registration was cancelled',
            'AUTH_CANCELLED',
            { cause: error }
          )
        }
        throw AuthOperationError.from(error, 'Failed to register Passkey')
      } finally {
        if (operation.current === controller) operation.current = null
        if (mounted.current) setRegistering(false)
      }
    },
    [fetchStatus, supported]
  )

  const rename = useCallback(
    async (passkeyId: number, name: string) => {
      if (operation.current) {
        throw new AuthOperationError(
          'A security operation is already in progress.'
        )
      }
      const controller = new AbortController()
      operation.current = controller
      setRenamingId(passkeyId)
      try {
        await renamePasskey(passkeyId, name, controller.signal)
        controller.signal.throwIfAborted()
        await fetchStatus()
      } catch (error) {
        if (mounted.current && !controller.signal.aborted) await fetchStatus()
        throw AuthOperationError.from(error, 'Failed to rename Passkey')
      } finally {
        if (operation.current === controller) operation.current = null
        if (mounted.current) setRenamingId(null)
      }
    },
    [fetchStatus]
  )

  const remove = useCallback(
    async (passkeyId: number, proofToken: string) => {
      if (operation.current) {
        throw new AuthOperationError(
          'A security operation is already in progress.'
        )
      }
      const controller = new AbortController()
      operation.current = controller
      setRemovingId(passkeyId)
      try {
        await deletePasskey(passkeyId, proofToken, controller.signal)
        controller.signal.throwIfAborted()
        await fetchStatus()
      } catch (error) {
        if (mounted.current && !controller.signal.aborted) await fetchStatus()
        if (controller.signal.aborted) {
          throw new AuthOperationError(
            'Operation cancelled',
            'AUTH_CANCELLED',
            { cause: error }
          )
        }
        throw AuthOperationError.from(error, 'Failed to remove Passkey')
      } finally {
        if (operation.current === controller) operation.current = null
        if (mounted.current) setRemovingId(null)
      }
    },
    [fetchStatus]
  )

  return {
    status,
    statusError,
    loading,
    registering,
    renamingId,
    removingId,
    renaming: renamingId !== null,
    removing: removingId !== null,
    supported,
    enabled: Boolean(status?.enabled),
    passkeys: status?.passkeys ?? [],
    lastUsed: status?.last_used_at ?? null,
    fetchStatus,
    register,
    rename,
    remove,
  }
}
