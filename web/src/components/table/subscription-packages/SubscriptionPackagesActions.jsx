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
import { Button } from '@douyinfe/semi-ui';
import { Plus } from 'lucide-react';

const SubscriptionPackagesActions = ({ setShowAddPackage, t }) => {
  return (
    <div className='flex gap-2'>
      <Button
        onClick={() => setShowAddPackage(true)}
        theme='solid'
        type='primary'
        icon={<Plus size={16} />}
      >
        {t('新建套餐')}
      </Button>
    </div>
  );
};

export default SubscriptionPackagesActions;
