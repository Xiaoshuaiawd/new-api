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
import * as XLSX from 'xlsx';
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
  const [downloadLoading, setDownloadLoading] = useState(false);
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
  };

  // Column visibility state
  const [visibleColumns, setVisibleColumns] = useState({});
  const [showColumnSelector, setShowColumnSelector] = useState(false);

  // Compact mode
  const [compactMode, setCompactMode] = useTableCompactMode('financial-logs');


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
  const loadLogs = async (page = 1, size = pageSize) => {
    setLoading(true);

    const {
      key,
      type,
      model_name,
      group,
      start_timestamp,
      end_timestamp,
    } = getFormValues();

    if (!key || key.trim() === '') {
      showError(t('请输入Token密钥'));
      setLoading(false);
      return;
    }

    try {
      let url = `/api/log/token?key=${encodeURIComponent(key)}`;
      
      // Add pagination parameters
      url += `&page=${page}&page_size=${size}`;

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
        // Handle regular pagination response
        setActivePage(res.data.page || page);
        setPageSize(res.data.page_size || size);
        setLogCount(res.data.total || 0);

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
    loadLogs(1, size);
  };

  // Refresh function
  const refresh = async () => {
    setActivePage(1);
    await loadLogs(1, pageSize);
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

  // Download logs function
  const downloadLogs = async () => {
    const {
      key,
      type,
      model_name,
      group,
      start_timestamp,
      end_timestamp,
    } = getFormValues();

    if (!key || key.trim() === '') {
      showError(t('请输入Token密钥'));
      return;
    }

    setDownloadLoading(true);

    try {
      // 构建下载URL，使用大页面大小获取所有数据
      let url = `/api/log/token?key=${encodeURIComponent(key)}`;
      
      // 使用大页面大小来获取所有数据
      url += `&page=1&page_size=100000`;

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
      
      if (success && data && data.length > 0) {
        // 转换数据为Excel格式
        const excelData = data.map((log, index) => ({
          '序号': index + 1,
          'ID': log.id,
          '时间': timestamp2string(log.created_at),
          '类型': getLogTypeText(log.type),
          '用户名': log.username || '-',
          'Token名称': log.token_name || '-',
          '模型': log.model_name || '-',
          '配额': renderQuota(log.quota, 6),
          '提示Token': renderNumber(log.prompt_tokens),
          '完成Token': renderNumber(log.completion_tokens),
          '耗时(秒)': log.use_time || 0,
          '流式': log.is_stream ? '是' : '否',
          '渠道ID': log.channel_id || '-',
          '渠道名称': log.channel_name || '-',
          'TokenID': log.token_id || '-',
          '分组': log.group || 'default',
          'IP': log.ip || '-',
          '其他信息': log.other || '-',
        }));

        // 创建Excel文件并下载
        downloadExcel(excelData, `财务日志_${timestamp2string(Date.now() / 1000).replace(/[:\s]/g, '_')}.xlsx`);
        showSuccess(t('下载成功，共导出 {{count}} 条记录', { count: data.length }));
      } else {
        showError(message || t('没有数据可以下载'));
      }
    } catch (error) {
      console.error('Download logs error:', error);
      showError(t('下载失败，请重试'));
    } finally {
      setDownloadLoading(false);
    }
  };

  // Helper function to get log type text
  const getLogTypeText = (type) => {
    const typeMap = {
      0: t('全部'),
      1: t('充值'),
      2: t('消费'),
      3: t('管理'),
      4: t('系统'),
      5: t('错误'),
    };
    return typeMap[type] || t('未知');
  };

  // Helper function to download Excel
  const downloadExcel = (data, filename) => {
    // 创建工作表
    const ws = XLSX.utils.json_to_sheet(data);
    
    // 创建工作簿
    const wb = XLSX.utils.book_new();
    XLSX.utils.book_append_sheet(wb, ws, '财务日志');
    
    // 设置列宽
    const colWidths = [
      { wch: 6 },  // 序号
      { wch: 10 }, // ID
      { wch: 20 }, // 时间
      { wch: 8 },  // 类型
      { wch: 15 }, // 用户名
      { wch: 20 }, // Token名称
      { wch: 15 }, // 模型
      { wch: 12 }, // 配额
      { wch: 10 }, // 提示Token
      { wch: 10 }, // 完成Token
      { wch: 10 }, // 耗时
      { wch: 8 },  // 流式
      { wch: 10 }, // 渠道ID
      { wch: 15 }, // 渠道名称
      { wch: 10 }, // TokenID
      { wch: 10 }, // 分组
      { wch: 15 }, // IP
      { wch: 20 }, // 其他信息
    ];
    ws['!cols'] = colWidths;
    
    // 下载文件
    XLSX.writeFile(wb, filename);
  };

  return {
    // Basic state
    logs,
    loading,
    downloadLoading,
    activePage,
    logCount,
    pageSize,
    tokenKey,
    setTokenKey,

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
    downloadLogs,
    setLogsFormat,

    // Translation
    t,
  };
};
