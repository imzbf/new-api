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
import { zodResolver } from '@hookform/resolvers/zod'
import { useQuery } from '@tanstack/react-query'
import { ChevronLeft, ChevronRight, RefreshCw, Trash2 } from 'lucide-react'
import { useEffect, useMemo, useState } from 'react'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import * as z from 'zod'

import { DateTimePicker } from '@/components/datetime-picker'
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
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Form,
  FormControl,
  FormDescription,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Progress } from '@/components/ui/progress'
import { Skeleton } from '@/components/ui/skeleton'
import { Switch } from '@/components/ui/switch'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { Textarea } from '@/components/ui/textarea'
import { formatTimestampToDate } from '@/lib/format'

import {
  getCurrentSensitiveReplacementLogCleanupTask,
  getSensitiveReplacementLogs,
  getSystemTask,
  startSensitiveReplacementLogCleanupTask,
} from '../api'
import {
  SettingsControlGroup,
  SettingsForm,
  SettingsSwitchContent,
  SettingsSwitchItem,
} from '../components/settings-form-layout'
import { SettingsPageFormActions } from '../components/settings-page-context'
import { SettingsSection } from '../components/settings-section'
import { useUpdateOption } from '../hooks/use-update-option'
import type {
  SensitiveReplacementLog,
  SensitiveReplacementLogCleanupTask,
} from '../types'

const REPLACEMENT_LOG_PAGE_SIZE = 10
const CLEANUP_TASK_POLL_INTERVAL_MS = 1000

const sensitiveReplacementSchema = z.object({
  SensitiveReplacementEnabled: z.boolean(),
  SensitiveReplacementRules: z.string().optional(),
  SensitiveReplacementLogRetentionDays: z.number().min(0),
})

type SensitiveReplacementFormValues = z.infer<
  typeof sensitiveReplacementSchema
>

type SensitiveReplacementSectionProps = {
  defaultValues: SensitiveReplacementFormValues
}

const getDateDaysAgo = (days: number) => {
  const date = new Date()
  date.setDate(date.getDate() - days)
  return date
}

const cleanupQuickSelectOptions = [
  { label: '30 days ago', getValue: () => getDateDaysAgo(30) },
  { label: '90 days ago', getValue: () => getDateDaysAgo(90) },
  { label: '180 days ago', getValue: () => getDateDaysAgo(180) },
]

export function SensitiveReplacementSection({
  defaultValues,
}: SensitiveReplacementSectionProps) {
  const { t } = useTranslation()
  const updateOption = useUpdateOption()
  const form = useForm<SensitiveReplacementFormValues>({
    resolver: zodResolver(sensitiveReplacementSchema),
    defaultValues,
  })

  useEffect(() => {
    form.reset(defaultValues)
  }, [defaultValues, form])

  const onSubmit = async (values: SensitiveReplacementFormValues) => {
    const updates = Object.entries(values).filter(
      ([key, value]) =>
        value !== defaultValues[key as keyof SensitiveReplacementFormValues]
    )

    for (const [key, value] of updates) {
      await updateOption.mutateAsync({ key, value: value ?? '' })
    }
  }

  return (
    <SettingsSection title={t('Sensitive Word Replacement')}>
      <Form {...form}>
        <SettingsForm onSubmit={form.handleSubmit(onSubmit)}>
          <SettingsPageFormActions
            onSave={form.handleSubmit(onSubmit)}
            isSaving={updateOption.isPending}
            saveLabel='Save replacement settings'
          />
          <FormField
            control={form.control}
            name='SensitiveReplacementEnabled'
            render={({ field }) => (
              <SettingsSwitchItem>
                <SettingsSwitchContent>
                  <FormLabel>{t('Enable replacement')}</FormLabel>
                  <FormDescription>
                    {t(
                      'Replace matched words before billing and upstream forwarding.'
                    )}
                  </FormDescription>
                </SettingsSwitchContent>
                <FormControl>
                  <Switch
                    checked={field.value}
                    onCheckedChange={field.onChange}
                  />
                </FormControl>
              </SettingsSwitchItem>
            )}
          />

          <FormField
            control={form.control}
            name='SensitiveReplacementRules'
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t('Replacement rules')}</FormLabel>
                <FormControl>
                  <Textarea
                    rows={12}
                    placeholder={t(
                      'Sensitive words replacement rule placeholder'
                    )}
                    {...field}
                  />
                </FormControl>
                <FormDescription>
                  {t(
                    'Enter one rule per line. Use keyword=>replacement, omit replacement to use XX, and start a line with # or // for comments.'
                  )}
                </FormDescription>
                <FormMessage />
              </FormItem>
            )}
          />

          <SettingsControlGroup className='space-y-3'>
            <FormField
              control={form.control}
              name='SensitiveReplacementLogRetentionDays'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('Replacement log retention days')}</FormLabel>
                  <FormControl>
                    <Input
                      type='number'
                      min={0}
                      step={1}
                      value={field.value}
                      onChange={(event) =>
                        field.onChange(Number(event.target.value))
                      }
                    />
                  </FormControl>
                  <FormDescription>
                    {t(
                      'Automatically clean replacement records older than this many days. Set 0 to disable automatic cleanup.'
                    )}
                  </FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />
          </SettingsControlGroup>
        </SettingsForm>
      </Form>
      <SensitiveReplacementLogsPanel />
    </SettingsSection>
  )
}

function SensitiveReplacementLogsPanel() {
  const { t } = useTranslation()
  const [page, setPage] = useState(1)
  const [cleanupDate, setCleanupDate] = useState<Date | undefined>(() =>
    getDateDaysAgo(30)
  )
  const [cleanupTask, setCleanupTask] =
    useState<SensitiveReplacementLogCleanupTask | null>(null)
  const [isStartingCleanup, setIsStartingCleanup] = useState(false)
  const [showCleanupConfirmDialog, setShowCleanupConfirmDialog] =
    useState(false)
  const logsQuery = useQuery({
    queryKey: ['sensitive-replacement-logs', page],
    queryFn: () =>
      getSensitiveReplacementLogs({
        p: page,
        page_size: REPLACEMENT_LOG_PAGE_SIZE,
      }),
  })

  const pageData = logsQuery.data?.success ? logsQuery.data.data : undefined
  const logs = pageData?.items ?? []
  const total = pageData?.total ?? 0
  const refetchLogs = logsQuery.refetch
  const totalPages = Math.max(
    1,
    Math.ceil(total / REPLACEMENT_LOG_PAGE_SIZE)
  )
  const cleanupTimestamp = useMemo(() => {
    if (!cleanupDate) return null
    return Math.floor(cleanupDate.getTime() / 1000)
  }, [cleanupDate])
  const formattedCleanupDate = useMemo(() => {
    if (!cleanupDate) return ''
    return formatTimestampToDate(cleanupDate.getTime(), 'milliseconds')
  }, [cleanupDate])
  const cleanupActive = isActiveSensitiveReplacementLogCleanupTask(cleanupTask)
  const cleanupTaskState = cleanupTask?.state
  const cleanupProgress = Math.min(
    100,
    Math.max(0, cleanupTaskState?.progress ?? 0)
  )
  const cleanupProcessed = cleanupTaskState?.processed ?? 0
  const cleanupTotal = cleanupTaskState?.total ?? 0
  const cleanupTaskId = cleanupTask?.task_id

  useEffect(() => {
    if (page > totalPages) {
      setPage(totalPages)
    }
  }, [page, totalPages])

  useEffect(() => {
    let cancelled = false

    async function fetchCurrentCleanupTask() {
      try {
        const res = await getCurrentSensitiveReplacementLogCleanupTask()
        if (!cancelled && res.success && res.data) {
          setCleanupTask(res.data)
        }
      } catch {
        /* ignore */
      }
    }

    fetchCurrentCleanupTask()

    return () => {
      cancelled = true
    }
  }, [])

  useEffect(() => {
    if (!cleanupTaskId || !cleanupActive) return

    let cancelled = false
    const interval = window.setInterval(async () => {
      try {
        const res =
          await getSystemTask<SensitiveReplacementLogCleanupTask>(cleanupTaskId)
        if (!cancelled && res.success && res.data) {
          setCleanupTask(res.data)
          if (!isActiveSensitiveReplacementLogCleanupTask(res.data)) {
            await refetchLogs()
          }
        }
      } catch (error) {
        if (!cancelled) {
          toast.error(
            error instanceof Error
              ? error.message
              : t('Failed to refresh cleanup progress')
          )
        }
      }
    }, CLEANUP_TASK_POLL_INTERVAL_MS)

    return () => {
      cancelled = true
      window.clearInterval(interval)
    }
  }, [cleanupActive, cleanupTaskId, refetchLogs, t])

  const handleRequestCleanup = () => {
    if (!cleanupTimestamp) {
      toast.error(t('Select a timestamp before clearing replacement records.'))
      return
    }
    setShowCleanupConfirmDialog(true)
  }

  const handleCleanup = async () => {
    if (!cleanupTimestamp) {
      toast.error(t('Select a timestamp before clearing replacement records.'))
      return
    }

    setIsStartingCleanup(true)
    try {
      const res =
        await startSensitiveReplacementLogCleanupTask(cleanupTimestamp)
      if (!res.success) {
        throw new Error(
          res.message || t('Failed to clean replacement records')
        )
      }
      if (!res.data) {
        throw new Error(t('Failed to clean replacement records'))
      }
      setCleanupTask(res.data)
      setShowCleanupConfirmDialog(false)
      toast.success(t('Replacement record cleanup task started.'))
    } catch (error) {
      const message =
        error instanceof Error
          ? error.message
          : t('Failed to clean replacement records')
      toast.error(message)
    } finally {
      setIsStartingCleanup(false)
    }
  }

  return (
    <>
      <div className='min-w-0 rounded-lg border'>
        <div className='flex flex-col gap-3 border-b px-3 py-3 sm:flex-row sm:items-center sm:justify-between'>
          <div className='min-w-0 space-y-1'>
            <h4 className='text-sm font-medium'>{t('Recent replacements')}</h4>
            <p className='text-muted-foreground text-xs'>
              {t('Only context snippets are stored, not full prompts.')}
            </p>
          </div>
          <Button
            type='button'
            variant='outline'
            size='sm'
            onClick={() => logsQuery.refetch()}
            disabled={logsQuery.isFetching}
          >
            <RefreshCw
              className={
                logsQuery.isFetching ? 'size-4 animate-spin' : 'size-4'
              }
            />
            {t('Refresh')}
          </Button>
        </div>
        <div className='space-y-3 border-b px-3 py-3'>
          <div className='flex flex-col gap-3 lg:flex-row lg:items-end'>
            <div className='min-w-0 flex-1 space-y-1.5'>
              <Label className='text-xs'>
                {t('Clean replacement records before')}
              </Label>
              <DateTimePicker
                value={cleanupDate}
                onChange={setCleanupDate}
                placeholder={t('Select cleanup timestamp')}
              />
            </div>
            <div className='flex flex-wrap gap-2'>
              {cleanupQuickSelectOptions.map((option) => (
                <Button
                  key={option.label}
                  type='button'
                  variant='outline'
                  size='sm'
                  onClick={() => setCleanupDate(option.getValue())}
                >
                  {t(option.label)}
                </Button>
              ))}
              <Button
                type='button'
                variant='destructive'
                size='sm'
                onClick={handleRequestCleanup}
                disabled={isStartingCleanup || cleanupActive}
              >
                <Trash2 data-icon='inline-start' />
                {isStartingCleanup || cleanupActive
                  ? t('Cleaning...')
                  : t('Clean records')}
              </Button>
            </div>
          </div>
          {cleanupTask && (
            <div className='rounded-md border p-3'>
              <div className='mb-2 flex items-center justify-between gap-3 text-sm'>
                <span className='font-medium'>
                  {t('Replacement record cleanup progress')}
                </span>
                <span className='text-muted-foreground tabular-nums'>
                  {cleanupProgress}%
                </span>
              </div>
              <Progress value={cleanupProgress} />
              <div className='text-muted-foreground mt-2 text-xs'>
                {t('{{processed}} of {{total}} replacement records processed.', {
                  processed: cleanupProcessed,
                  total: cleanupTotal,
                })}
              </div>
              {cleanupTask.status === 'failed' && cleanupTask.error && (
                <div className='text-destructive mt-2 text-xs'>
                  {cleanupTask.error}
                </div>
              )}
            </div>
          )}
        </div>
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead className='w-[9.5rem]'>{t('Time')}</TableHead>
              <TableHead className='w-[10rem]'>{t('User / Token')}</TableHead>
              <TableHead className='w-[14rem]'>{t('Request')}</TableHead>
              <TableHead className='w-[11rem]'>{t('Rule')}</TableHead>
              <TableHead className='w-[5rem]'>{t('Count')}</TableHead>
              <TableHead>{t('Context snippets')}</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            <ReplacementLogsTableRows
              isLoading={logsQuery.isLoading}
              logs={logs}
            />
          </TableBody>
        </Table>
        <div className='flex flex-col gap-3 border-t px-3 py-2.5 sm:flex-row sm:items-center sm:justify-between'>
          <p className='text-muted-foreground text-xs'>
            {t('Page {{page}} of {{total}}', {
              page,
              total: totalPages,
            })}
          </p>
          <div className='flex items-center gap-2'>
            <Button
              type='button'
              variant='outline'
              size='sm'
              onClick={() => setPage((current) => Math.max(1, current - 1))}
              disabled={page <= 1 || logsQuery.isFetching}
            >
              <ChevronLeft className='size-4' />
              {t('Previous')}
            </Button>
            <Button
              type='button'
              variant='outline'
              size='sm'
              onClick={() =>
                setPage((current) => Math.min(totalPages, current + 1))
              }
              disabled={page >= totalPages || logsQuery.isFetching}
            >
              {t('Next')}
              <ChevronRight className='size-4' />
            </Button>
          </div>
        </div>
      </div>

      <AlertDialog
        open={showCleanupConfirmDialog}
        onOpenChange={setShowCleanupConfirmDialog}
      >
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>
              {t('Confirm replacement record cleanup')}
            </AlertDialogTitle>
            <AlertDialogDescription>
              {formattedCleanupDate
                ? t(
                    'This will permanently remove sensitive replacement records created before {{date}}.',
                    { date: formattedCleanupDate }
                  )
                : t(
                    'This will permanently remove sensitive replacement records before the selected timestamp.'
                  )}{' '}
              {t('This action cannot be undone.')}
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel disabled={isStartingCleanup}>
              {t('Cancel')}
            </AlertDialogCancel>
            <AlertDialogAction
              variant='destructive'
              onClick={handleCleanup}
              disabled={isStartingCleanup}
            >
              {isStartingCleanup ? t('Cleaning...') : t('Delete records')}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </>
  )
}

function isActiveSensitiveReplacementLogCleanupTask(
  task: SensitiveReplacementLogCleanupTask | null
) {
  return task?.status === 'pending' || task?.status === 'running'
}

function ReplacementLogRow(props: { log: SensitiveReplacementLog }) {
  const { t } = useTranslation()
  const log = props.log

  return (
    <TableRow>
      <TableCell className='align-top text-xs'>
        {formatTimestampToDate(log.created_at)}
      </TableCell>
      <TableCell className='align-top'>
        <div className='min-w-0 space-y-1'>
          <p className='truncate font-medium'>
            {log.username || `#${log.user_id}`}
          </p>
          <p className='text-muted-foreground truncate text-xs'>
            {log.token_name || (log.token_id ? `#${log.token_id}` : '-')}
          </p>
        </div>
      </TableCell>
      <TableCell className='align-top'>
        <div className='min-w-0 space-y-1'>
          <p className='truncate font-mono text-xs'>{log.request_path}</p>
          <p className='text-muted-foreground truncate font-mono text-xs'>
            {log.request_id || '-'}
          </p>
          <p className='text-muted-foreground truncate text-xs'>
            {log.model_name || '-'}
          </p>
        </div>
      </TableCell>
      <TableCell className='align-top'>
        <div className='flex min-w-0 flex-wrap items-center gap-1.5'>
          <Badge variant='outline'>{log.matched_word}</Badge>
          <span className='text-muted-foreground text-xs'>=&gt;</span>
          <Badge variant='secondary'>{log.replacement}</Badge>
        </div>
      </TableCell>
      <TableCell className='align-top'>
        <Badge variant='outline'>{log.count}</Badge>
      </TableCell>
      <TableCell className='max-w-[28rem] align-top whitespace-normal'>
        <div className='space-y-1.5 text-xs'>
          <p className='break-words'>
            <span className='text-foreground font-medium'>{t('Before')}:</span>{' '}
            <span className='text-muted-foreground'>
              {log.original_context || '-'}
            </span>
          </p>
          <p className='break-words'>
            <span className='text-foreground font-medium'>{t('After')}:</span>{' '}
            <span className='text-muted-foreground'>
              {log.replaced_context || '-'}
            </span>
          </p>
        </div>
      </TableCell>
    </TableRow>
  )
}

function ReplacementLogsTableRows(props: {
  isLoading: boolean
  logs: SensitiveReplacementLog[]
}) {
  const { t } = useTranslation()

  if (props.isLoading) {
    return <ReplacementLogsSkeleton />
  }

  if (props.logs.length > 0) {
    return props.logs.map((log) => (
      <ReplacementLogRow key={log.id} log={log} />
    ))
  }

  return (
    <TableRow>
      <TableCell
        colSpan={6}
        className='text-muted-foreground h-28 text-center'
      >
        {t('No replacements recorded')}
      </TableCell>
    </TableRow>
  )
}

function ReplacementLogsSkeleton() {
  const rows = ['first', 'second', 'third']
  const cells = ['time', 'user', 'request', 'rule', 'count', 'context']

  return rows.map((row) => (
    <TableRow key={row}>
      {cells.map((cell) => (
        <TableCell key={cell}>
          <Skeleton className='h-5 w-full' />
        </TableCell>
      ))}
    </TableRow>
  ))
}
