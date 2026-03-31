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
import {
  Button,
  Dropdown,
  Space,
  SplitButtonGroup,
  Tag,
  AvatarGroup,
  Avatar,
  Tooltip,
  Progress,
  Popover,
  Typography,
  Input,
  Modal,
  Select,
} from '@douyinfe/semi-ui';
import {
  timestamp2string,
  renderGroup,
  renderQuota,
  getModelCategories,
  showError,
  isAdmin,
} from '../../../helpers';
import {
  formatSubscriptionDuration,
  formatSubscriptionResetPeriod,
} from '../../../helpers/subscriptionFormat';
import {
  IconTreeTriangleDown,
  IconCopy,
  IconEyeOpened,
  IconEyeClosed,
} from '@douyinfe/semi-icons';

// progress color helper
const getProgressColor = (pct) => {
  if (pct === 100) return 'var(--semi-color-success)';
  if (pct <= 10) return 'var(--semi-color-danger)';
  if (pct <= 30) return 'var(--semi-color-warning)';
  return undefined;
};

// Render functions
function renderTimestamp(timestamp) {
  return <>{timestamp2string(timestamp)}</>;
}

const getEffectiveTokenStatus = (record) => {
  const now = Math.floor(Date.now() / 1000);
  if (record?.plan_id > 0 && record?.activation_time === 0 && record?.status === 1) {
    return 'pending_activation';
  }
  if (record?.expired_time > 0 && record?.expired_time <= now) {
    return 3;
  }
  return record?.status;
};

// Render status column only (no usage)
const renderStatus = (text, record, t) => {
  const effectiveStatus = getEffectiveTokenStatus(record);

  if (effectiveStatus === 'pending_activation') {
    return (
      <Tag color='cyan' shape='circle' size='small'>
        {t('未激活')}
      </Tag>
    );
  }

  const enabled = effectiveStatus === 1;

  let tagColor = 'black';
  let tagText = t('未知状态');
  if (enabled) {
    tagColor = 'green';
    tagText = t('已启用');
  } else if (effectiveStatus === 2) {
    tagColor = 'red';
    tagText = t('已禁用');
  } else if (effectiveStatus === 3) {
    tagColor = 'yellow';
    tagText = t('已过期');
  } else if (effectiveStatus === 4) {
    tagColor = 'grey';
    tagText = t('已耗尽');
  }

  return (
    <Tag color={tagColor} shape='circle' size='small'>
      {tagText}
    </Tag>
  );
};

const renderTokenType = (record, t) => {
  if (record.plan_id > 0) {
    return (
      <Tag color='blue' shape='circle' size='small'>
        {t('订阅型')}
      </Tag>
    );
  }
  return (
    <Tag color='grey' shape='circle' size='small'>
      {t('普通')}
    </Tag>
  );
};

const renderPlanTitle = (record, t) => {
  if (record.plan_id <= 0) {
    return <span>{t('无')}</span>;
  }
  const planTitle = record.plan_title || `#${record.plan_id}`;
  const queuedCount = Number(record.renewal_queued_count || 0);
  if (!record.next_renewal_plan_title) {
    return <span>{planTitle}</span>;
  }
  const queuedLabel =
    queuedCount > 1 ? `${t('待续费')} +${queuedCount}` : t('待续费');
  const queuedContent =
    queuedCount > 1
      ? `${t('已排队')} ${queuedCount} ${t('个套餐')}，${t('下一个续费套餐')}：${record.next_renewal_plan_title}`
      : `${t('下一个续费套餐')}：${record.next_renewal_plan_title}`;
  return (
    <Space wrap>
      <span>{planTitle}</span>
      <Tooltip content={queuedContent} position='top'>
        <Tag color='orange' shape='circle' size='small'>
          {queuedLabel}
        </Tag>
      </Tooltip>
    </Space>
  );
};

// Render group column
const renderGroupColumn = (text, record, t) => {
  if (text === 'auto') {
    return (
      <Tooltip
        content={t(
          '当前分组为 auto，会自动选择最优分组，当一个组不可用时自动降级到下一个组（熔断机制）',
        )}
        position='top'
      >
        <Tag color='white' shape='circle'>
          {t('智能熔断')}
          {record && record.cross_group_retry ? `(${t('跨分组')})` : ''}
        </Tag>
      </Tooltip>
    );
  }
  return renderGroup(text);
};

// Render token key column with show/hide and copy functionality
const renderTokenKey = (text, record, showKeys, setShowKeys, copyText) => {
  const fullKey = 'sk-' + record.key;
  const maskedKey =
    'sk-' + record.key.slice(0, 4) + '**********' + record.key.slice(-4);
  const revealed = !!showKeys[record.id];

  return (
    <div className='w-[200px]'>
      <Input
        readOnly
        value={revealed ? fullKey : maskedKey}
        size='small'
        suffix={
          <div className='flex items-center'>
            <Button
              theme='borderless'
              size='small'
              type='tertiary'
              icon={revealed ? <IconEyeClosed /> : <IconEyeOpened />}
              aria-label='toggle token visibility'
              onClick={(e) => {
                e.stopPropagation();
                setShowKeys((prev) => ({ ...prev, [record.id]: !revealed }));
              }}
            />
            <Button
              theme='borderless'
              size='small'
              type='tertiary'
              icon={<IconCopy />}
              aria-label='copy token key'
              onClick={async (e) => {
                e.stopPropagation();
                await copyText(fullKey);
              }}
            />
          </div>
        }
      />
    </div>
  );
};

// Render model limits column
const renderModelLimits = (text, record, t) => {
  if (record.model_limits_enabled && text) {
    const models = text.split(',').filter(Boolean);
    const categories = getModelCategories(t);

    const vendorAvatars = [];
    const matchedModels = new Set();
    Object.entries(categories).forEach(([key, category]) => {
      if (key === 'all') return;
      if (!category.icon || !category.filter) return;
      const vendorModels = models.filter((m) =>
        category.filter({ model_name: m }),
      );
      if (vendorModels.length > 0) {
        vendorAvatars.push(
          <Tooltip
            key={key}
            content={vendorModels.join(', ')}
            position='top'
            showArrow
          >
            <Avatar
              size='extra-extra-small'
              alt={category.label}
              color='transparent'
            >
              {category.icon}
            </Avatar>
          </Tooltip>,
        );
        vendorModels.forEach((m) => matchedModels.add(m));
      }
    });

    const unmatchedModels = models.filter((m) => !matchedModels.has(m));
    if (unmatchedModels.length > 0) {
      vendorAvatars.push(
        <Tooltip
          key='unknown'
          content={unmatchedModels.join(', ')}
          position='top'
          showArrow
        >
          <Avatar size='extra-extra-small' alt='unknown'>
            {t('其他')}
          </Avatar>
        </Tooltip>,
      );
    }

    return <AvatarGroup size='extra-extra-small'>{vendorAvatars}</AvatarGroup>;
  } else {
    return (
      <Tag color='white' shape='circle'>
        {t('无限制')}
      </Tag>
    );
  }
};

// Render IP restrictions column
const renderAllowIps = (text, t) => {
  if (!text || text.trim() === '') {
    return (
      <Tag color='white' shape='circle'>
        {t('无限制')}
      </Tag>
    );
  }

  const ips = text
    .split('\n')
    .map((ip) => ip.trim())
    .filter(Boolean);

  const displayIps = ips.slice(0, 1);
  const extraCount = ips.length - displayIps.length;

  const ipTags = displayIps.map((ip, idx) => (
    <Tag key={idx} shape='circle'>
      {ip}
    </Tag>
  ));

  if (extraCount > 0) {
    ipTags.push(
      <Tooltip
        key='extra'
        content={ips.slice(1).join(', ')}
        position='top'
        showArrow
      >
        <Tag shape='circle'>{'+' + extraCount}</Tag>
      </Tooltip>,
    );
  }

  return <Space wrap>{ipTags}</Space>;
};

// Render separate quota usage column
const renderQuotaUsage = (text, record, t) => {
  const { Paragraph } = Typography;
  const used = parseInt(record.used_quota) || 0;
  const remain = parseInt(record.remain_quota) || 0;
  const total = used + remain;
  if (record.unlimited_quota) {
    const popoverContent = (
      <div className='text-xs p-2'>
        <Paragraph copyable={{ content: renderQuota(used) }}>
          {t('已用额度')}: {renderQuota(used)}
        </Paragraph>
      </div>
    );
    return (
      <Popover content={popoverContent} position='top'>
        <Tag color='white' shape='circle'>
          {t('无限额度')}
        </Tag>
      </Popover>
    );
  }
  const percent = total > 0 ? (remain / total) * 100 : 0;
  const popoverContent = (
    <div className='text-xs p-2'>
      <Paragraph copyable={{ content: renderQuota(used) }}>
        {t('已用额度')}: {renderQuota(used)}
      </Paragraph>
      <Paragraph copyable={{ content: renderQuota(remain) }}>
        {t('剩余额度')}: {renderQuota(remain)} ({percent.toFixed(0)}%)
      </Paragraph>
      <Paragraph copyable={{ content: renderQuota(total) }}>
        {t('总额度')}: {renderQuota(total)}
      </Paragraph>
    </div>
  );
  return (
    <Popover content={popoverContent} position='top'>
      <Tag color='white' shape='circle'>
        <div className='flex flex-col items-end'>
          <span className='text-xs leading-none'>{`${renderQuota(remain)} / ${renderQuota(total)}`}</span>
          <Progress
            percent={percent}
            stroke={getProgressColor(percent)}
            aria-label='quota usage'
            format={() => `${percent.toFixed(0)}%`}
            style={{ width: '100%', marginTop: '1px', marginBottom: 0 }}
          />
        </div>
      </Tag>
    </Popover>
  );
};

const buildCurrentSnapshotPlan = (record) => ({
  id: 0,
  title: record.plan_title || `#${record.plan_id}`,
  duration_unit: record.plan_duration_unit,
  duration_value: record.plan_duration_value,
  custom_seconds: record.plan_custom_seconds,
  total_amount: record.plan_amount_total,
  quota_reset_period: record.plan_reset_period,
  quota_reset_custom_seconds: record.plan_reset_seconds,
});

const RenewSubscriptionPlanSelector = ({
  record,
  subscriptionPlans,
  onPlanChange,
  includeCurrentSnapshot = true,
  title,
  description = '',
  t,
}) => {
  const currentSnapshotPlan = buildCurrentSnapshotPlan(record);
  const defaultPlanId = includeCurrentSnapshot ? 0 : subscriptionPlans[0]?.id;
  const [selectedPlanId, setSelectedPlanId] = React.useState(defaultPlanId);

  React.useEffect(() => {
    setSelectedPlanId(defaultPlanId);
  }, [defaultPlanId]);

  React.useEffect(() => {
    onPlanChange(selectedPlanId);
  }, [onPlanChange, selectedPlanId]);

  const selectedPlan =
    includeCurrentSnapshot && selectedPlanId === 0
      ? currentSnapshotPlan
      : subscriptionPlans.find((plan) => plan.id === selectedPlanId) || null;
  const optionList = [
    ...(includeCurrentSnapshot
      ? [
          {
            value: 0,
            label: `${t('沿用当前套餐快照')} (${currentSnapshotPlan.title || '-'})`,
          },
        ]
      : []),
    ...subscriptionPlans.map((plan) => ({
      value: plan.id,
      label: plan.title || `#${plan.id}`,
    })),
  ];

  return (
    <div style={{ minWidth: 320 }}>
      <div style={{ marginBottom: 8 }}>{title || t('请选择续费套餐')}</div>
      <Select
        style={{ width: '100%' }}
        optionList={optionList}
        value={selectedPlanId}
        placeholder={t('请选择套餐')}
        onChange={(value) => {
          const nextPlanId =
            value === undefined || value === null ? undefined : Number(value);
          setSelectedPlanId(nextPlanId);
          onPlanChange(nextPlanId);
        }}
      />
      <div style={{ marginTop: 12, fontSize: 12, lineHeight: 1.8 }}>
        <div>
          {t('套餐')}：{selectedPlan?.title || '-'}
        </div>
        <div>
          {t('有效期')}：{formatSubscriptionDuration(selectedPlan, t)}
        </div>
        <div>
          {t('额度刷新')}：{formatSubscriptionResetPeriod(selectedPlan, t)}
        </div>
        <div>
          {t('套餐额度')}：
          {Number(selectedPlan?.total_amount || 0) === 0
            ? t('无限')
            : renderQuota(selectedPlan?.total_amount || 0)}
        </div>
        <div style={{ marginTop: 8, color: 'var(--semi-color-text-2)' }}>
          {description || t('当前套餐未结束时，续费会先加入队列，待当前套餐结束后自动切换。')}
        </div>
      </div>
    </div>
  );
};

const TokenRenewalQueueManager = ({
  record,
  onRemoveRenewal,
  t,
}) => {
  const [queue, setQueue] = React.useState(record?.renewal_queue || []);
  const [removingIndex, setRemovingIndex] = React.useState(null);

  React.useEffect(() => {
    setQueue(record?.renewal_queue || []);
  }, [record]);

  if (!queue.length) {
    return <div>{t('暂无待续费套餐')}</div>;
  }

  return (
    <div style={{ minWidth: 380, maxHeight: 420, overflowY: 'auto' }}>
      {queue.map((item) => (
        <div
          key={`${item.queue_index}-${item.plan_id}-${item.queued_at}`}
          style={{
            border: '1px solid var(--semi-color-border)',
            borderRadius: 8,
            padding: 12,
            marginBottom: 12,
          }}
        >
          <div style={{ display: 'flex', justifyContent: 'space-between', gap: 12 }}>
            <div style={{ fontWeight: 600 }}>
              {item.plan_title || `#${item.plan_id}`}
            </div>
            <Tag color='orange' shape='circle' size='small'>
              #{item.queue_index + 1}
            </Tag>
          </div>
          <div style={{ marginTop: 8, fontSize: 12, lineHeight: 1.8 }}>
            <div>
              {t('加入时间')}：{timestamp2string(item.queued_at)}
            </div>
            <div>
              {t('有效期')}：{formatSubscriptionDuration(item, t)}
            </div>
            <div>
              {t('额度刷新')}：{formatSubscriptionResetPeriod(item, t)}
            </div>
            <div>
              {t('套餐额度')}：
              {Number(item?.total_amount || 0) === 0
                ? t('无限')
                : renderQuota(item?.total_amount || 0)}
            </div>
          </div>
          <div style={{ marginTop: 12 }}>
            <Button
              type='danger'
              size='small'
              loading={removingIndex === item.queue_index}
              onClick={() => {
                Modal.confirm({
                  title: t('确认删除这条待续费吗？'),
                  content: t('删除后不会影响当前正在使用的套餐，只会移除这条排队续费。'),
                  onOk: async () => {
                    setRemovingIndex(item.queue_index);
                    try {
                      const updated = await onRemoveRenewal(
                        record.id,
                        item.queue_index,
                      );
                      if (updated) {
                        setQueue(updated.renewal_queue || []);
                      }
                    } finally {
                      setRemovingIndex(null);
                    }
                  },
                });
              }}
            >
              {t('删除待续费')}
            </Button>
          </div>
        </div>
      ))}
    </div>
  );
};

// Render operations column
const renderOperations = (
  text,
  record,
  onOpenLink,
  setEditingToken,
  setShowEdit,
  manageToken,
  renewSubscriptionToken,
  upgradeSubscriptionToken,
  removeSubscriptionTokenRenewal,
  subscriptionPlans,
  refresh,
  t,
) => {
  const isAdminUser = isAdmin();
  const canRenewSubscription = isAdminUser && record.plan_id > 0;
  const now = Math.floor(Date.now() / 1000);
  const upgradePlans = subscriptionPlans.filter((plan) => plan.id !== record.plan_id);
  const canUpgradeSubscription =
    isAdminUser &&
    record.plan_id > 0 &&
    record.activation_time > 0 &&
    (record.expired_time <= 0 || record.expired_time > now);
  let chatsArray = [];
  try {
    const raw = localStorage.getItem('chats');
    const parsed = JSON.parse(raw);
    if (Array.isArray(parsed)) {
      for (let i = 0; i < parsed.length; i++) {
        const item = parsed[i];
        const name = Object.keys(item)[0];
        if (!name) continue;
        chatsArray.push({
          node: 'item',
          key: i,
          name,
          value: item[name],
          onClick: () => onOpenLink(name, item[name], record),
        });
      }
    }
  } catch (_) {
    showError(t('聊天链接配置错误，请联系管理员'));
  }

  return (
    <Space wrap>
      <SplitButtonGroup
        className='overflow-hidden'
        aria-label={t('项目操作按钮组')}
      >
        <Button
          size='small'
          type='tertiary'
          onClick={() => {
            if (chatsArray.length === 0) {
              showError(t('请联系管理员配置聊天链接'));
            } else {
              const first = chatsArray[0];
              onOpenLink(first.name, first.value, record);
            }
          }}
        >
          {t('聊天')}
        </Button>
        <Dropdown trigger='click' position='bottomRight' menu={chatsArray}>
          <Button
            type='tertiary'
            icon={<IconTreeTriangleDown />}
            size='small'
          ></Button>
        </Dropdown>
      </SplitButtonGroup>

      {record.status === 1 ? (
        <Button
          type='danger'
          size='small'
          onClick={async () => {
            await manageToken(record.id, 'disable', record);
            await refresh();
          }}
        >
          {t('禁用')}
        </Button>
      ) : (
        <Button
          size='small'
          onClick={async () => {
            await manageToken(record.id, 'enable', record);
            await refresh();
          }}
        >
          {t('启用')}
        </Button>
      )}

      <Button
        type='tertiary'
        size='small'
        onClick={() => {
          setEditingToken(record);
          setShowEdit(true);
        }}
      >
        {t('编辑')}
      </Button>

      {canRenewSubscription && (
        <>
          {Number(record.renewal_queued_count || 0) > 0 && (
            <Button
              type='secondary'
              size='small'
              onClick={() => {
                Modal.info({
                  title: t('待续费列表'),
                  footer: null,
                  content: (
                    <TokenRenewalQueueManager
                      record={record}
                      onRemoveRenewal={removeSubscriptionTokenRenewal}
                      t={t}
                    />
                  ),
                });
              }}
            >
              {t('待续费')} ({record.renewal_queued_count})
            </Button>
          )}

          <Button
            type='warning'
            size='small'
            onClick={() => {
              let selectedPlanId = 0;
              Modal.confirm({
                title: t('请选择续费套餐'),
                content: (
                  <RenewSubscriptionPlanSelector
                    record={record}
                    subscriptionPlans={subscriptionPlans}
                    onPlanChange={(planId) => {
                      selectedPlanId = planId;
                    }}
                    title={t('请选择续费套餐')}
                    t={t}
                  />
                ),
                onOk: async () => {
                  const ok = await renewSubscriptionToken(
                    record.id,
                    selectedPlanId,
                  );
                  if (ok) {
                    await refresh();
                  }
                },
              });
            }}
          >
            {t('续费')}
          </Button>

          {canUpgradeSubscription && (
            <Button
              type='primary'
              size='small'
              onClick={() => {
                if (upgradePlans.length === 0) {
                  showError(t('暂无可升级套餐'));
                  return;
                }
                let selectedPlanId = upgradePlans[0]?.id;
                Modal.confirm({
                  title: t('请选择升级套餐'),
                  content: (
                    <RenewSubscriptionPlanSelector
                      record={record}
                      subscriptionPlans={upgradePlans}
                      includeCurrentSnapshot={false}
                      title={t('请选择升级套餐')}
                      description={t(
                        '升级会立即切换当前套餐、重置本周期额度，但保持原到期时间不变。',
                      )}
                      onPlanChange={(planId) => {
                        selectedPlanId = planId;
                      }}
                      t={t}
                    />
                  ),
                  onOk: async () => {
                    if (!selectedPlanId) {
                      showError(t('请选择升级套餐'));
                      return false;
                    }
                    const ok = await upgradeSubscriptionToken(
                      record.id,
                      selectedPlanId,
                    );
                    if (ok) {
                      await refresh();
                    }
                    return ok;
                  },
                });
              }}
            >
              {t('升级套餐')}
            </Button>
          )}
        </>
      )}

      <Button
        type='danger'
        size='small'
        onClick={() => {
          Modal.confirm({
            title: t('确定是否要删除此令牌？'),
            content: t('此修改将不可逆'),
            onOk: () => {
              (async () => {
                await manageToken(record.id, 'delete', record);
                await refresh();
              })();
            },
          });
        }}
      >
        {t('删除')}
      </Button>
    </Space>
  );
};

export const getTokensColumns = ({
  t,
  showKeys,
  setShowKeys,
  copyText,
  manageToken,
  renewSubscriptionToken,
  upgradeSubscriptionToken,
  removeSubscriptionTokenRenewal,
  subscriptionPlans,
  onOpenLink,
  setEditingToken,
  setShowEdit,
  refresh,
}) => {
  return [
    {
      title: t('名称'),
      dataIndex: 'name',
    },
    {
      title: t('状态'),
      dataIndex: 'status',
      key: 'status',
      render: (text, record) => renderStatus(text, record, t),
    },
    {
      title: t('类型'),
      key: 'token_type',
      render: (text, record) => renderTokenType(record, t),
    },
    {
      title: t('套餐'),
      key: 'plan_title',
      render: (text, record) => renderPlanTitle(record, t),
    },
    {
      title: t('剩余额度/总额度'),
      key: 'quota_usage',
      render: (text, record) => renderQuotaUsage(text, record, t),
    },
    {
      title: t('分组'),
      dataIndex: 'group',
      key: 'group',
      render: (text, record) => renderGroupColumn(text, record, t),
    },
    {
      title: t('密钥'),
      key: 'token_key',
      render: (text, record) =>
        renderTokenKey(text, record, showKeys, setShowKeys, copyText),
    },
    {
      title: t('可用模型'),
      dataIndex: 'model_limits',
      render: (text, record) => renderModelLimits(text, record, t),
    },
    {
      title: t('IP限制'),
      dataIndex: 'allow_ips',
      render: (text) => renderAllowIps(text, t),
    },
    {
      title: t('创建时间'),
      dataIndex: 'created_time',
      render: (text, record, index) => {
        return <div>{renderTimestamp(text)}</div>;
      },
    },
    {
      title: t('过期时间'),
      dataIndex: 'expired_time',
      render: (text, record, index) => {
        if (record.plan_id > 0 && record.activation_time === 0) {
          return <div>{t('首用激活')}</div>;
        }
        return (
          <div>
            {record.expired_time === -1 ? t('永不过期') : renderTimestamp(text)}
          </div>
        );
      },
    },
    {
      title: t('激活时间'),
      dataIndex: 'activation_time',
      render: (text, record) => {
        if (record.plan_id <= 0) {
          return <div>—</div>;
        }
        return <div>{text > 0 ? renderTimestamp(text) : t('未激活')}</div>;
      },
    },
    {
      title: '',
      dataIndex: 'operate',
      fixed: 'right',
      render: (text, record, index) =>
        renderOperations(
          text,
          record,
          onOpenLink,
          setEditingToken,
          setShowEdit,
          manageToken,
          renewSubscriptionToken,
          upgradeSubscriptionToken,
          removeSubscriptionTokenRenewal,
          subscriptionPlans,
          refresh,
          t,
        ),
    },
  ];
};
