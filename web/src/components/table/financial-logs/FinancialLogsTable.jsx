/*
Copyright (C) 2025 QuantumNous

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

import React, { useMemo } from 'react';
import { Empty } from '@douyinfe/semi-ui';
import CardTable from '../../common/ui/CardTable';
import {
  IllustrationNoResult,
  IllustrationNoResultDark,
} from '@douyinfe/semi-illustrations';
import { getFinancialLogsColumns } from './FinancialLogsColumnDefs';

const FinancialLogsTable = (logsData) => {
  const {
    logs,
    loading,
    activePage,
    pageSize,
    logCount,
    compactMode,
    visibleColumns,
    handlePageChange,
    handlePageSizeChange,
    copyText,
    useCursor,
    hasMore,
    loadNextPage,
    t,
    COLUMN_KEYS,
  } = logsData;

  // Get all columns
  const allColumns = useMemo(() => {
    return getFinancialLogsColumns({
      t,
      COLUMN_KEYS,
      copyText,
    });
  }, [t, COLUMN_KEYS, copyText]);

  // Filter columns based on visibility settings
  const getVisibleColumns = () => {
    return allColumns.filter((column) => visibleColumns[column.key]);
  };

  const visibleColumnsList = useMemo(() => {
    return getVisibleColumns();
  }, [visibleColumns, allColumns]);

  const tableColumns = useMemo(() => {
    return compactMode
      ? visibleColumnsList.map(({ fixed, ...rest }) => rest)
      : visibleColumnsList;
  }, [compactMode, visibleColumnsList]);

  return (
    <CardTable
      columns={tableColumns}
      dataSource={logs}
      rowKey='key'
      loading={loading}
      scroll={compactMode ? undefined : { x: 'max-content' }}
      className='rounded-xl overflow-hidden'
      size='middle'
      empty={
        <Empty
          image={<IllustrationNoResult style={{ width: 150, height: 150 }} />}
          darkModeImage={
            <IllustrationNoResultDark style={{ width: 150, height: 150 }} />
          }
          description={t('暂无数据')}
          style={{ padding: 30 }}
        />
      }
      pagination={
        useCursor
          ? {
              // 游标分页模式的自定义分页控制
              pageSize: pageSize,
              pageSizeOptions: [10, 20, 40, 100],
              showSizeChanger: true,
              onPageSizeChange: (size) => {
                handlePageSizeChange(size);
              },
              // 隐藏页码导航，但保留页面大小选择
              showQuickJumper: false,
              showTotal: (total, range) => {
                return (
                  <div className='flex items-center gap-2'>
                    <span>{t('当前显示: {{count}} 条', { count: logs.length })}</span>
                    {hasMore && (
                      <button
                        onClick={loadNextPage}
                        disabled={loading}
                        className='px-3 py-1 text-sm bg-blue-500 text-white rounded hover:bg-blue-600 disabled:opacity-50'
                      >
                        {loading ? t('加载中...') : t('加载更多')}
                      </button>
                    )}
                  </div>
                );
              },
              // 隐藏页码导航
              itemRender: () => null,
              current: 1,
              total: logs.length,
            }
          : {
              currentPage: activePage,
              pageSize: pageSize,
              total: logCount,
              pageSizeOptions: [10, 20, 40, 100],
              showSizeChanger: true,
              onPageSizeChange: (size) => {
                handlePageSizeChange(size);
              },
              onPageChange: handlePageChange,
            }
      }
      hidePagination={false}
    />
  );
};

export default FinancialLogsTable;
