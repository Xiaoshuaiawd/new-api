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
import { Modal } from '@douyinfe/semi-ui';
import {
  API,
  getTodayStartTimestamp,
  showError,
  showSuccess,
  timestamp2string,
  renderQuota,
  renderNumber,
  copy,
} from '../../helpers';
import { ITEMS_PER_PAGE } from '../../constants';
import { useTableCompactMode } from '../common/useTableCompactMode';

export const useFinancialLogsData = () => {
  const { t } = useTranslation();

  // Define column keys for selection
  const COLUMN_KEYS = {
    ID: 'id',
    CREATED_AT: 'created_at',
    TYPE: 'type',
    CONTENT: 'content',
    USERNAME: 'username',
    TOKEN_NAME: 'token_name',
    MODEL_NAME: 'model_name',
    QUOTA: 'quota',
    PROMPT_TOKENS: 'prompt_tokens',
    COMPLETION_TOKENS: 'completion_tokens',
    USE_TIME: 'use_time',
    IS_STREAM: 'is_stream',
    CHANNEL_ID: 'channel_id',
    CHANNEL_NAME: 'channel_name',
    TOKEN_ID: 'token_id',
    GROUP: 'group',
    IP: 'ip',
    OTHER: 'other',
  };

  // Basic state
  const [logs, setLogs] = useState([]);
  const [loading, setLoading] = useState(false);
  const [activePage, setActivePage] = useState(1);
  const [logCount, setLogCount] = useState(0);
  const [pageSize, setPageSize] = useState(ITEMS_PER_PAGE);

  // Form state
  const [formApi, setFormApi] = useState(null);
  const [tokenKey, setTokenKey] = useState('');
  let now = new Date();
  const formInitValues = {
    key: '',
    type: '0',
    model_name: '',
    group: '',
    dateRange: [
      timestamp2string(getTodayStartTimestamp()),
      timestamp2string(now.getTime() / 1000 + 3600),
    ],
    query_mode: 'normal',
  };

  // Column visibility state
  const [visibleColumns, setVisibleColumns] = useState({});
  const [showColumnSelector, setShowColumnSelector] = useState(false);

  // Compact mode
  const [compactMode, setCompactMode] = useTableCompactMode('financial-logs');

  // Cursor pagination state
  const [cursor, setCursor] = useState(null);
  const [hasMore, setHasMore] = useState(false);
  const [useCursor, setUseCursor] = useState(false);

  // Load saved column preferences from localStorage
  useEffect(() => {
    const savedColumns = localStorage.getItem('financial-logs-table-columns');
    if (savedColumns) {
      try {
        const parsed = JSON.parse(savedColumns);
        const defaults = getDefaultColumnVisibility();
        const merged = { ...defaults, ...parsed };
        setVisibleColumns(merged);
      } catch (e) {
        console.error('Failed to parse saved column preferences', e);
        initDefaultColumns();
      }
    } else {
      initDefaultColumns();
    }
  }, []);

  // Get default column visibility
  const getDefaultColumnVisibility = () => {
    return {
      [COLUMN_KEYS.ID]: true,
      [COLUMN_KEYS.CREATED_AT]: true,
      [COLUMN_KEYS.TYPE]: true,
      [COLUMN_KEYS.USERNAME]: true,
      [COLUMN_KEYS.TOKEN_NAME]: true,
      [COLUMN_KEYS.MODEL_NAME]: true,
      [COLUMN_KEYS.QUOTA]: true,
      [COLUMN_KEYS.PROMPT_TOKENS]: true,
      [COLUMN_KEYS.COMPLETION_TOKENS]: true,
      [COLUMN_KEYS.USE_TIME]: true,
      [COLUMN_KEYS.IS_STREAM]: false,
      [COLUMN_KEYS.CHANNEL_ID]: false,
      [COLUMN_KEYS.CHANNEL_NAME]: true,
      [COLUMN_KEYS.TOKEN_ID]: false,
      [COLUMN_KEYS.GROUP]: true,
      [COLUMN_KEYS.IP]: false,
      [COLUMN_KEYS.OTHER]: false,
    };
  };

  // Initialize default column visibility
  const initDefaultColumns = () => {
    const defaults = getDefaultColumnVisibility();
    setVisibleColumns(defaults);
    localStorage.setItem('financial-logs-table-columns', JSON.stringify(defaults));
  };

  // Handle column visibility change
  const handleColumnVisibilityChange = (columnKey, checked) => {
    const updatedColumns = { ...visibleColumns, [columnKey]: checked };
    setVisibleColumns(updatedColumns);
  };

  // Handle "Select All" checkbox
  const handleSelectAll = (checked) => {
    const allKeys = Object.keys(COLUMN_KEYS).map((key) => COLUMN_KEYS[key]);
    const updatedColumns = {};

    allKeys.forEach((key) => {
      updatedColumns[key] = checked;
    });

    setVisibleColumns(updatedColumns);
  };

  // Persist column settings
  useEffect(() => {
    if (Object.keys(visibleColumns).length > 0) {
      localStorage.setItem('financial-logs-table-columns', JSON.stringify(visibleColumns));
    }
  }, [visibleColumns]);

  // 获取表单值的辅助函数
  const getFormValues = () => {
    const formValues = formApi ? formApi.getValues() : {};

    let start_timestamp = timestamp2string(getTodayStartTimestamp());
    let end_timestamp = timestamp2string(now.getTime() / 1000 + 3600);

    if (
      formValues.dateRange &&
      Array.isArray(formValues.dateRange) &&
      formValues.dateRange.length === 2
    ) {
      start_timestamp = formValues.dateRange[0];
      end_timestamp = formValues.dateRange[1];
    }

    return {
      key: tokenKey || formValues.key || '',
      type: formValues.type ? parseInt(formValues.type) : 0,
      model_name: formValues.model_name || '',
      group: formValues.group || '',
      start_timestamp,
      end_timestamp,
      use_cursor: useCursor,
    };
  };

  // Format logs data
  const setLogsFormat = (logs) => {
    for (let i = 0; i < logs.length; i++) {
      logs[i].timestamp2string = timestamp2string(logs[i].created_at);
      logs[i].key = logs[i].id;
      
      // Format quota display
      logs[i].quota_display = renderQuota(logs[i].quota, 6);
      
      // Format tokens display
      logs[i].prompt_tokens_display = renderNumber(logs[i].prompt_tokens);
      logs[i].completion_tokens_display = renderNumber(logs[i].completion_tokens);
      
      // Format use time
      logs[i].use_time_display = logs[i].use_time ? `${logs[i].use_time}s` : '-';
      
      // Format stream status
      logs[i].is_stream_display = logs[i].is_stream ? t('是') : t('否');
      
      // Format type
      const typeMap = {
        0: t('全部'),
        1: t('充值'),
        2: t('消费'),
        3: t('管理'),
        4: t('系统'),
        5: t('错误'),
      };
      logs[i].type_display = typeMap[logs[i].type] || t('未知');
    }

    setLogs(logs);
  };

  // Load logs function using the /api/log/token endpoint
  const loadLogs = async (page = 1, size = pageSize, resetCursor = false) => {
    setLoading(true);

    const {
      key,
      type,
      model_name,
      group,
      start_timestamp,
      end_timestamp,
      use_cursor,
    } = getFormValues();

    if (!key || key.trim() === '') {
      showError(t('请输入Token密钥'));
      setLoading(false);
      return;
    }

    try {
      let url = `/api/log/token?key=${encodeURIComponent(key)}`;
      
      // Add pagination parameters
      if (use_cursor) {
        url += `&use_cursor=true&page_size=${size}`;
        if (cursor && !resetCursor) {
          url += `&cursor=${encodeURIComponent(cursor)}`;
        }
      } else {
        url += `&page=${page}&page_size=${size}`;
      }

      // Add filter parameters
      if (type > 0) {
        url += `&type=${type}`;
      }
      if (model_name) {
        url += `&model_name=${encodeURIComponent(model_name)}`;
      }
      if (group) {
        url += `&group=${encodeURIComponent(group)}`;
      }

      // Add timestamp parameters
      let localStartTimestamp = Date.parse(start_timestamp) / 1000;
      let localEndTimestamp = Date.parse(end_timestamp) / 1000;
      url += `&start_timestamp=${localStartTimestamp}&end_timestamp=${localEndTimestamp}`;

      const res = await API.get(url);
      const { success, message, data } = res.data;
      
      if (success) {
        if (use_cursor) {
          // Handle cursor pagination response
          setCursor(res.data.next_cursor);
          setHasMore(res.data.has_more);
          setActivePage(1); // Cursor pagination doesn't use page numbers
          setLogCount(0); // No total count in cursor mode for performance
        } else {
          // Handle regular pagination response
          setActivePage(res.data.page || page);
          setPageSize(res.data.page_size || size);
          setLogCount(res.data.total || 0);
        }

        setLogsFormat(data);
      } else {
        showError(message);
        setLogs([]);
        setLogCount(0);
      }
    } catch (error) {
      console.error('Load logs error:', error);
      showError(t('加载日志失败，请重试'));
      setLogs([]);
      setLogCount(0);
    }
    setLoading(false);
  };

  // Page handlers
  const handlePageChange = (page) => {
    setActivePage(page);
    loadLogs(page, pageSize);
  };

  const handlePageSizeChange = async (size) => {
    localStorage.setItem('page-size', size + '');
    setPageSize(size);
    setActivePage(1);
    setCursor(null); // Reset cursor when changing page size
    loadLogs(1, size, true);
  };

  // Load next page for cursor pagination
  const loadNextPage = () => {
    if (hasMore && !loading) {
      loadLogs(1, pageSize, false);
    }
  };

  // Refresh function
  const refresh = async () => {
    setActivePage(1);
    setCursor(null);
    await loadLogs(1, pageSize, true);
  };

  // Copy text function
  const copyText = async (e, text) => {
    e.stopPropagation();
    if (await copy(text)) {
      showSuccess('已复制：' + text);
    } else {
      Modal.error({ title: t('无法复制到剪贴板，请手动复制'), content: text });
    }
  };

  // Handle cursor pagination mode change
  const handleCursorModeChange = (checked) => {
    setUseCursor(checked);
    setCursor(null);
    setActivePage(1);
  };

  return {
    // Basic state
    logs,
    loading,
    activePage,
    logCount,
    pageSize,
    tokenKey,
    setTokenKey,

    // Cursor pagination
    cursor,
    hasMore,
    useCursor,
    setUseCursor,
    handleCursorModeChange,
    loadNextPage,

    // Form state
    formApi,
    setFormApi,
    formInitValues,
    getFormValues,

    // Column visibility
    visibleColumns,
    showColumnSelector,
    setShowColumnSelector,
    handleColumnVisibilityChange,
    handleSelectAll,
    initDefaultColumns,
    COLUMN_KEYS,

    // Compact mode
    compactMode,
    setCompactMode,

    // Functions
    loadLogs,
    handlePageChange,
    handlePageSizeChange,
    refresh,
    copyText,
    setLogsFormat,

    // Translation
    t,
  };
};
