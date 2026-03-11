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

import { useMemo } from 'react';
import { Wallet, Activity, Zap, Gauge } from 'lucide-react';
import {
  IconMoneyExchangeStroked,
  IconHistogram,
  IconCoinMoneyStroked,
  IconTextStroked,
  IconPulse,
  IconStopwatchStroked,
  IconTypograph,
  IconSend,
} from '@douyinfe/semi-icons';
import { renderQuota } from '../../helpers';
import { createSectionTitle } from '../../helpers/dashboard';

export const useDashboardStats = (
  userState,
  consumeQuota,
  consumeTokens,
  times,
  trendData,
  performanceMetrics,
  navigate,
  t,
  subscriptionOnlyModeEnabled,
  subscriptionInfo,
) => {
  const groupedStatsData = useMemo(() => {
    if (subscriptionOnlyModeEnabled) {
      const activeSubs = subscriptionInfo?.active || [];
      const currentSub = activeSubs.find((s) => s?.subscription)?.subscription;
      const total = currentSub?.amount_total || 0;
      const used = currentSub?.amount_used || 0;
      const remaining =
        total > 0 ? Math.max(total - used, 0) : Number.POSITIVE_INFINITY;
      const nextReset = currentSub?.next_reset_time || 0;
      const endTime = currentSub?.end_time || 0;
      const formatTime = (ts) => {
        if (!ts) return t('不重置');
        const d = new Date(ts * 1000);
        return d.toLocaleString();
      };
      return [
        {
          title: createSectionTitle(Wallet, t('订阅数据')),
          color: 'bg-blue-50',
          items: [
            {
              title: t('订阅剩余额度'),
              value:
                remaining === Number.POSITIVE_INFINITY
                  ? t('无限')
                  : renderQuota(remaining),
              icon: <IconMoneyExchangeStroked />,
              avatarColor: 'blue',
              trendData: [],
              trendColor: '#3b82f6',
              onClick: () => navigate('/topup'),
            },
            {
              title: t('下次刷新时间'),
              value: formatTime(nextReset),
              icon: <IconStopwatchStroked />,
              avatarColor: 'indigo',
              trendData: [],
              trendColor: '#6366f1',
              onClick: () => navigate('/topup'),
            },
            {
              title: t('订阅到期时间'),
              value: endTime ? formatTime(endTime) : t('无到期时间'),
              icon: <IconHistogram />,
              avatarColor: 'purple',
              trendData: [],
              trendColor: '#8b5cf6',
              onClick: () => navigate('/topup'),
            },
          ],
        },
      ];
    }

    return [
      {
        title: createSectionTitle(Wallet, t('账户数据')),
        color: 'bg-blue-50',
        items: [
          {
            title: t('当前余额'),
            value: renderQuota(userState?.user?.quota),
            icon: <IconMoneyExchangeStroked />,
            avatarColor: 'blue',
            trendData: [],
            trendColor: '#3b82f6',
          },
          {
            title: t('历史消耗'),
            value: renderQuota(userState?.user?.used_quota),
            icon: <IconHistogram />,
            avatarColor: 'purple',
            trendData: [],
            trendColor: '#8b5cf6',
          },
        ],
      },
      {
        title: createSectionTitle(Activity, t('使用统计')),
        color: 'bg-green-50',
        items: [
          {
            title: t('请求次数'),
            value: userState.user?.request_count,
            icon: <IconSend />,
            avatarColor: 'green',
            trendData: [],
            trendColor: '#10b981',
          },
          {
            title: t('统计次数'),
            value: times,
            icon: <IconPulse />,
            avatarColor: 'cyan',
            trendData: trendData.times,
            trendColor: '#06b6d4',
          },
        ],
      },
      {
        title: createSectionTitle(Zap, t('资源消耗')),
        color: 'bg-yellow-50',
        items: [
          {
            title: t('统计额度'),
            value: renderQuota(consumeQuota),
            icon: <IconCoinMoneyStroked />,
            avatarColor: 'yellow',
            trendData: trendData.consumeQuota,
            trendColor: '#f59e0b',
          },
          {
            title: t('统计Tokens'),
            value: isNaN(consumeTokens) ? 0 : consumeTokens.toLocaleString(),
            icon: <IconTextStroked />,
            avatarColor: 'pink',
            trendData: trendData.tokens,
            trendColor: '#ec4899',
          },
        ],
      },
      {
        title: createSectionTitle(Gauge, t('性能指标')),
        color: 'bg-indigo-50',
        items: [
          {
            title: t('平均RPM'),
            value: performanceMetrics.avgRPM,
            icon: <IconStopwatchStroked />,
            avatarColor: 'indigo',
            trendData: trendData.rpm,
            trendColor: '#6366f1',
          },
          {
            title: t('平均TPM'),
            value: performanceMetrics.avgTPM,
            icon: <IconTypograph />,
            avatarColor: 'orange',
            trendData: trendData.tpm,
            trendColor: '#f97316',
          },
        ],
      },
    ];
  }, [
    subscriptionOnlyModeEnabled,
    subscriptionInfo,
    userState?.user?.quota,
    userState?.user?.used_quota,
    userState?.user?.request_count,
    times,
    consumeQuota,
    consumeTokens,
    trendData,
    performanceMetrics,
    navigate,
    t,
  ]);

  return {
    groupedStatsData,
  };
};
