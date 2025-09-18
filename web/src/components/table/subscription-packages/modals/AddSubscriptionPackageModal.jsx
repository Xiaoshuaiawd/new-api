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

import React, { useState } from 'react';
import {
  Modal,
  Form,
  Input,
  InputNumber,
  Select,
  Switch,
  Toast,
  Typography,
} from '@douyinfe/semi-ui';
import { API } from '../../../../helpers';

const { TextArea } = Input;
const { Text } = Typography;

const AddSubscriptionPackageModal = ({ visible, handleClose, refresh }) => {
  const [loading, setLoading] = useState(false);

  const handleSubmit = async (values) => {
    try {
      setLoading(true);
      const payload = {
        ...values,
        status: values.status ? 1 : 0,
      };
      const response = await API.post('/api/subscription/packages', payload);

      if (response.data.success) {
        Toast.success('套餐创建成功');
        refresh();
        handleClose();
      } else {
        Toast.error(response.data.message || '创建失败');
      }
    } catch (error) {
      Toast.error('网络错误，请稍后重试');
      console.error('Create package error:', error);
    } finally {
      setLoading(false);
    }
  };

  return (
    <Modal
      title='新建订阅套餐'
      visible={visible}
      onCancel={handleClose}
      width={600}
      footer={null}
      centered
    >
      <Form
        onSubmit={handleSubmit}
        labelPosition='left'
        labelWidth={120}
        style={{ padding: '20px 0' }}
      >
        <Form.Input
          field='name'
          label='套餐名称'
          placeholder='请输入套餐名称'
          rules={[{ required: true, message: '套餐名称不能为空' }]}
        />

        <Form.TextArea
          field='description'
          label='套餐描述'
          placeholder='请输入套餐描述'
          maxCount={500}
          showClear
        />

        <div style={{ marginBottom: 24 }}>
          <Text strong>额度设置</Text>
          <Text
            type='secondary'
            size='small'
            style={{ display: 'block', marginTop: 4 }}
          >
            至少需要设置一种额度类型
          </Text>
        </div>

        <Form.InputNumber
          field='permanent_quota'
          label='永久额度'
          placeholder='设置永久额度'
          min={0}
          step={1000}
          style={{ width: '100%' }}
          suffix='tokens'
        />

        <Form.InputNumber
          field='monthly_quota'
          label='每月额度'
          placeholder='设置每月额度'
          min={0}
          step={1000}
          style={{ width: '100%' }}
          suffix='tokens'
        />

        <Form.InputNumber
          field='daily_quota'
          label='每日额度'
          placeholder='设置每日额度'
          min={0}
          step={100}
          style={{ width: '100%' }}
          suffix='tokens'
        />

        <div style={{ marginBottom: 24 }}>
          <Text strong>价格设置</Text>
        </div>

        <Form.InputNumber
          field='price'
          label='套餐价格'
          placeholder='设置套餐价格'
          min={0}
          precision={2}
          style={{ width: '100%' }}
          rules={[{ required: true, message: '套餐价格不能为空' }]}
        />

        <Form.Select
          field='currency'
          label='货币类型'
          placeholder='选择货币类型'
          defaultValue='CNY'
          optionList={[
            { label: 'CNY (人民币)', value: 'CNY' },
            { label: 'USD (美元)', value: 'USD' },
            { label: 'EUR (欧元)', value: 'EUR' },
          ]}
        />

        <Form.InputNumber
          field='duration'
          label='有效期'
          placeholder='设置有效期（天）'
          min={1}
          defaultValue={30}
          style={{ width: '100%' }}
          suffix='天'
          rules={[{ required: true, message: '有效期不能为空' }]}
        />

        <Form.Switch field='status' label='启用状态' defaultValue={true} />

        <Form.InputNumber
          field='sort_order'
          label='排序'
          placeholder='设置排序（数字越小越靠前）'
          min={0}
          defaultValue={0}
          style={{ width: '100%' }}
        />

        <div className='flex justify-end gap-3 mt-6'>
          <button
            type='button'
            onClick={handleClose}
            className='px-4 py-2 text-gray-600 hover:text-gray-800'
          >
            取消
          </button>
          <button
            type='submit'
            disabled={loading}
            className='px-4 py-2 bg-blue-500 text-white rounded hover:bg-blue-600 disabled:opacity-50'
          >
            {loading ? '创建中...' : '创建套餐'}
          </button>
        </div>
      </Form>
    </Modal>
  );
};

export default AddSubscriptionPackageModal;
