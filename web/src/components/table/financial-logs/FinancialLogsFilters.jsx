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
import { Button, Form, Tooltip, Divider, Typography } from '@douyinfe/semi-ui';
import { IconSearch, IconInfoCircle } from '@douyinfe/semi-icons';

const { Text } = Typography;

const FinancialLogsFilters = ({
  formInitValues,
  setFormApi,
  refresh,
  setShowColumnSelector,
  formApi,
  loading,
  tokenKey,
  setTokenKey,
  t,
}) => {
  return (
    <Form
      initValues={formInitValues}
      getFormApi={(api) => setFormApi(api)}
      onSubmit={refresh}
      allowEmpty={true}
      autoComplete='off'
      layout='vertical'
      trigger='change'
      stopValidateWithError={false}
    >
      <div className='flex flex-col gap-3'>
        {/* Token密钥输入 */}
        <div className='w-full'>
          <Form.Input
            field='key'
            label={
              <div className='flex items-center gap-1'>
                <span>{t('Token密钥')}</span>
                <Tooltip content={t('请输入要查询的Token密钥，例如：sk-xxx')}>
                  <IconInfoCircle size="small" />
                </Tooltip>
              </div>
            }
            placeholder={t('请输入Token密钥，例如：sk-LYjCRscuufA465EuJCADmweH7OnCnECg9ZzVJ8ZSYjsJ2Gru')}
            showClear
            pure
            size='small'
            value={tokenKey}
            onChange={(value) => setTokenKey(value)}
            required
          />
        </div>

        <div className='grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-2'>
          {/* 时间选择器 */}
          <div className='col-span-1 lg:col-span-2'>
            <Form.DatePicker
              field='dateRange'
              className='w-full'
              type='dateTimeRange'
              placeholder={[t('开始时间'), t('结束时间')]}
              showClear
              pure
              size='small'
            />
          </div>

          {/* 其他搜索字段 */}
          <Form.Input
            field='model_name'
            prefix={<IconSearch />}
            placeholder={t('模型名称（支持模糊匹配）')}
            showClear
            pure
            size='small'
          />

          <Form.Input
            field='group'
            prefix={<IconSearch />}
            placeholder={t('分组名称')}
            showClear
            pure
            size='small'
          />
        </div>

        <Divider margin='8px' />
        
        {/* 高级选项 */}
        <div className='space-y-3'>
          <Text type='secondary' size='small' className='block'>
            {t('高级选项')}
          </Text>
          
          <div className='grid grid-cols-1 md:grid-cols-1 gap-4'>
            <Form.Select
              field='type'
              label={t('日志类型')}
              placeholder={t('选择日志类型')}
              className='w-full'
              showClear
              pure
              size='small'
            >
              <Form.Select.Option value='0'>{t('全部类型')}</Form.Select.Option>
              <Form.Select.Option value='1'>{t('充值日志')}</Form.Select.Option>
              <Form.Select.Option value='2'>{t('消费日志')}</Form.Select.Option>
              <Form.Select.Option value='3'>{t('管理日志')}</Form.Select.Option>
              <Form.Select.Option value='4'>{t('系统日志')}</Form.Select.Option>
              <Form.Select.Option value='5'>{t('错误日志')}</Form.Select.Option>
            </Form.Select>
          </div>
        </div>

        {/* 操作按钮区域 */}
        <div className='flex flex-col sm:flex-row justify-between items-start sm:items-center gap-3'>
          <div className='flex gap-2 w-full sm:w-auto justify-start'>
            <Button
              type='primary'
              htmlType='submit'
              loading={loading}
              size='small'
            >
              {t('查询')}
            </Button>
            <Button
              type='tertiary'
              onClick={() => {
                if (formApi) {
                  formApi.reset();
                  setTokenKey('');
                  setTimeout(() => {
                    refresh();
                  }, 100);
                }
              }}
              size='small'
            >
              {t('重置')}
            </Button>
          </div>

          <div className='flex gap-2 w-full sm:w-auto justify-end'>
            <Button
              type='tertiary'
              onClick={() => setShowColumnSelector(true)}
              size='small'
            >
              {t('列设置')}
            </Button>
          </div>
        </div>
      </div>
    </Form>
  );
};

export default FinancialLogsFilters;
