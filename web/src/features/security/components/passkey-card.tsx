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
import {
  AlertTriangle,
  KeyRound,
  Loader2,
  Pencil,
  Plus,
  ShieldAlert,
  Trash2,
} from 'lucide-react'
import { useCallback, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { StatusBadge } from '@/components/status-badge'
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from '@/components/ui/alert-dialog'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { IconBadge } from '@/components/ui/icon-badge'
import {
  Item,
  ItemActions,
  ItemContent,
  ItemDescription,
  ItemGroup,
  ItemMedia,
  ItemTitle,
} from '@/components/ui/item'
import { Skeleton } from '@/components/ui/skeleton'
import {
  MAX_PASSKEYS_PER_USER,
  type Passkey,
  usePasskeyManagement,
} from '@/features/auth/passkey'
import {
  SecureVerificationDialog,
  useSecureVerification,
} from '@/features/auth/secure-verification'
import dayjs from '@/lib/dayjs'
import { handleServerError } from '@/lib/handle-server-error'
import { AuthOperationError } from '@/lib/secure-verification'

import { PasskeyNameDialog } from './passkey-name-dialog'

interface PasskeyCardProps {
  loading: boolean
}

type NameDialogState =
  | { mode: 'create' }
  | { mode: 'rename'; passkey: Passkey }
  | null

function formatPasskeyDate(value?: string | null) {
  return value && !Number.isNaN(Date.parse(value))
    ? dayjs(value).format('LLL')
    : null
}

export function PasskeyCard({ loading: pageLoading }: PasskeyCardProps) {
  const { t } = useTranslation()
  const [nameDialog, setNameDialog] = useState<NameDialogState>(null)
  const [deleteTarget, setDeleteTarget] = useState<Passkey | null>(null)
  const {
    statusError,
    fetchStatus,
    loading,
    registering,
    renamingId,
    removingId,
    supported,
    enabled,
    passkeys,
    register,
    rename,
    remove,
  } = usePasskeyManagement()
  const verification = useSecureVerification()

  const handleRegister = useCallback(
    async (name: string) => {
      if (passkeys.length >= MAX_PASSKEYS_PER_USER) {
        toast.error(
          t('You can add up to {{count}} Passkeys.', {
            count: MAX_PASSKEYS_PER_USER,
          })
        )
        return
      }
      const proof = await verification.requestVerification({
        scope: 'passkey.register',
      })
      if (!proof) return
      try {
        await register(name, proof.proof_token)
        setNameDialog(null)
        toast.success(t('Passkey registered successfully'))
      } catch (error) {
        const failure = AuthOperationError.from(error)
        if (failure.code !== 'AUTH_CANCELLED') handleServerError(failure)
      }
    },
    [passkeys.length, register, t, verification]
  )

  const handleRename = useCallback(
    async (passkey: Passkey, name: string) => {
      try {
        await rename(passkey.id, name)
        setNameDialog(null)
        toast.success(t('Passkey renamed successfully'))
      } catch (error) {
        handleServerError(AuthOperationError.from(error))
      }
    },
    [rename, t]
  )

  const handleRemove = useCallback(async () => {
    if (!deleteTarget) return
    const target = deleteTarget
    const proof = await verification.requestVerification({
      scope: 'passkey.delete',
      context: { passkey_id: target.id },
    })
    if (!proof) return
    try {
      await remove(target.id, proof.proof_token)
      setDeleteTarget(null)
      toast.success(t('Passkey removed successfully'))
    } catch (error) {
      const failure = AuthOperationError.from(error)
      if (failure.code !== 'AUTH_CANCELLED') handleServerError(failure)
    }
  }, [deleteTarget, remove, t, verification])

  if (pageLoading || loading) {
    return (
      <Card data-card-hover='false' className='gap-0 overflow-hidden py-0'>
        <CardHeader className='p-3 sm:p-5'>
          <Skeleton className='h-6 w-48' />
          <Skeleton className='mt-2 h-4 w-64' />
        </CardHeader>
        <CardContent className='p-3 sm:p-5'>
          <Skeleton className='h-20 w-full' />
        </CardContent>
      </Card>
    )
  }

  if (statusError) {
    return (
      <Card>
        <CardHeader>
          <CardTitle>{t('Passkey Login')}</CardTitle>
        </CardHeader>
        <CardContent className='space-y-3'>
          <p role='alert'>{t(statusError)}</p>
          <Button onClick={() => void fetchStatus()}>{t('Retry')}</Button>
        </CardContent>
      </Card>
    )
  }

  const showUnsupportedNotice = !supported && !enabled
  const limitReached = passkeys.length >= MAX_PASSKEYS_PER_USER

  return (
    <>
      <Card data-card-hover='false' className='gap-0 overflow-hidden py-0'>
        <CardHeader className='p-3 sm:p-5'>
          <CardTitle className='text-lg tracking-tight sm:text-xl'>
            {t('Passkey Login')}
          </CardTitle>
          <CardDescription className='text-xs sm:text-sm'>
            {t('Use Passkey to sign in without entering your password.')}
          </CardDescription>
        </CardHeader>

        <CardContent className='p-3 sm:p-5'>
          <div className='space-y-5'>
            <div className='flex flex-col gap-4 sm:flex-row sm:items-start sm:justify-between'>
              <div className='flex items-start gap-4'>
                <IconBadge tone='info' size='sm'>
                  <KeyRound />
                </IconBadge>
                <div className='space-y-1'>
                  <div className='flex flex-wrap items-center gap-2'>
                    <p className='font-medium'>{t('Passkey Authentication')}</p>
                    <StatusBadge
                      label={enabled ? t('Enabled') : t('Disabled')}
                      variant={enabled ? 'success' : 'neutral'}
                      showDot
                      copyable={false}
                    />
                  </div>
                  <p className='text-muted-foreground text-sm'>
                    {t('{{count}} Passkeys', { count: passkeys.length })}
                  </p>
                </div>
              </div>

              <Button
                className='w-full sm:w-auto'
                onClick={() => setNameDialog({ mode: 'create' })}
                disabled={!supported || limitReached || registering}
              >
                <Plus className='mr-2 h-4 w-4' />
                {enabled ? t('Add Passkey') : t('Enable Passkey')}
              </Button>
            </div>

            {limitReached && (
              <p className='text-muted-foreground text-sm'>
                {t('You can add up to {{count}} Passkeys.', {
                  count: MAX_PASSKEYS_PER_USER,
                })}
              </p>
            )}

            {passkeys.length > 0 && (
              <ItemGroup>
                {passkeys.map((passkey) => {
                  const createdAt = formatPasskeyDate(passkey.created_at)
                  const lastUsedAt = formatPasskeyDate(passkey.last_used_at)
                  return (
                    <Item key={passkey.id} variant='outline'>
                      <ItemMedia variant='icon'>
                        <KeyRound />
                      </ItemMedia>
                      <ItemContent>
                        <ItemTitle>{passkey.name}</ItemTitle>
                        <ItemDescription>
                          {passkey.rp_id || t('Unknown domain')}
                          {createdAt ? `   ${t('Created:')} ${createdAt}` : ''}
                          {`   ${t('Last used:')} ${lastUsedAt ?? t('Not used yet')}`}
                        </ItemDescription>
                      </ItemContent>
                      <ItemActions>
                        <Button
                          type='button'
                          variant='ghost'
                          size='icon'
                          aria-label={t('Rename Passkey')}
                          disabled={renamingId === passkey.id}
                          onClick={() =>
                            setNameDialog({ mode: 'rename', passkey })
                          }
                        >
                          {renamingId === passkey.id ? (
                            <Loader2 className='h-4 w-4 animate-spin' />
                          ) : (
                            <Pencil className='h-4 w-4' />
                          )}
                        </Button>
                        <Button
                          type='button'
                          variant='ghost'
                          size='icon'
                          aria-label={t('Remove Passkey')}
                          disabled={removingId === passkey.id}
                          onClick={() => setDeleteTarget(passkey)}
                        >
                          {removingId === passkey.id ? (
                            <Loader2 className='h-4 w-4 animate-spin' />
                          ) : (
                            <Trash2 className='h-4 w-4' />
                          )}
                        </Button>
                      </ItemActions>
                    </Item>
                  )
                })}
              </ItemGroup>
            )}

            {showUnsupportedNotice && (
              <div className='bg-muted/60 text-muted-foreground flex items-start gap-3 rounded-md p-4 text-sm'>
                <ShieldAlert className='mt-0.5 h-4 w-4 flex-shrink-0 text-amber-500' />
                <div>
                  <p className='text-foreground font-medium'>
                    {t('Passkey not supported on this device')}
                  </p>
                  <p>
                    {t(
                      'Use a compatible browser or device with biometric authentication or a security key to register a Passkey.'
                    )}
                  </p>
                </div>
              </div>
            )}
          </div>
        </CardContent>
      </Card>

      <PasskeyNameDialog
        open={nameDialog !== null}
        mode={nameDialog?.mode ?? 'create'}
        initialName={
          nameDialog?.mode === 'rename' ? nameDialog.passkey.name : ''
        }
        loading={registering || renamingId !== null}
        onOpenChange={(open) => {
          if (!open) setNameDialog(null)
        }}
        onSubmit={(name) => {
          if (nameDialog?.mode === 'rename') {
            void handleRename(nameDialog.passkey, name)
            return
          }
          void handleRegister(name)
        }}
      />

      <AlertDialog
        open={deleteTarget !== null}
        onOpenChange={(open) => {
          if (!open && removingId === null) setDeleteTarget(null)
        }}
      >
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>
              {t('Remove {{name}}?', {
                name: deleteTarget?.name ?? '',
              })}
            </AlertDialogTitle>
            <AlertDialogDescription>
              {t(
                'Confirm your identity before removing this Passkey from your account.'
              )}
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel disabled={removingId !== null}>
              {t('Cancel')}
            </AlertDialogCancel>
            <AlertDialogAction
              variant='destructive'
              disabled={removingId !== null}
              onClick={(event) => {
                event.preventDefault()
                void handleRemove()
              }}
            >
              {removingId !== null ? (
                <Loader2 className='mr-2 h-4 w-4 animate-spin' />
              ) : (
                <AlertTriangle className='mr-2 h-4 w-4' />
              )}
              {t('Remove')}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>

      <SecureVerificationDialog {...verification.dialogProps} />
    </>
  )
}
