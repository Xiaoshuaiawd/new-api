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
import { Modal, Checkbox, Button, Space, Divider } from '@douyinfe/semi-ui';

const ColumnSelectorModal = ({
  showColumnSelector,
  setShowColumnSelector,
  visibleColumns,
  handleColumnVisibilityChange,
  handleSelectAll,
  initDefaultColumns,
  COLUMN_KEYS,
  t,
}) => {
  // Column display names mapping
  const columnDisplayNames = {
    [COLUMN_KEYS.ID]: t('ID'),
    [COLUMN_KEYS.CREATED_AT]: t('时间'),
    [COLUMN_KEYS.TYPE]: t('类型'),
    [COLUMN_KEYS.TOKEN_NAME]: t('Token名称'),
    [COLUMN_KEYS.MODEL_NAME]: t('模型'),
    [COLUMN_KEYS.QUOTA]: t('配额'),
    [COLUMN_KEYS.PROMPT_TOKENS]: t('提示'),
    [COLUMN_KEYS.COMPLETION_TOKENS]: t('完成'),
    [COLUMN_KEYS.INPUT_PRICE]: t('输入价格'),
    [COLUMN_KEYS.OUTPUT_PRICE]: t('输出价格'),
    [COLUMN_KEYS.INPUT_AMOUNT]: t('输入金额'),
    [COLUMN_KEYS.OUTPUT_AMOUNT]: t('输出金额'),
    [COLUMN_KEYS.IS_STREAM]: t('流式'),
    [COLUMN_KEYS.CHANNEL_ID]: t('渠道ID'),
    [COLUMN_KEYS.TOKEN_ID]: t('TokenID'),
    [COLUMN_KEYS.IP]: t('IP'),
    [COLUMN_KEYS.OTHER]: t('其他'),
  };

  // Check if all columns are selected
  const allSelected = Object.values(visibleColumns).every((visible) => visible);
  const someSelected = Object.values(visibleColumns).some((visible) => visible);

  const handleSelectAllChange = (checked) => {
    handleSelectAll(checked);
  };

  return (
    <Modal
      title={t('选择显示列')}
      visible={showColumnSelector}
      onCancel={() => setShowColumnSelector(false)}
      onOk={() => setShowColumnSelector(false)}
      width={500}
      bodyStyle={{ maxHeight: '60vh', overflowY: 'auto' }}
    >
      <div className='space-y-4'>
        {/* Select All / None */}
        <div className='flex justify-between items-center'>
          <Checkbox
            checked={allSelected}
            indeterminate={!allSelected && someSelected}
            onChange={(e) => handleSelectAllChange(e.target.checked)}
          >
            {t('全选')}
          </Checkbox>
          <Button
            type='tertiary'
            size='small'
            onClick={initDefaultColumns}
          >
            {t('恢复默认')}
          </Button>
        </div>

        <Divider margin='12px' />

        {/* Column checkboxes */}
        <div className='grid grid-cols-2 gap-3'>
          {Object.entries(COLUMN_KEYS).map(([key, columnKey]) => (
            <Checkbox
              key={columnKey}
              checked={visibleColumns[columnKey] || false}
              onChange={(e) =>
                handleColumnVisibilityChange(columnKey, e.target.checked)
              }
            >
              {columnDisplayNames[columnKey] || key}
            </Checkbox>
          ))}
        </div>
      </div>
    </Modal>
  );
};

export default ColumnSelectorModal;
