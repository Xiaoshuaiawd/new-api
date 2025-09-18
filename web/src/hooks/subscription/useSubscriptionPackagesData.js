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

import { useState, useEffect } from 'react';
import { useTranslation } from 'react-i18next';
import { Toast } from '@douyinfe/semi-ui';
import { API } from '../../helpers';

export const useSubscriptionPackagesData = () => {
  const { t } = useTranslation();

  // State management
  const [packages, setPackages] = useState([]);
  const [packageCount, setPackageCount] = useState(0);
  const [loading, setLoading] = useState(false);
  const [searching, setSearching] = useState(false);
  const [activePage, setActivePage] = useState(1);
  const [pageSize, setPageSize] = useState(20);
  const [compactMode, setCompactMode] = useState(false);

  // Modal state
  const [showAddPackage, setShowAddPackage] = useState(false);
  const [showEditPackage, setShowEditPackage] = useState(false);
  const [editingPackage, setEditingPackage] = useState(null);

  // Form state
  const [formApi, setFormApi] = useState(null);
  const [formInitValues] = useState({
    status: '',
  });

  // Load packages
  const loadPackages = async (page = 1, size = 20, status = '') => {
    setLoading(true);
    try {
      const params = new URLSearchParams();
      if (status !== '') params.append('status', status);

      const queryString = params.toString();
      const url = queryString ? `/api/subscription/packages?${queryString}` : '/api/subscription/packages';

      const response = await API.get(url);

      if (response.data.success) {
        setPackages(response.data.data || []);
        setPackageCount(response.data.data?.length || 0);
        setActivePage(page);
        setPageSize(size);
      } else {
        Toast.error(response.data.message || '获取套餐列表失败');
      }
    } catch (error) {
      Toast.error('网络错误，请稍后重试');
      console.error('Load packages error:', error);
    } finally {
      setLoading(false);
    }
  };

  // Search packages
  const searchPackages = async (values) => {
    setSearching(true);
    try {
      const params = new URLSearchParams();
      if (values.status) params.append('status', values.status);

      const queryString = params.toString();
      const url = queryString ? `/api/subscription/packages?${queryString}` : '/api/subscription/packages';

      const response = await API.get(url);

      if (response.data.success) {
        setPackages(response.data.data || []);
        setPackageCount(response.data.data?.length || 0);
        setActivePage(1);
      } else {
        Toast.error(response.data.message || '搜索失败');
      }
    } catch (error) {
      Toast.error('网络错误，请稍后重试');
      console.error('Search packages error:', error);
    } finally {
      setSearching(false);
    }
  };

  // Delete package
  const handleDeletePackage = async (packageId) => {
    try {
      const response = await API.delete(
        `/api/subscription/packages/${packageId}`,
      );

      if (response.data.success) {
        Toast.success('套餐删除成功');
        loadPackages(activePage, pageSize);
      } else {
        Toast.error(response.data.message || '删除失败');
      }
    } catch (error) {
      Toast.error('网络错误，请稍后重试');
      console.error('Delete package error:', error);
    }
  };

  // Modal handlers
  const closeAddPackage = () => {
    setShowAddPackage(false);
  };

  const closeEditPackage = () => {
    setShowEditPackage(false);
    setEditingPackage(null);
  };

  // Pagination handlers
  const handlePageChange = (page) => {
    setActivePage(page);
    loadPackages(page, pageSize);
  };

  const handlePageSizeChange = (size) => {
    setPageSize(size);
    setActivePage(1);
    loadPackages(1, size);
  };

  // Refresh data
  const refresh = () => {
    loadPackages(activePage, pageSize);
  };

  // Load initial data
  useEffect(() => {
    loadPackages();
  }, []);

  return {
    // Data
    packages,
    packageCount,
    loading,
    searching,
    activePage,
    pageSize,
    compactMode,
    setCompactMode,

    // Modal state
    showAddPackage,
    showEditPackage,
    editingPackage,
    setShowAddPackage,
    setEditingPackage,
    setShowEditPackage,
    closeAddPackage,
    closeEditPackage,

    // Form state
    formInitValues,
    setFormApi,

    // Functions
    loadPackages,
    searchPackages,
    handleDeletePackage,
    handlePageChange,
    handlePageSizeChange,
    refresh,

    // Translation
    t,
  };
};
