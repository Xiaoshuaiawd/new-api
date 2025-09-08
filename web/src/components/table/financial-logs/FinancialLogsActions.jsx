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

import React from 'react';
import { Tag, Space, Skeleton, Button } from '@douyinfe/semi-ui';
import { IconRefresh } from '@douyinfe/semi-icons';
import CompactModeToggle from '../../common/ui/CompactModeToggle';
import { useMinimumLoadingTime } from '../../../hooks/common/useMinimumLoadingTime';

const FinancialLogsActions = ({
  logCount,
  loading,
  compactMode,
  setCompactMode,
  refresh,
  useCursor,
  hasMore,
  t,
}) => {
  const showSkeleton = useMinimumLoadingTime(loading);
  const needSkeleton = showSkeleton;

  const placeholder = (
    <Space>
      <Skeleton.Title style={{ width: 120, height: 21, borderRadius: 6 }} />
      <Skeleton.Title style={{ width: 80, height: 21, borderRadius: 6 }} />
    </Space>
  );

  return (
    <div className='flex flex-col md:flex-row justify-between items-start md:items-center gap-2 w-full'>
      <Skeleton loading={needSkeleton} active placeholder={placeholder}>
        <Space>
          <Tag
            color='blue'
            style={{
              fontWeight: 500,
              boxShadow: '0 2px 8px rgba(0, 0, 0, 0.1)',
              padding: 13,
            }}
            className='!rounded-lg'
          >
            {useCursor 
              ? t('当前显示条数: {{count}}', { count: logCount || 0 })
              : t('总条数: {{count}}', { count: logCount || 0 })
            }
          </Tag>
          
          {useCursor && (
            <Tag
              color={hasMore ? 'green' : 'grey'}
              style={{
                fontWeight: 500,
                boxShadow: '0 2px 8px rgba(0, 0, 0, 0.1)',
                padding: 13,
              }}
              className='!rounded-lg'
            >
              {hasMore ? t('有更多数据') : t('已加载全部')}
            </Tag>
          )}

          <Button
            type='tertiary'
            icon={<IconRefresh />}
            onClick={refresh}
            loading={loading}
            size='small'
            style={{
              boxShadow: '0 2px 8px rgba(0, 0, 0, 0.1)',
            }}
            className='!rounded-lg'
          >
            {t('刷新')}
          </Button>
        </Space>
      </Skeleton>

      <CompactModeToggle
        compactMode={compactMode}
        setCompactMode={setCompactMode}
        t={t}
      />
    </div>
  );
};

export default FinancialLogsActions;
