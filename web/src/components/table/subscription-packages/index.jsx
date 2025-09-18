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
import SubscriptionPackagesTable from './SubscriptionPackagesTable';
import SubscriptionPackagesActions from './SubscriptionPackagesActions';
import SubscriptionPackagesFilters from './SubscriptionPackagesFilters';
import SubscriptionPackagesDescription from './SubscriptionPackagesDescription';
import AddSubscriptionPackageModal from './modals/AddSubscriptionPackageModal';
import EditSubscriptionPackageModal from './modals/EditSubscriptionPackageModal';
import { useSubscriptionPackagesData } from '../../../hooks/subscription/useSubscriptionPackagesData';
import { useIsMobile } from '../../../hooks/common/useIsMobile';
import { createCardProPagination } from '../../../helpers/utils';

const SubscriptionPackagesPage = () => {
  const subscriptionData = useSubscriptionPackagesData();
  const isMobile = useIsMobile();

  const {
    // Modal state
    showAddPackage,
    showEditPackage,
    editingPackage,
    setShowAddPackage,
    closeAddPackage,
    closeEditPackage,
    refresh,

    // Form state
    formInitValues,
    setFormApi,
    searchPackages,
    loadPackages,
    activePage,
    pageSize,
    loading,
    searching,

    // Description state
    compactMode,
    setCompactMode,

    // Translation
    t,
  } = subscriptionData;

  return (
    <>
      <AddSubscriptionPackageModal
        refresh={refresh}
        visible={showAddPackage}
        handleClose={closeAddPackage}
      />

      <EditSubscriptionPackageModal
        refresh={refresh}
        visible={showEditPackage}
        handleClose={closeEditPackage}
        editingPackage={editingPackage}
      />

      <CardPro
        type='type1'
        descriptionArea={
          <SubscriptionPackagesDescription
            compactMode={compactMode}
            setCompactMode={setCompactMode}
            t={t}
          />
        }
        actionsArea={
          <div className='flex flex-col md:flex-row justify-between items-center gap-2 w-full'>
            <SubscriptionPackagesActions
              setShowAddPackage={setShowAddPackage}
              t={t}
            />

            <SubscriptionPackagesFilters
              formInitValues={formInitValues}
              setFormApi={setFormApi}
              searchPackages={searchPackages}
              loadPackages={loadPackages}
              activePage={activePage}
              pageSize={pageSize}
              loading={loading}
              searching={searching}
              t={t}
            />
          </div>
        }
        paginationArea={createCardProPagination({
          currentPage: subscriptionData.activePage,
          pageSize: subscriptionData.pageSize,
          total: subscriptionData.packageCount,
          onPageChange: subscriptionData.handlePageChange,
          onPageSizeChange: subscriptionData.handlePageSizeChange,
          isMobile: isMobile,
          t: subscriptionData.t,
        })}
        t={subscriptionData.t}
      >
        <SubscriptionPackagesTable {...subscriptionData} />
      </CardPro>
    </>
  );
};

export default SubscriptionPackagesPage;
