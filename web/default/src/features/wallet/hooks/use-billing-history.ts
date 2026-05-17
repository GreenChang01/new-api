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
import { useState, useEffect, useCallback } from 'react'
import i18next from 'i18next'
import { toast } from 'sonner'
import { useIsAdmin } from '@/hooks/use-admin'
import {
  getUserBillingHistory,
  getAllBillingHistory,
  completeOrder,
  queryZPayOrder,
  refundZPayOrder,
  isApiSuccess,
} from '../api'
import type { TopupRecord, ZPayOrderInfo, ZPayRefundResult } from '../types'

// ============================================================================
// Billing History Hook
// ============================================================================

interface UseBillingHistoryOptions {
  /** Initial page number */
  initialPage?: number
  /** Initial page size */
  initialPageSize?: number
}

export function useBillingHistory(options: UseBillingHistoryOptions = {}) {
  const { initialPage = 1, initialPageSize = 10 } = options
  const isAdmin = useIsAdmin()

  const [records, setRecords] = useState<TopupRecord[]>([])
  const [total, setTotal] = useState(0)
  const [page, setPage] = useState(initialPage)
  const [pageSize, setPageSize] = useState(initialPageSize)
  const [keyword, setKeyword] = useState('')
  const [loading, setLoading] = useState(false)
  const [completing, setCompleting] = useState(false)
  const [querying, setQuerying] = useState(false)
  const [refunding, setRefunding] = useState(false)

  /**
   * Fetch billing history
   */
  const fetchBillingHistory = useCallback(async () => {
    setLoading(true)
    try {
      const response = isAdmin
        ? await getAllBillingHistory(page, pageSize, keyword)
        : await getUserBillingHistory(page, pageSize, keyword)

      if (isApiSuccess(response) && response.data) {
        setRecords(response.data.items || [])
        setTotal(response.data.total || 0)
      } else {
        toast.error(
          response.message || i18next.t('Failed to load billing history')
        )
        setRecords([])
        setTotal(0)
      }
    } catch (error) {
      // eslint-disable-next-line no-console
      console.error('Failed to fetch billing history:', error)
      toast.error(i18next.t('Failed to load billing history'))
      setRecords([])
      setTotal(0)
    } finally {
      setLoading(false)
    }
  }, [isAdmin, page, pageSize, keyword])

  /**
   * Complete a pending order (admin only)
   */
  const handleCompleteOrder = useCallback(
    async (tradeNo: string) => {
      if (!isAdmin) {
        toast.error(i18next.t('Admin access required'))
        return false
      }

      setCompleting(true)
      try {
        const response = await completeOrder({ trade_no: tradeNo })
        if (isApiSuccess(response)) {
          toast.success(i18next.t('Order completed successfully'))
          // Refresh the list
          await fetchBillingHistory()
          return true
        } else {
          toast.error(response.message || i18next.t('Failed to complete order'))
          return false
        }
      } catch (error) {
        // eslint-disable-next-line no-console
        console.error('Failed to complete order:', error)
        toast.error(i18next.t('Failed to complete order'))
        return false
      } finally {
        setCompleting(false)
      }
    },
    [isAdmin, fetchBillingHistory]
  )

  /**
   * Query a Z Pay order from provider (admin only)
   */
  const handleQueryZPayOrder = useCallback(
    async (tradeNo: string): Promise<ZPayOrderInfo | null> => {
      if (!isAdmin) {
        toast.error(i18next.t('Admin access required'))
        return null
      }

      setQuerying(true)
      try {
        const response = await queryZPayOrder({ trade_no: tradeNo })
        if (isApiSuccess(response) && response.data) {
          return response.data
        }
        toast.error(response.message || i18next.t('Failed to query Z Pay order'))
        return null
      } catch (error) {
        // eslint-disable-next-line no-console
        console.error('Failed to query Z Pay order:', error)
        toast.error(i18next.t('Failed to query Z Pay order'))
        return null
      } finally {
        setQuerying(false)
      }
    },
    [isAdmin]
  )

  /**
   * Refund a Z Pay order and roll back local quota (admin only)
   */
  const handleRefundZPayOrder = useCallback(
    async (tradeNo: string): Promise<ZPayRefundResult | null> => {
      if (!isAdmin) {
        toast.error(i18next.t('Admin access required'))
        return null
      }

      setRefunding(true)
      try {
        const response = await refundZPayOrder({ trade_no: tradeNo })
        if (isApiSuccess(response)) {
          toast.success(i18next.t('Z Pay order refunded successfully'))
          await fetchBillingHistory()
          return response.data || null
        }
        toast.error(response.message || i18next.t('Failed to refund Z Pay order'))
        return null
      } catch (error) {
        // eslint-disable-next-line no-console
        console.error('Failed to refund Z Pay order:', error)
        toast.error(i18next.t('Failed to refund Z Pay order'))
        return null
      } finally {
        setRefunding(false)
      }
    },
    [isAdmin, fetchBillingHistory]
  )

  /**
   * Change page
   */
  const handlePageChange = useCallback((newPage: number) => {
    setPage(newPage)
  }, [])

  /**
   * Change page size
   */
  const handlePageSizeChange = useCallback((newPageSize: number) => {
    setPageSize(newPageSize)
    setPage(1) // Reset to first page when changing page size
  }, [])

  /**
   * Search by keyword
   */
  const handleSearch = useCallback((newKeyword: string) => {
    setKeyword(newKeyword)
    setPage(1) // Reset to first page when searching
  }, [])

  // Fetch data when dependencies change
  useEffect(() => {
    fetchBillingHistory()
  }, [fetchBillingHistory])

  return {
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
    refresh: fetchBillingHistory,
  }
}
