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
import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Dialog } from '@/components/dialog'
import { Button } from '@/components/ui/button'
import { Field, FieldError, FieldLabel } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { Spinner } from '@/components/ui/spinner'
import { MAX_PASSKEY_NAME_LENGTH } from '@/features/auth/passkey'

interface PasskeyNameDialogProps {
  open: boolean
  mode: 'create' | 'rename'
  initialName?: string
  loading?: boolean
  onOpenChange: (open: boolean) => void
  onSubmit: (name: string) => void
}

export function PasskeyNameDialog({
  open,
  mode,
  initialName = '',
  loading = false,
  onOpenChange,
  onSubmit,
}: PasskeyNameDialogProps) {
  const { t } = useTranslation()
  const [name, setName] = useState(initialName)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    if (open) {
      setName(initialName)
      setError(null)
    }
  }, [initialName, open])

  const handleSubmit = () => {
    const trimmed = name.trim()
    if (!trimmed) {
      setError(t('Passkey name is required'))
      return
    }
    if ([...trimmed].length > MAX_PASSKEY_NAME_LENGTH) {
      setError(
        t('Passkey name must be {{count}} characters or fewer', {
          count: MAX_PASSKEY_NAME_LENGTH,
        })
      )
      return
    }
    setError(null)
    onSubmit(trimmed)
  }

  return (
    <Dialog
      open={open}
      onOpenChange={(nextOpen) => {
        if (!loading) onOpenChange(nextOpen)
      }}
      title={mode === 'create' ? t('Name Passkey') : t('Rename Passkey')}
      description={
        mode === 'create'
          ? t('Give this Passkey a name so you can identify it later.')
          : t('Update the name used to identify this Passkey.')
      }
      footer={
        <>
          <Button
            type='button'
            variant='outline'
            disabled={loading}
            onClick={() => onOpenChange(false)}
          >
            {t('Cancel')}
          </Button>
          <Button type='button' disabled={loading} onClick={handleSubmit}>
            {loading ? <Spinner data-icon='inline-start' /> : null}
            {mode === 'create' ? t('Continue') : t('Save')}
          </Button>
        </>
      }
    >
      <form
        onSubmit={(event) => {
          event.preventDefault()
          handleSubmit()
        }}
      >
        <Field data-invalid={Boolean(error)}>
          <FieldLabel htmlFor='passkey-name'>{t('Passkey name')}</FieldLabel>
          <Input
            id='passkey-name'
            autoFocus
            value={name}
            aria-invalid={Boolean(error)}
            onChange={(event) => {
              setName(event.target.value)
              if (error) setError(null)
            }}
          />
          {error ? <FieldError>{error}</FieldError> : null}
        </Field>
      </form>
    </Dialog>
  )
}