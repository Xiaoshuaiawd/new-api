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
import { Typography, Switch } from '@douyinfe/semi-ui';

const { Paragraph } = Typography;

const SubscriptionPackagesDescription = ({
  compactMode,
  setCompactMode,
  t,
}) => {
  return (
    <div className='flex flex-col md:flex-row gap-4 md:items-center md:justify-between'>
      <div className='flex-1'>
        <Paragraph style={{ marginBottom: 0 }}>
          {t('管理系统中的订阅套餐，包括套餐配置、额度设置和用户订阅情况。')}
        </Paragraph>
      </div>
      <div className='flex items-center gap-2'>
        <span className='text-sm text-gray-600'>{t('紧凑模式')}</span>
        <Switch checked={compactMode} onChange={setCompactMode} size='small' />
      </div>
    </div>
  );
};

export default SubscriptionPackagesDescription;
