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
import { useState } from 'react'
import { Search, Copy, Check, ChevronLeft, ChevronRight } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { formatCurrencyFromUSD } from '@/lib/currency'
import { formatNumber } from '@/lib/format'
import { useCopyToClipboard } from '@/hooks/use-copy-to-clipboard'
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
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { ScrollArea } from '@/components/ui/scroll-area'
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Skeleton } from '@/components/ui/skeleton'
import { StatusBadge } from '@/components/status-badge'
import { useBillingHistory } from '../../hooks/use-billing-history'
import type { TopupRecord, ZPayOrderInfo } from '../../types'
import {
  getStatusConfig,
  getPaymentMethodName,
  formatTimestamp,
} from '../../lib/billing'

interface BillingHistoryDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
}

export function BillingHistoryDialog({
  open,
  onOpenChange,
}: BillingHistoryDialogProps) {
  const { t } = useTranslation()
  const {
    records,
    total,
    page,
    pageSize,
    keyword,
    loading,
    completing,
    querying,
    refunding,
    isAdmin,
    handlePageChange,
    handlePageSizeChange,
    handleSearch,
    handleCompleteOrder,
    handleQueryZPayOrder,
    handleRefundZPayOrder,
  } = useBillingHistory()

  const [confirmTradeNo, setConfirmTradeNo] = useState<string | null>(null)
  const [queryOrderInfo, setQueryOrderInfo] = useState<ZPayOrderInfo | null>(
    null
  )
  const [queryDialogOpen, setQueryDialogOpen] = useState(false)
  const [refundRecord, setRefundRecord] = useState<TopupRecord | null>(null)
  const { copyToClipboard, copiedText } = useCopyToClipboard({ notify: false })

  const totalPages = Math.ceil(total / pageSize)

  const handleConfirmComplete = async () => {
    if (confirmTradeNo) {
      const success = await handleCompleteOrder(confirmTradeNo)
      if (success) {
        setConfirmTradeNo(null)
      }
    }
  }

  const handleQueryOrder = async (tradeNo: string) => {
    const info = await handleQueryZPayOrder(tradeNo)
    if (info) {
      setQueryOrderInfo(info)
      setQueryDialogOpen(true)
    }
  }

  const handleConfirmRefund = async () => {
    if (refundRecord) {
      const result = await handleRefundZPayOrder(refundRecord.trade_no)
      if (result !== null) {
        setRefundRecord(null)
      }
    }
  }

  const renderZPayActions = (record: TopupRecord) => {
    if (!isAdmin) {
      return null
    }

    const isZPayOrder =
      record.payment_provider === 'zpay' || record.payment_method.startsWith('zpay_')
    const disabled = completing || querying || refunding
    return (
      <div className='mt-4 flex flex-wrap justify-end gap-2'>
        {record.status === 'pending' && (
          <Button
            size='sm'
            variant='outline'
            onClick={() => setConfirmTradeNo(record.trade_no)}
            disabled={disabled}
          >
            {t('Complete Order')}
          </Button>
        )}
        {isZPayOrder && (
          <>
            <Button
              size='sm'
              variant='outline'
              onClick={() => handleQueryOrder(record.trade_no)}
              disabled={disabled}
            >
              {t('Query Order')}
            </Button>
            {record.status === 'success' && (
              <Button
                size='sm'
                variant='destructive'
                onClick={() => setRefundRecord(record)}
                disabled={disabled}
              >
                {t('Refund')}
              </Button>
            )}
          </>
        )}
      </div>
    )
  }

  const zPayOrderFields = queryOrderInfo
    ? [
        { label: t('Platform Order No.'), value: queryOrderInfo.trade_no },
        { label: t('Merchant Order No.'), value: queryOrderInfo.out_trade_no },
        { label: t('Amount'), value: queryOrderInfo.money },
        {
          label: t('Payment Method'),
          value: queryOrderInfo.type
            ? getPaymentMethodName(`zpay_${queryOrderInfo.type}`, t)
            : undefined,
        },
        {
          label: t('Payment Status'),
          value:
            queryOrderInfo.status === undefined
              ? undefined
              : String(queryOrderInfo.status),
        },
        { label: t('Created At'), value: queryOrderInfo.addtime },
        { label: t('Completed At'), value: queryOrderInfo.endtime },
        { label: t('Buyer Account'), value: queryOrderInfo.buyer },
      ]
    : []

  return (
    <>
      <Dialog open={open} onOpenChange={onOpenChange}>
        <DialogContent className='flex max-h-[calc(100dvh-2rem)] flex-col max-sm:h-dvh max-sm:w-screen max-sm:max-w-none max-sm:rounded-none max-sm:p-4 sm:max-w-4xl'>
          <DialogHeader>
            <DialogTitle>{t('Billing History')}</DialogTitle>
            <DialogDescription>
              {t('View your topup transaction records and payment history')}
            </DialogDescription>
          </DialogHeader>

          <div className='min-h-0 flex-1 space-y-3 sm:space-y-4'>
            {/* Search and Filter Bar */}
            <div className='flex items-center gap-2'>
              <div className='relative flex-1'>
                <Search className='text-muted-foreground absolute top-1/2 left-3 h-4 w-4 -translate-y-1/2' />
                <Input
                  placeholder={t('Search by order number...')}
                  value={keyword}
                  onChange={(e) => handleSearch(e.target.value)}
                  className='h-9 pl-10'
                />
              </div>
              <Select
                items={[
                  { value: '10', label: t('10 / page') },
                  { value: '20', label: t('20 / page') },
                  { value: '50', label: t('50 / page') },
                  { value: '100', label: t('100 / page') },
                ]}
                value={pageSize.toString()}
                onValueChange={(value) =>
                  value !== null && handlePageSizeChange(parseInt(value))
                }
              >
                <SelectTrigger className='h-9 w-[92px] sm:w-32'>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent alignItemWithTrigger={false}>
                  <SelectGroup>
                    <SelectItem value='10'>{t('10 / page')}</SelectItem>
                    <SelectItem value='20'>{t('20 / page')}</SelectItem>
                    <SelectItem value='50'>{t('50 / page')}</SelectItem>
                    <SelectItem value='100'>{t('100 / page')}</SelectItem>
                  </SelectGroup>
                </SelectContent>
              </Select>
            </div>

            {/* Records List */}
            <ScrollArea className='h-[calc(100dvh-15rem)] pr-3 sm:h-[500px] sm:pr-4'>
              {loading ? (
                <div className='space-y-3'>
                  {Array.from({ length: 5 }).map((_, i) => (
                    <div key={i} className='rounded-lg border p-3 sm:p-4'>
                      <div className='flex items-start justify-between'>
                        <div className='flex-1 space-y-2'>
                          <Skeleton className='h-4 w-48' />
                          <Skeleton className='h-3 w-32' />
                        </div>
                        <Skeleton className='h-5 w-16' />
                      </div>
                      <div className='mt-3 grid grid-cols-2 gap-3 sm:grid-cols-3 sm:gap-4'>
                        <Skeleton className='h-3 w-full' />
                        <Skeleton className='h-3 w-full' />
                        <Skeleton className='h-3 w-full' />
                      </div>
                    </div>
                  ))}
                </div>
              ) : records.length === 0 ? (
                <div className='text-muted-foreground flex h-[320px] flex-col items-center justify-center text-center sm:h-[400px]'>
                  <p className='text-sm font-medium'>
                    {t('No billing records found')}
                  </p>
                  <p className='mt-1 text-xs'>
                    {keyword
                      ? t('Try adjusting your search')
                      : t('Your transaction history will appear here')}
                  </p>
                </div>
              ) : (
                <div className='space-y-3'>
                  {records.map((record) => {
                    const statusConfig = getStatusConfig(record.status)
                    return (
                      <div
                        key={record.id}
                        className='hover:bg-muted/50 rounded-lg border p-3 transition-colors sm:p-4'
                      >
                        {/* Header Row */}
                        <div className='flex items-start justify-between gap-2'>
                          <div className='flex-1 space-y-1'>
                            <div className='flex min-w-0 items-center gap-2'>
                              <code className='text-foreground truncate font-mono text-sm'>
                                {record.trade_no}
                              </code>
                              <Button
                                variant='ghost'
                                size='sm'
                                className='h-5 w-5 p-0'
                                onClick={() => copyToClipboard(record.trade_no)}
                              >
                                {copiedText === record.trade_no ? (
                                  <Check className='h-3 w-3' />
                                ) : (
                                  <Copy className='h-3 w-3' />
                                )}
                              </Button>
                              {isAdmin && record.user_id != null && (
                                <StatusBadge
                                  label={`${t('User ID')}: ${record.user_id}`}
                                  variant='neutral'
                                  size='sm'
                                  copyText={String(record.user_id)}
                                />
                              )}
                            </div>
                            <div className='text-muted-foreground text-xs'>
                              {formatTimestamp(record.create_time)}
                            </div>
                          </div>
                          <StatusBadge
                            label={statusConfig.label}
                            variant={statusConfig.variant}
                            showDot
                            copyable={false}
                          />
                        </div>

                        {/* Details Grid */}
                        <div className='mt-3 grid grid-cols-2 gap-3 sm:mt-4 sm:grid-cols-3 sm:gap-4'>
                          <div className='space-y-1'>
                            <Label className='text-muted-foreground text-xs'>
                              {t('Payment Method')}
                            </Label>
                            <div className='text-sm font-medium'>
                              {getPaymentMethodName(record.payment_method, t)}
                            </div>
                          </div>
                          <div className='space-y-1'>
                            <Label className='text-muted-foreground text-xs'>
                              {t('Amount')}
                            </Label>
                            <div className='text-sm font-semibold'>
                              {formatCurrencyFromUSD(record.amount, {
                                digitsLarge: 2,
                                digitsSmall: 2,
                                abbreviate: false,
                              })}
                            </div>
                          </div>
                          <div className='space-y-1'>
                            <Label className='text-muted-foreground text-xs'>
                              {t('Payment')}
                            </Label>
                            <div className='text-sm font-semibold text-red-600'>
                              {formatNumber(record.money)}
                            </div>
                          </div>
                        </div>

                        {/* Admin Actions */}
                        {renderZPayActions(record)}
                      </div>
                    )
                  })}
                </div>
              )}
            </ScrollArea>

            {/* Pagination */}
            {!loading && records.length > 0 && (
              <div className='flex flex-col items-center gap-3 border-t pt-4 sm:flex-row sm:items-center sm:justify-between'>
                <div className='text-muted-foreground text-xs sm:text-sm'>
                  {t('Showing')} {(page - 1) * pageSize + 1}-
                  {Math.min(page * pageSize, total)} {t('of')} {total}
                </div>
                <div className='flex items-center gap-2'>
                  <Button
                    variant='outline'
                    size='sm'
                    onClick={() => handlePageChange(page - 1)}
                    disabled={page <= 1}
                    className='h-8 w-8 p-0'
                  >
                    <ChevronLeft className='h-4 w-4' />
                  </Button>
                  <div className='text-muted-foreground flex items-center gap-1 text-sm'>
                    <span className='font-medium'>{page}</span>
                    <span>/</span>
                    <span>{totalPages}</span>
                  </div>
                  <Button
                    variant='outline'
                    size='sm'
                    onClick={() => handlePageChange(page + 1)}
                    disabled={page >= totalPages}
                    className='h-8 w-8 p-0'
                  >
                    <ChevronRight className='h-4 w-4' />
                  </Button>
                </div>
              </div>
            )}
          </div>
        </DialogContent>
      </Dialog>

      {/* Confirm Complete Order Dialog */}
      <AlertDialog
        open={!!confirmTradeNo}
        onOpenChange={(open) => !open && setConfirmTradeNo(null)}
      >
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>{t('Complete Order')}</AlertDialogTitle>
            <AlertDialogDescription>
              {t(
                'Are you sure you want to manually complete this order? The user will be credited with the corresponding quota.'
              )}
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel disabled={completing}>
              {t('Cancel')}
            </AlertDialogCancel>
            <AlertDialogAction
              onClick={handleConfirmComplete}
              disabled={completing}
            >
              {completing ? t('Processing...') : t('Confirm')}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>

      {/* Z Pay Query Result Dialog */}
      <Dialog open={queryDialogOpen} onOpenChange={setQueryDialogOpen}>
        <DialogContent className='sm:max-w-lg'>
          <DialogHeader>
            <DialogTitle>{t('Z Pay Order Details')}</DialogTitle>
            <DialogDescription>
              {queryOrderInfo?.msg || t('Provider order query result')}
            </DialogDescription>
          </DialogHeader>
          <div className='grid gap-3 sm:grid-cols-2'>
            {zPayOrderFields.map((field) => (
              <div key={field.label} className='space-y-1'>
                <Label className='text-muted-foreground text-xs'>
                  {field.label}
                </Label>
                <div className='break-all text-sm font-medium'>
                  {field.value || '-'}
                </div>
              </div>
            ))}
          </div>
        </DialogContent>
      </Dialog>

      {/* Confirm Z Pay Refund Dialog */}
      <AlertDialog
        open={!!refundRecord}
        onOpenChange={(open) => !open && setRefundRecord(null)}
      >
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>{t('Refund Z Pay Order')}</AlertDialogTitle>
            <AlertDialogDescription>
              {t(
                'Refund this Z Pay order? The local credited quota from this top-up will be deducted after the provider refund succeeds.'
              )}
            </AlertDialogDescription>
          </AlertDialogHeader>
          {refundRecord && (
            <div className='space-y-2 rounded-md border p-3 text-sm'>
              <div className='flex justify-between gap-4'>
                <span className='text-muted-foreground'>{t('Order No.')}</span>
                <span className='break-all font-mono'>
                  {refundRecord.trade_no}
                </span>
              </div>
              <div className='flex justify-between gap-4'>
                <span className='text-muted-foreground'>{t('User ID')}</span>
                <span>{refundRecord.user_id}</span>
              </div>
              <div className='flex justify-between gap-4'>
                <span className='text-muted-foreground'>
                  {t('Refund Amount')}
                </span>
                <span>{formatNumber(refundRecord.money)}</span>
              </div>
            </div>
          )}
          <AlertDialogFooter>
            <AlertDialogCancel disabled={refunding}>
              {t('Cancel')}
            </AlertDialogCancel>
            <AlertDialogAction
              onClick={handleConfirmRefund}
              disabled={refunding}
            >
              {refunding ? t('Processing...') : t('Confirm Refund')}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </>
  )
}
