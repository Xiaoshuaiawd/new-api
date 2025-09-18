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
import { Form, Button, Select, Space } from '@douyinfe/semi-ui';
import { Search, RotateCcw } from 'lucide-react';

const SubscriptionPackagesFilters = ({
  formInitValues,
  setFormApi,
  searchPackages,
  loadPackages,
  activePage,
  pageSize,
  loading,
  searching,
  t,
}) => {
  const handleSearch = (values) => {
    searchPackages(values);
  };

  const handleReset = () => {
    loadPackages(activePage, pageSize);
  };

  return (
    <Form
      layout='horizontal'
      onSubmit={handleSearch}
      getFormApi={setFormApi}
      initValues={formInitValues}
      className='flex-1 md:flex-none'
    >
      <Space wrap className='w-full md:w-auto justify-end'>
        <Form.Select
          field='status'
          placeholder={t('按状态筛选')}
          style={{ width: 120 }}
          optionList={[
            { label: t('全部'), value: '' },
            { label: t('启用'), value: '1' },
            { label: t('禁用'), value: '0' },
          ]}
        />

        <Button
          htmlType='submit'
          type='primary'
          theme='solid'
          loading={searching}
          icon={<Search size={16} />}
        >
          {t('搜索')}
        </Button>

        <Button
          onClick={handleReset}
          type='tertiary'
          loading={loading}
          icon={<RotateCcw size={16} />}
        >
          {t('重置')}
        </Button>
      </Space>
    </Form>
  );
};

export default SubscriptionPackagesFilters;
