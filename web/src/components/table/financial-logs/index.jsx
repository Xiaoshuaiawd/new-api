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
import CardPro from '../../common/ui/CardPro';
import FinancialLogsTable from './FinancialLogsTable';
import FinancialLogsActions from './FinancialLogsActions';
import FinancialLogsFilters from './FinancialLogsFilters';
import ColumnSelectorModal from './modals/ColumnSelectorModal';
import { useFinancialLogsData } from '../../../hooks/financial-logs/useFinancialLogsData';
import { useIsMobile } from '../../../hooks/common/useIsMobile';
import { createCardProPagination } from '../../../helpers/utils';

const FinancialLogsPage = () => {
  const logsData = useFinancialLogsData();
  const isMobile = useIsMobile();

  return (
    <>
      {/* Modals */}
      <ColumnSelectorModal {...logsData} />

      {/* Main Content */}
      <CardPro
        type='type2'
        statsArea={<FinancialLogsActions {...logsData} />}
        searchArea={<FinancialLogsFilters {...logsData} />}
        paginationArea={null}
        t={logsData.t}
      >
        <FinancialLogsTable {...logsData} />
      </CardPro>
    </>
  );
};

export default FinancialLogsPage;
