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

import { useState, useEffect, useContext } from 'react';
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
import { StatusContext } from '../../context/Status';

export const useFinancialLogsData = () => {
  const { t } = useTranslation();
  const [statusState] = useContext(StatusContext);

  // Define column keys for selection
  const COLUMN_KEYS = {
    ID: 'id',
    CREATED_AT: 'created_at',
    TYPE: 'type',
    TOKEN_NAME: 'token_name',
    MODEL_NAME: 'model_name',
    QUOTA: 'quota',
    PROMPT_TOKENS: 'prompt_tokens',
    COMPLETION_TOKENS: 'completion_tokens',
    INPUT_PRICE: 'input_price',
    OUTPUT_PRICE: 'output_price',
    INPUT_AMOUNT: 'input_amount',
    OUTPUT_AMOUNT: 'output_amount',
    IS_STREAM: 'is_stream',
    CHANNEL_ID: 'channel_id',
    TOKEN_ID: 'token_id',
    IP: 'ip',
    OTHER: 'other',
  };

  // Basic state
  const [logs, setLogs] = useState([]);
  const [loading, setLoading] = useState(false);
  const [downloadLoading, setDownloadLoading] = useState(false);
  
  // Pricing and ratio state
  const [groupRatio, setGroupRatio] = useState({});
  const [completionRatio, setCompletionRatio] = useState({});
  const [modelRatio, setModelRatio] = useState({});
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
    
    // 加载倍率配置
    loadPricingConfig();
  }, []);

  // Get default column visibility
  const getDefaultColumnVisibility = () => {
    return {
      [COLUMN_KEYS.ID]: true,
      [COLUMN_KEYS.CREATED_AT]: true,
      [COLUMN_KEYS.TYPE]: true,
      [COLUMN_KEYS.TOKEN_NAME]: true,
      [COLUMN_KEYS.MODEL_NAME]: true,
      [COLUMN_KEYS.QUOTA]: true,
      [COLUMN_KEYS.PROMPT_TOKENS]: true,
      [COLUMN_KEYS.COMPLETION_TOKENS]: true,
      [COLUMN_KEYS.INPUT_PRICE]: true,
      [COLUMN_KEYS.OUTPUT_PRICE]: true,
      [COLUMN_KEYS.INPUT_AMOUNT]: true,
      [COLUMN_KEYS.OUTPUT_AMOUNT]: true,
      [COLUMN_KEYS.IS_STREAM]: false,
      [COLUMN_KEYS.CHANNEL_ID]: false,
      [COLUMN_KEYS.TOKEN_ID]: false,
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

  // Load pricing configuration from API
  const loadPricingConfig = async () => {
    try {
      const res = await API.get('/api/pricing');
      const { success, data, group_ratio } = res.data;
      if (success) {
        setGroupRatio(group_ratio || {});
        // 可以根据需要添加其他倍率配置
        console.log('Loaded pricing config:', { group_ratio });
      }
    } catch (error) {
      console.error('Failed to load pricing config:', error);
    }
  };

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

  // Calculate price and amount based on tokens
  const calculatePriceAndAmount = (log) => {
    // 从系统状态获取基础配置
    const basePrice = 0.5; // 基础价格 $0.5 / 1M tokens
    const outputMultiplier = 2; // 输出倍率 - 这个可以后续从completionRatio中获取
    
    // 获取分组倍率
    const logGroup = log.group || 'default';
    const groupMultiplier = groupRatio[logGroup] || 1;
    
    // 计算输入价格和输出价格 (每1M tokens的价格)
    const inputPrice = basePrice; // $0.5 / 1M tokens
    const outputPrice = basePrice * outputMultiplier; // $0.5 * 2 = $1 / 1M tokens
    
    // 计算输入金额和输出金额
    const promptTokens = log.prompt_tokens || 0;
    const completionTokens = log.completion_tokens || 0;
    
    // (tokens / 1,000,000) * price per 1M tokens * group multiplier
    const inputAmount = (promptTokens / 1000000) * inputPrice * groupMultiplier;
    const outputAmount = (completionTokens / 1000000) * outputPrice * groupMultiplier;
    
    return {
      inputPrice,
      outputPrice,
      inputAmount,
      outputAmount
    };
  };

  // Format logs data
  const setLogsFormat = (logs) => {
    for (let i = 0; i < logs.length; i++) {
      logs[i].timestamp2string = timestamp2string(logs[i].created_at);
      logs[i].key = logs[i].id;
      
      // Format quota display
      logs[i].quota_display = renderQuota(logs[i].quota, 6);
      
      // Format tokens display - use normal integer format
      logs[i].prompt_tokens_display = logs[i].prompt_tokens || 0;
      logs[i].completion_tokens_display = logs[i].completion_tokens || 0;
      
      // Calculate and format price and amount
      const priceAmount = calculatePriceAndAmount(logs[i]);
      logs[i].input_price_display = `$${priceAmount.inputPrice.toFixed(3)} / 1M`;
      logs[i].output_price_display = `$${priceAmount.outputPrice.toFixed(3)} / 1M`;
      logs[i].input_amount_display = `$${priceAmount.inputAmount.toFixed(6)}`;
      logs[i].output_amount_display = `$${priceAmount.outputAmount.toFixed(6)}`;
      
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
      // 第一步：先获取总数，使用小页面大小
      let countUrl = `/api/log/token?key=${encodeURIComponent(key)}`;
      countUrl += `&page=1&page_size=1`;

      // Add filter parameters
      if (type > 0) {
        countUrl += `&type=${type}`;
      }
      if (model_name) {
        countUrl += `&model_name=${encodeURIComponent(model_name)}`;
      }
      if (group) {
        countUrl += `&group=${encodeURIComponent(group)}`;
      }

      // Add timestamp parameters
      let localStartTimestamp = Date.parse(start_timestamp) / 1000;
      let localEndTimestamp = Date.parse(end_timestamp) / 1000;
      countUrl += `&start_timestamp=${localStartTimestamp}&end_timestamp=${localEndTimestamp}`;

      const countRes = await API.get(countUrl);
      const { success: countSuccess, message: countMessage, data: countData } = countRes.data;
      
      if (!countSuccess) {
        showError(countMessage || t('获取数据总数失败'));
        return;
      }

      const totalCount = countRes.data.total || 0;
      
      if (totalCount === 0) {
        showError(t('没有数据可以下载'));
        return;
      }

      // 显示下载确认信息
      if (totalCount > 50000) {
        const confirmed = window.confirm(t('检测到大量数据（{{count}} 条），下载可能需要较长时间，是否继续？', { count: totalCount }));
        if (!confirmed) {
          return;
        }
      }

      // 第二步：根据数据量决定下载策略
      let allData = [];
      
      if (totalCount <= 100000) {
        // 小于等于10万条，一次性下载
        let downloadUrl = `/api/log/token?key=${encodeURIComponent(key)}`;
        downloadUrl += `&page=1&page_size=${totalCount}`;

        // Add filter parameters
        if (type > 0) {
          downloadUrl += `&type=${type}`;
        }
        if (model_name) {
          downloadUrl += `&model_name=${encodeURIComponent(model_name)}`;
        }
        if (group) {
          downloadUrl += `&group=${encodeURIComponent(group)}`;
        }
        downloadUrl += `&start_timestamp=${localStartTimestamp}&end_timestamp=${localEndTimestamp}`;

        const res = await API.get(downloadUrl);
        const { success, message, data } = res.data;
        
        if (success && data) {
          allData = data;
        } else {
          showError(message || t('下载数据失败'));
          return;
        }
      } else {
        // 大于10万条，分批下载
        const batchSize = 50000; // 每批5万条
        const totalPages = Math.ceil(totalCount / batchSize);
        
        showSuccess(t('数据量较大，将分 {{pages}} 批下载，请稍候...', { pages: totalPages }));
        
        for (let page = 1; page <= totalPages; page++) {
          let batchUrl = `/api/log/token?key=${encodeURIComponent(key)}`;
          batchUrl += `&page=${page}&page_size=${batchSize}`;

          // Add filter parameters
          if (type > 0) {
            batchUrl += `&type=${type}`;
          }
          if (model_name) {
            batchUrl += `&model_name=${encodeURIComponent(model_name)}`;
          }
          if (group) {
            batchUrl += `&group=${encodeURIComponent(group)}`;
          }
          batchUrl += `&start_timestamp=${localStartTimestamp}&end_timestamp=${localEndTimestamp}`;

          const batchRes = await API.get(batchUrl);
          const { success: batchSuccess, message: batchMessage, data: batchData } = batchRes.data;
          
          if (batchSuccess && batchData) {
            allData = allData.concat(batchData);
            // 显示进度
            const progress = Math.round((page / totalPages) * 100);
            console.log(`下载进度: ${progress}% (${page}/${totalPages})`);
          } else {
            showError(t('第 {{page}} 批数据下载失败: {{message}}', { page, message: batchMessage }));
            return;
          }
        }
      }

      if (allData.length > 0) {
        // 转换数据为Excel格式
        const excelData = allData.map((log, index) => {
          const priceAmount = calculatePriceAndAmount(log);
          return {
            '序号': index + 1,
            'ID': log.id,
            '时间': timestamp2string(log.created_at),
            '类型': getLogTypeText(log.type),
            'Token名称': log.token_name || '-',
            '模型': log.model_name || '-',
            '配额': renderQuota(log.quota, 6),
            '提示Token': (log.prompt_tokens || 0).toLocaleString(),
            '完成Token': (log.completion_tokens || 0).toLocaleString(),
            '输入价格': `$${priceAmount.inputPrice.toFixed(3)} / 1M`,
            '输出价格': `$${priceAmount.outputPrice.toFixed(3)} / 1M`,
            '输入金额': `$${priceAmount.inputAmount.toFixed(6)}`,
            '输出金额': `$${priceAmount.outputAmount.toFixed(6)}`,
            '流式': log.is_stream ? '是' : '否',
            '渠道ID': log.channel_id || '-',
            'TokenID': log.token_id || '-',
            'IP': log.ip || '-',
            '其他信息': log.other || '-',
          };
        });

        // 创建Excel文件并下载
        downloadExcel(excelData, `财务日志_${timestamp2string(Date.now() / 1000).replace(/[:\s]/g, '_')}.xlsx`);
        showSuccess(t('下载成功，共导出 {{count}} 条记录', { count: allData.length }));
      } else {
        showError(t('没有数据可以下载'));
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
      { wch: 20 }, // Token名称
      { wch: 15 }, // 模型
      { wch: 12 }, // 配额
      { wch: 12 }, // 提示Token
      { wch: 12 }, // 完成Token
      { wch: 18 }, // 输入价格
      { wch: 18 }, // 输出价格
      { wch: 15 }, // 输入金额
      { wch: 15 }, // 输出金额
      { wch: 8 },  // 流式
      { wch: 10 }, // 渠道ID
      { wch: 10 }, // TokenID
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
