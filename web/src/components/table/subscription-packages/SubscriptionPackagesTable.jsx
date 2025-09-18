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
import {
  Table,
  Tag,
  Button,
  Typography,
  Popconfirm,
  Tooltip,
} from '@douyinfe/semi-ui';
import { Edit, Trash2, Users, DollarSign } from 'lucide-react';
import { formatQuota, timestampToTime } from '../../../helpers';

const { Text } = Typography;

const SubscriptionPackagesTable = ({
  packages,
  loading,
  setShowEditPackage,
  setEditingPackage,
  handleDeletePackage,
  compactMode,
  t,
}) => {
  const handleEditPackage = (pkg) => {
    setEditingPackage(pkg);
    setShowEditPackage(true);
  };

  const columns = [
    {
      title: t('套餐名称'),
      dataIndex: 'name',
      key: 'name',
      render: (text, record) => (
        <div className='flex flex-col'>
          <Text strong>{text}</Text>
          {!compactMode && record.description && (
            <Text type='secondary' size='small'>
              {record.description}
            </Text>
          )}
        </div>
      ),
    },
    {
      title: t('永久额度'),
      dataIndex: 'permanent_quota',
      key: 'permanent_quota',
      render: (quota) => (
        <Text type={quota > 0 ? 'success' : 'tertiary'}>
          {quota > 0 ? formatQuota(quota) : '-'}
        </Text>
      ),
    },
    {
      title: t('月额度'),
      dataIndex: 'monthly_quota',
      key: 'monthly_quota',
      render: (quota) => (
        <Text type={quota > 0 ? 'primary' : 'tertiary'}>
          {quota > 0 ? formatQuota(quota) : '-'}
        </Text>
      ),
    },
    {
      title: t('日额度'),
      dataIndex: 'daily_quota',
      key: 'daily_quota',
      render: (quota) => (
        <Text type={quota > 0 ? 'warning' : 'tertiary'}>
          {quota > 0 ? formatQuota(quota) : '-'}
        </Text>
      ),
    },
    {
      title: t('价格'),
      dataIndex: 'price',
      key: 'price',
      render: (price, record) => (
        <div className='flex items-center gap-1'>
          <DollarSign size={14} />
          <Text>
            {price} {record.currency || 'CNY'}
          </Text>
        </div>
      ),
    },
    {
      title: t('持续时间'),
      dataIndex: 'duration',
      key: 'duration',
      render: (duration) => `${duration} ${t('天')}`,
    },
    {
      title: t('状态'),
      dataIndex: 'status',
      key: 'status',
      render: (status) => (
        <Tag color={status === 1 ? 'green' : 'red'}>
          {status === 1 ? t('启用') : t('禁用')}
        </Tag>
      ),
    },
    !compactMode && {
      title: t('创建时间'),
      dataIndex: 'created_time',
      key: 'created_time',
      render: (time) => timestampToTime(time),
    },
    {
      title: t('操作'),
      key: 'action',
      fixed: 'right',
      width: 150,
      render: (_, record) => (
        <div className='flex gap-2'>
          <Tooltip content={t('编辑套餐')}>
            <Button
              size='small'
              type='primary'
              theme='light'
              icon={<Edit size={14} />}
              onClick={() => handleEditPackage(record)}
            />
          </Tooltip>

          <Popconfirm
            title={t('确定删除此套餐吗？')}
            content={t('删除后无法恢复，请谨慎操作')}
            onConfirm={() => handleDeletePackage(record.id)}
          >
            <Tooltip content={t('删除套餐')}>
              <Button
                size='small'
                type='danger'
                theme='light'
                icon={<Trash2 size={14} />}
              />
            </Tooltip>
          </Popconfirm>
        </div>
      ),
    },
  ].filter(Boolean);

  return (
    <Table
      columns={columns}
      dataSource={packages}
      loading={loading}
      size={compactMode ? 'small' : 'default'}
      pagination={false}
      rowKey='id'
      scroll={{ x: 800 }}
    />
  );
};

export default SubscriptionPackagesTable;
