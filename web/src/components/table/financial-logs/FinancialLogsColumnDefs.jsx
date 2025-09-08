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
import { Tag, Button, Typography } from '@douyinfe/semi-ui';
import { IconCopy } from '@douyinfe/semi-icons';

const { Text } = Typography;

export const getFinancialLogsColumns = ({ t, COLUMN_KEYS, copyText }) => {
  const getTypeColor = (type) => {
    const colorMap = {
      0: 'grey',
      1: 'green',
      2: 'red',
      3: 'blue',
      4: 'orange',
      5: 'red',
    };
    return colorMap[type] || 'grey';
  };

  return [
    {
      title: t('ID'),
      dataIndex: 'id',
      key: COLUMN_KEYS.ID,
      width: 80,
      fixed: 'left',
      render: (text) => (
        <Text copyable={{ content: text }} style={{ fontSize: '12px' }}>
          {text}
        </Text>
      ),
    },
    {
      title: t('时间'),
      dataIndex: 'timestamp2string',
      key: COLUMN_KEYS.CREATED_AT,
      width: 160,
      fixed: 'left',
      sorter: (a, b) => a.created_at - b.created_at,
      render: (text, record) => (
        <Text style={{ fontSize: '12px' }}>{text}</Text>
      ),
    },
    {
      title: t('类型'),
      dataIndex: 'type_display',
      key: COLUMN_KEYS.TYPE,
      width: 80,
      render: (text, record) => (
        <Tag color={getTypeColor(record.type)} size="small">
          {text}
        </Tag>
      ),
    },
    {
      title: t('Token名称'),
      dataIndex: 'token_name',
      key: COLUMN_KEYS.TOKEN_NAME,
      width: 150,
      render: (text) => (
        <div style={{ display: 'flex', alignItems: 'center', gap: '4px' }}>
          <div style={{ 
            maxWidth: '120px', 
            overflow: 'hidden', 
            textOverflow: 'ellipsis',
            whiteSpace: 'nowrap'
          }}>
            <Text 
              style={{ fontSize: '12px' }}
              ellipsis={{ showTooltip: true }}
            >
              {text || '-'}
            </Text>
          </div>
          {text && (
            <Button
              theme="borderless"
              type="tertiary"
              size="small"
              icon={<IconCopy />}
              onClick={(e) => copyText(e, text)}
              style={{ minWidth: 'auto', padding: '2px' }}
            />
          )}
        </div>
      ),
    },
    {
      title: t('模型'),
      dataIndex: 'model_name',
      key: COLUMN_KEYS.MODEL_NAME,
      width: 150,
      render: (text) => (
        <div style={{ display: 'flex', alignItems: 'center', gap: '4px' }}>
          <div style={{ 
            maxWidth: '120px', 
            overflow: 'hidden', 
            textOverflow: 'ellipsis',
            whiteSpace: 'nowrap'
          }}>
            <Text 
              style={{ fontSize: '12px' }}
              ellipsis={{ showTooltip: true }}
            >
              {text || '-'}
            </Text>
          </div>
          {text && (
            <Button
              theme="borderless"
              type="tertiary"
              size="small"
              icon={<IconCopy />}
              onClick={(e) => copyText(e, text)}
              style={{ minWidth: 'auto', padding: '2px' }}
            />
          )}
        </div>
      ),
    },
    {
      title: t('配额'),
      dataIndex: 'quota_display',
      key: COLUMN_KEYS.QUOTA,
      width: 100,
      align: 'right',
      sorter: (a, b) => a.quota - b.quota,
      render: (text, record) => (
        <Text 
          style={{ 
            fontSize: '12px',
            color: record.quota < 0 ? 'var(--semi-color-danger)' : 'inherit'
          }}
        >
          {text}
        </Text>
      ),
    },
    {
      title: t('提示'),
      dataIndex: 'prompt_tokens_display',
      key: COLUMN_KEYS.PROMPT_TOKENS,
      width: 80,
      align: 'right',
      sorter: (a, b) => (a.prompt_tokens || 0) - (b.prompt_tokens || 0),
      render: (text, record) => (
        <Text style={{ fontSize: '12px' }}>
          {(record.prompt_tokens || 0).toLocaleString()}
        </Text>
      ),
    },
    {
      title: t('完成'),
      dataIndex: 'completion_tokens_display',
      key: COLUMN_KEYS.COMPLETION_TOKENS,
      width: 80,
      align: 'right',
      sorter: (a, b) => (a.completion_tokens || 0) - (b.completion_tokens || 0),
      render: (text, record) => (
        <Text style={{ fontSize: '12px' }}>
          {(record.completion_tokens || 0).toLocaleString()}
        </Text>
      ),
    },
    {
      title: t('输入价格'),
      dataIndex: 'input_price_display',
      key: COLUMN_KEYS.INPUT_PRICE,
      width: 120,
      align: 'right',
      render: (text, record) => (
        <Text style={{ fontSize: '12px' }}>
          {record.input_price_display || '-'}
        </Text>
      ),
    },
    {
      title: t('输出价格'),
      dataIndex: 'output_price_display',
      key: COLUMN_KEYS.OUTPUT_PRICE,
      width: 120,
      align: 'right',
      render: (text, record) => (
        <Text style={{ fontSize: '12px' }}>
          {record.output_price_display || '-'}
        </Text>
      ),
    },
    {
      title: t('输入金额'),
      dataIndex: 'input_amount_display',
      key: COLUMN_KEYS.INPUT_AMOUNT,
      width: 100,
      align: 'right',
      render: (text, record) => (
        <Text style={{ fontSize: '12px', color: 'var(--semi-color-success)' }}>
          {record.input_amount_display || '-'}
        </Text>
      ),
    },
    {
      title: t('输出金额'),
      dataIndex: 'output_amount_display',
      key: COLUMN_KEYS.OUTPUT_AMOUNT,
      width: 100,
      align: 'right',
      render: (text, record) => (
        <Text style={{ fontSize: '12px', color: 'var(--semi-color-warning)' }}>
          {record.output_amount_display || '-'}
        </Text>
      ),
    },
    {
      title: t('流式'),
      dataIndex: 'is_stream_display',
      key: COLUMN_KEYS.IS_STREAM,
      width: 60,
      align: 'center',
      render: (text, record) => (
        <Tag 
          color={record.is_stream ? 'green' : 'grey'} 
          size="small"
        >
          {text}
        </Tag>
      ),
    },
    {
      title: t('渠道ID'),
      dataIndex: 'channel_id',
      key: COLUMN_KEYS.CHANNEL_ID,
      width: 80,
      align: 'center',
      render: (text) => (
        <Text style={{ fontSize: '12px' }}>
          {text || '-'}
        </Text>
      ),
    },
    {
      title: t('TokenID'),
      dataIndex: 'token_id',
      key: COLUMN_KEYS.TOKEN_ID,
      width: 80,
      align: 'center',
      render: (text) => (
        <Text style={{ fontSize: '12px' }}>
          {text || '-'}
        </Text>
      ),
    },
    {
      title: t('IP'),
      dataIndex: 'ip',
      key: COLUMN_KEYS.IP,
      width: 120,
      render: (text) => (
        <div style={{ display: 'flex', alignItems: 'center', gap: '4px' }}>
          <Text style={{ fontSize: '12px' }}>
            {text || '-'}
          </Text>
          {text && (
            <Button
              theme="borderless"
              type="tertiary"
              size="small"
              icon={<IconCopy />}
              onClick={(e) => copyText(e, text)}
              style={{ minWidth: 'auto', padding: '2px' }}
            />
          )}
        </div>
      ),
    },
    {
      title: t('其他'),
      dataIndex: 'other',
      key: COLUMN_KEYS.OTHER,
      width: 150,
      render: (text) => (
        <div style={{ 
          maxWidth: '130px', 
          overflow: 'hidden', 
          textOverflow: 'ellipsis',
          whiteSpace: 'nowrap'
        }}>
          <Text 
            style={{ fontSize: '12px' }}
            ellipsis={{ showTooltip: true }}
          >
            {text || '-'}
          </Text>
        </div>
      ),
    },
  ];
};
