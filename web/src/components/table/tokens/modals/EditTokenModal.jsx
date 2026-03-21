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

import React, { useEffect, useState, useContext, useRef } from 'react';
import {
  API,
  downloadTextAsFile,
  showError,
  showSuccess,
  timestamp2string,
  renderGroupOption,
  renderQuotaWithPrompt,
  getModelCategories,
  selectFilter,
  isAdmin,
} from '../../../../helpers';
import { useIsMobile } from '../../../../hooks/common/useIsMobile';
import {
  Button,
  SideSheet,
  Space,
  Spin,
  Typography,
  Card,
  Tag,
  Avatar,
  Form,
  Col,
  Row,
} from '@douyinfe/semi-ui';
import {
  IconCreditCard,
  IconLink,
  IconSave,
  IconClose,
  IconKey,
} from '@douyinfe/semi-icons';
import { useTranslation } from 'react-i18next';
import { StatusContext } from '../../../../context/Status';
import {
  formatSubscriptionDuration,
  formatSubscriptionResetPeriod,
} from '../../../../helpers/subscriptionFormat';

const { Text, Title } = Typography;

const EditTokenModal = (props) => {
  const { t } = useTranslation();
  const [statusState, statusDispatch] = useContext(StatusContext);
  const [loading, setLoading] = useState(false);
  const isMobile = useIsMobile();
  const formApiRef = useRef(null);
  const [models, setModels] = useState([]);
  const [groups, setGroups] = useState([]);
  const [subscriptionPlans, setSubscriptionPlans] = useState([]);
  const isEdit = props.editingToken.id !== undefined;
  const isAdminUser = isAdmin();

  const getInitValues = () => ({
    name: '',
    remain_quota: 0,
    expired_time: -1,
    unlimited_quota: true,
    model_limits_enabled: false,
    model_limits: [],
    allow_ips: '',
    group: '',
    cross_group_retry: false,
    tokenCount: 1,
    plan_id: 0,
  });

  const handleCancel = () => {
    props.handleClose();
  };

  const setExpiredTime = (month, day, hour, minute) => {
    let now = new Date();
    let timestamp = now.getTime() / 1000;
    let seconds = month * 30 * 24 * 60 * 60;
    seconds += day * 24 * 60 * 60;
    seconds += hour * 60 * 60;
    seconds += minute * 60;
    if (!formApiRef.current) return;
    if (seconds !== 0) {
      timestamp += seconds;
      formApiRef.current.setValue('expired_time', timestamp2string(timestamp));
    } else {
      formApiRef.current.setValue('expired_time', -1);
    }
  };

  const loadModels = async () => {
    let res = await API.get(`/api/user/models`);
    const { success, message, data } = res.data;
    if (success) {
      const categories = getModelCategories(t);
      let localModelOptions = data.map((model) => {
        let icon = null;
        for (const [key, category] of Object.entries(categories)) {
          if (key !== 'all' && category.filter({ model_name: model })) {
            icon = category.icon;
            break;
          }
        }
        return {
          label: (
            <span className='flex items-center gap-1'>
              {icon}
              {model}
            </span>
          ),
          value: model,
        };
      });
      setModels(localModelOptions);
    } else {
      showError(t(message));
    }
  };

  const loadGroups = async () => {
    let res = await API.get(`/api/user/self/groups`);
    const { success, message, data } = res.data;
    if (success) {
      let localGroupOptions = Object.entries(data).map(([group, info]) => ({
        label: info.desc,
        value: group,
        ratio: info.ratio,
      }));
      if (statusState?.status?.default_use_auto_group) {
        if (localGroupOptions.some((group) => group.value === 'auto')) {
          localGroupOptions.sort((a, b) => (a.value === 'auto' ? -1 : 1));
        }
      }
      setGroups(localGroupOptions);
      // if (statusState?.status?.default_use_auto_group && formApiRef.current) {
      //   formApiRef.current.setValue('group', 'auto');
      // }
    } else {
      showError(t(message));
    }
  };

  const loadSubscriptionPlans = async () => {
    if (!isAdminUser) return;
    let res = await API.get('/api/subscription/admin/plans');
    const { success, message, data } = res.data;
    if (success) {
      const plans = (data || []).map((item) => item.plan || item).filter(Boolean);
      setSubscriptionPlans(plans);
    } else {
      showError(t(message));
    }
  };

  const loadToken = async () => {
    setLoading(true);
    let res = await API.get(`/api/token/${props.editingToken.id}`);
    const { success, message, data } = res.data;
    if (success) {
      if (data.plan_id > 0 && data.activation_time === 0) {
        data.expired_time = -1;
      } else if (data.expired_time !== -1) {
        data.expired_time = timestamp2string(data.expired_time);
      }
      if (data.model_limits !== '') {
        data.model_limits = data.model_limits.split(',');
      } else {
        data.model_limits = [];
      }
      if (formApiRef.current) {
        formApiRef.current.setValues({ ...getInitValues(), ...data });
      }
    } else {
      showError(message);
    }
    setLoading(false);
  };

  useEffect(() => {
    if (formApiRef.current) {
      if (!isEdit) {
        formApiRef.current.setValues(getInitValues());
      }
    }
    loadModels();
    loadGroups();
    loadSubscriptionPlans();
  }, [props.editingToken.id]);

  useEffect(() => {
    if (props.visiable) {
      if (isEdit) {
        loadToken();
      } else {
        formApiRef.current?.setValues(getInitValues());
      }
    } else {
      formApiRef.current?.reset();
    }
  }, [props.visiable, props.editingToken.id]);

  const generateRandomSuffix = () => {
    const characters =
      'ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789';
    let result = '';
    for (let i = 0; i < 6; i++) {
      result += characters.charAt(
        Math.floor(Math.random() * characters.length),
      );
    }
    return result;
  };

  const findPlanById = (planId) => {
    const id = parseInt(planId, 10) || 0;
    return subscriptionPlans.find((plan) => plan.id === id);
  };

  const handlePlanChange = (planId) => {
    if (!formApiRef.current) return;
    const plan = findPlanById(planId);
    if (!plan) {
      formApiRef.current.setValue('remain_quota', 0);
      formApiRef.current.setValue('unlimited_quota', true);
      formApiRef.current.setValue('expired_time', -1);
      return;
    }
    const currentName = (formApiRef.current.getValue('name') || '').trim();
    if (!currentName) {
      formApiRef.current.setValue('name', plan.title || '');
    }
    formApiRef.current.setValue('remain_quota', Number(plan.total_amount || 0));
    formApiRef.current.setValue('unlimited_quota', Number(plan.total_amount || 0) === 0);
    formApiRef.current.setValue('expired_time', -1);
  };

  const submit = async (values) => {
    setLoading(true);
    if (isEdit) {
      let { tokenCount: _tc, ...localInputs } = values;
      localInputs.plan_id = parseInt(localInputs.plan_id, 10) || 0;
      const isSubscriptionToken = localInputs.plan_id > 0;
      localInputs.remain_quota = parseInt(localInputs.remain_quota);
      if (!isSubscriptionToken && localInputs.expired_time !== -1) {
        let time = Date.parse(localInputs.expired_time);
        if (isNaN(time)) {
          showError(t('过期时间格式错误！'));
          setLoading(false);
          return;
        }
        localInputs.expired_time = Math.ceil(time / 1000);
      }
      if (isSubscriptionToken) {
        localInputs.expired_time = 0;
        localInputs.unlimited_quota = Number(localInputs.remain_quota || 0) === 0;
      }
      localInputs.model_limits = localInputs.model_limits.join(',');
      localInputs.model_limits_enabled = localInputs.model_limits.length > 0;
      let res = await API.put(`/api/token/`, {
        ...localInputs,
        id: parseInt(props.editingToken.id),
      });
      const { success, message } = res.data;
      if (success) {
        showSuccess(t('令牌更新成功！'));
        props.refresh();
        props.handleClose();
      } else {
        showError(t(message));
      }
    } else {
      const count = parseInt(values.tokenCount, 10) || 1;
      const planId = parseInt(values.plan_id, 10) || 0;
      if (planId > 0) {
        if (!isAdminUser) {
          showError(t('仅管理员可创建订阅型令牌'));
          setLoading(false);
          return;
        }
        const localInputs = { ...values };
        localInputs.plan_id = planId;
        localInputs.model_limits = (localInputs.model_limits || []).join(',');
        localInputs.model_limits_enabled = localInputs.model_limits.length > 0;
        const res = await API.post('/api/token/admin/issue', {
          name: (localInputs.name || '').trim(),
          plan_id: planId,
          group: localInputs.group || '',
          cross_group_retry: !!localInputs.cross_group_retry,
          model_limits_enabled: !!localInputs.model_limits_enabled,
          model_limits: localInputs.model_limits,
          allow_ips: localInputs.allow_ips || '',
          token_count: count,
        });
        const { success, message, data } = res.data;
        if (success) {
          const issuedTokens = data?.tokens || [];
          showSuccess(t('订阅型令牌创建成功！'));
          props.refresh();
          props.handleClose();
          formApiRef.current?.setValues(getInitValues());
          if (issuedTokens.length > 0) {
            const text = issuedTokens
              .map((item) => `${item.name || ''}\t${item.key}`)
              .join('\n');
            Modal.confirm({
              title: t('订阅型令牌创建成功'),
              content: (
                <div>
                  <p>{t('已生成订阅型令牌，是否下载令牌清单？')}</p>
                  <p>{t('文件中将包含令牌名称与完整密钥。')}</p>
                </div>
              ),
              onOk: () => {
                const plan = findPlanById(planId);
                downloadTextAsFile(text, `${plan?.title || 'subscription-token'}.txt`);
              },
            });
          }
        } else {
          showError(t(message));
        }
        setLoading(false);
        return;
      }
      let successCount = 0;
      for (let i = 0; i < count; i++) {
        let { tokenCount: _tc, ...localInputs } = values;
        const baseName =
          values.name.trim() === '' ? 'default' : values.name.trim();
        if (i !== 0 || values.name.trim() === '') {
          localInputs.name = `${baseName}-${generateRandomSuffix()}`;
        } else {
          localInputs.name = baseName;
        }
        localInputs.remain_quota = parseInt(localInputs.remain_quota);

        if (localInputs.expired_time !== -1) {
          let time = Date.parse(localInputs.expired_time);
          if (isNaN(time)) {
            showError(t('过期时间格式错误！'));
            setLoading(false);
            break;
          }
          localInputs.expired_time = Math.ceil(time / 1000);
        }
        localInputs.model_limits = localInputs.model_limits.join(',');
        localInputs.model_limits_enabled = localInputs.model_limits.length > 0;
        let res = await API.post(`/api/token/`, localInputs);
        const { success, message } = res.data;
        if (success) {
          successCount++;
        } else {
          showError(t(message));
          break;
        }
      }
      if (successCount > 0) {
        showSuccess(t('令牌创建成功，请在列表页面点击复制获取令牌！'));
        props.refresh();
        props.handleClose();
      }
    }
    setLoading(false);
    formApiRef.current?.setValues(getInitValues());
  };

  return (
    <SideSheet
      placement={isEdit ? 'right' : 'left'}
      title={
        <Space>
          {isEdit ? (
            <Tag color='blue' shape='circle'>
              {t('更新')}
            </Tag>
          ) : (
            <Tag color='green' shape='circle'>
              {t('新建')}
            </Tag>
          )}
          <Title heading={4} className='m-0'>
            {isEdit ? t('更新令牌信息') : t('创建新的令牌')}
          </Title>
        </Space>
      }
      bodyStyle={{ padding: '0' }}
      visible={props.visiable}
      width={isMobile ? '100%' : 600}
      footer={
        <div className='flex justify-end bg-white'>
          <Space>
            <Button
              theme='solid'
              className='!rounded-lg'
              onClick={() => formApiRef.current?.submitForm()}
              icon={<IconSave />}
              loading={loading}
            >
              {t('提交')}
            </Button>
            <Button
              theme='light'
              className='!rounded-lg'
              type='primary'
              onClick={handleCancel}
              icon={<IconClose />}
            >
              {t('取消')}
            </Button>
          </Space>
        </div>
      }
      closeIcon={null}
      onCancel={() => handleCancel()}
    >
      <Spin spinning={loading}>
        <Form
          key={isEdit ? 'edit' : 'new'}
          initValues={getInitValues()}
          getFormApi={(api) => (formApiRef.current = api)}
          onSubmit={submit}
        >
          {({ values }) => {
            const selectedPlan = findPlanById(values.plan_id);
            const isSubscriptionToken = (parseInt(values.plan_id, 10) || 0) > 0;
            return (
            <div className='p-2'>
              {/* 基本信息 */}
              <Card className='!rounded-2xl shadow-sm border-0'>
                <div className='flex items-center mb-2'>
                  <Avatar size='small' color='blue' className='mr-2 shadow-md'>
                    <IconKey size={16} />
                  </Avatar>
                  <div>
                    <Text className='text-lg font-medium'>{t('基本信息')}</Text>
                    <div className='text-xs text-gray-600'>
                      {t('设置令牌的基本信息')}
                    </div>
                  </div>
                </div>
                <Row gutter={12}>
                  <Col span={24}>
                    <Form.Input
                      field='name'
                      label={t('名称')}
                      placeholder={
                        isSubscriptionToken
                          ? t('留空则默认使用套餐名称')
                          : t('请输入名称')
                      }
                      rules={
                        isSubscriptionToken
                          ? []
                          : [{ required: true, message: t('请输入名称') }]
                      }
                      showClear
                    />
                  </Col>
                  {isAdminUser && (
                    <Col span={24}>
                      <Form.Select
                        field='plan_id'
                        label={t('订阅套餐')}
                        placeholder={t('不选择则创建普通令牌')}
                        optionList={[
                          { value: 0, label: t('普通令牌') },
                          ...subscriptionPlans.map((plan) => ({
                            value: plan.id,
                            label: plan.title || String(plan.id),
                          })),
                        ]}
                        onChange={handlePlanChange}
                        showClear
                        style={{ width: '100%' }}
                        disabled={isEdit}
                      />
                    </Col>
                  )}
                  {isSubscriptionToken && selectedPlan && (
                    <Col span={24}>
                      <Card className='!rounded-xl border border-blue-100 bg-blue-50/60'>
                        <div className='text-sm text-gray-700 space-y-1'>
                          <div>
                            {t('套餐时长')}：
                            {formatSubscriptionDuration(selectedPlan, t)}
                          </div>
                          <div>
                            {t('套餐额度')}：
                            {Number(selectedPlan.total_amount || 0) === 0
                              ? t('无限')
                              : renderQuotaWithPrompt(selectedPlan.total_amount)}
                          </div>
                          <div>
                            {t('额度刷新')}：
                            {formatSubscriptionResetPeriod(selectedPlan, t)}
                          </div>
                          <div>
                            {t('激活方式')}：{t('首用激活，未使用前不开始计时')}
                          </div>
                        </div>
                      </Card>
                    </Col>
                  )}
                  <Col span={24}>
                    {groups.length > 0 ? (
                      <Form.Select
                        field='group'
                        label={t('令牌分组')}
                        placeholder={t('令牌分组，默认为用户的分组')}
                        optionList={groups}
                        renderOptionItem={renderGroupOption}
                        showClear
                        style={{ width: '100%' }}
                      />
                    ) : (
                      <Form.Select
                        placeholder={t('管理员未设置用户可选分组')}
                        disabled
                        label={t('令牌分组')}
                        style={{ width: '100%' }}
                      />
                    )}
                  </Col>
                  <Col
                    span={24}
                    style={{
                      display: values.group === 'auto' ? 'block' : 'none',
                    }}
                  >
                    <Form.Switch
                      field='cross_group_retry'
                      label={t('跨分组重试')}
                      size='default'
                      extraText={t(
                        '开启后，当前分组渠道失败时会按顺序尝试下一个分组的渠道',
                      )}
                    />
                  </Col>
                  {isSubscriptionToken ? (
                    <Col span={24}>
                      <Form.Slot label={t('有效期')}>
                        <Text type='secondary'>
                          {t('按所选套餐配置，在令牌首次调用时开始计算。')}
                        </Text>
                      </Form.Slot>
                    </Col>
                  ) : (
                    <>
                      <Col xs={24} sm={24} md={24} lg={10} xl={10}>
                        <Form.DatePicker
                          field='expired_time'
                          label={t('过期时间')}
                          type='dateTime'
                          placeholder={t('请选择过期时间')}
                          rules={[
                            { required: true, message: t('请选择过期时间') },
                            {
                              validator: (rule, value) => {
                                if (value === -1 || !value)
                                  return Promise.resolve();
                                const time = Date.parse(value);
                                if (isNaN(time)) {
                                  return Promise.reject(t('过期时间格式错误！'));
                                }
                                if (time <= Date.now()) {
                                  return Promise.reject(
                                    t('过期时间不能早于当前时间！'),
                                  );
                                }
                                return Promise.resolve();
                              },
                            },
                          ]}
                          showClear
                          style={{ width: '100%' }}
                        />
                      </Col>
                      <Col xs={24} sm={24} md={24} lg={14} xl={14}>
                        <Form.Slot label={t('过期时间快捷设置')}>
                          <Space wrap>
                            <Button
                              theme='light'
                              type='primary'
                              onClick={() => setExpiredTime(0, 0, 0, 0)}
                            >
                              {t('永不过期')}
                            </Button>
                            <Button
                              theme='light'
                              type='tertiary'
                              onClick={() => setExpiredTime(1, 0, 0, 0)}
                            >
                              {t('一个月')}
                            </Button>
                            <Button
                              theme='light'
                              type='tertiary'
                              onClick={() => setExpiredTime(0, 1, 0, 0)}
                            >
                              {t('一天')}
                            </Button>
                            <Button
                              theme='light'
                              type='tertiary'
                              onClick={() => setExpiredTime(0, 0, 1, 0)}
                            >
                              {t('一小时')}
                            </Button>
                          </Space>
                        </Form.Slot>
                      </Col>
                    </>
                  )}
                  {!isEdit && (
                    <Col span={24}>
                      <Form.InputNumber
                        field='tokenCount'
                        label={t('新建数量')}
                        min={1}
                        extraText={t('批量创建时会在名称后自动添加随机后缀')}
                        rules={[
                          { required: true, message: t('请输入新建数量') },
                        ]}
                        style={{ width: '100%' }}
                      />
                    </Col>
                  )}
                </Row>
              </Card>

              {/* 额度设置 */}
              <Card className='!rounded-2xl shadow-sm border-0'>
                <div className='flex items-center mb-2'>
                  <Avatar size='small' color='green' className='mr-2 shadow-md'>
                    <IconCreditCard size={16} />
                  </Avatar>
                  <div>
                    <Text className='text-lg font-medium'>{t('额度设置')}</Text>
                    <div className='text-xs text-gray-600'>
                      {t('设置令牌可用额度和数量')}
                    </div>
                  </div>
                </div>
                <Row gutter={12}>
                  {isSubscriptionToken ? (
                    <Col span={24}>
                      <Form.Slot label={t('额度')}>
                        <Text type='secondary'>
                          {selectedPlan && Number(selectedPlan.total_amount || 0) === 0
                            ? t('该订阅型令牌按套餐提供无限额度')
                            : t('该订阅型令牌将按套餐预置额度，并在刷新周期到达时自动重置')}
                        </Text>
                      </Form.Slot>
                    </Col>
                  ) : (
                    <>
                      <Col span={24}>
                        <Form.AutoComplete
                          field='remain_quota'
                          label={t('额度')}
                          placeholder={t('请输入额度')}
                          type='number'
                          disabled={values.unlimited_quota}
                          extraText={renderQuotaWithPrompt(values.remain_quota)}
                          rules={
                            values.unlimited_quota
                              ? []
                              : [{ required: true, message: t('请输入额度') }]
                          }
                          data={[
                            { value: 500000, label: '1$' },
                            { value: 5000000, label: '10$' },
                            { value: 25000000, label: '50$' },
                            { value: 50000000, label: '100$' },
                            { value: 250000000, label: '500$' },
                            { value: 500000000, label: '1000$' },
                          ]}
                        />
                      </Col>
                      <Col span={24}>
                        <Form.Switch
                          field='unlimited_quota'
                          label={t('无限额度')}
                          size='default'
                          extraText={t(
                            '令牌的额度仅用于限制令牌本身的最大额度使用量，实际的使用受到账户的剩余额度限制',
                          )}
                        />
                      </Col>
                    </>
                  )}
                </Row>
              </Card>

              {/* 访问限制 */}
              <Card className='!rounded-2xl shadow-sm border-0'>
                <div className='flex items-center mb-2'>
                  <Avatar
                    size='small'
                    color='purple'
                    className='mr-2 shadow-md'
                  >
                    <IconLink size={16} />
                  </Avatar>
                  <div>
                    <Text className='text-lg font-medium'>{t('访问限制')}</Text>
                    <div className='text-xs text-gray-600'>
                      {t('设置令牌的访问限制')}
                    </div>
                  </div>
                </div>
                <Row gutter={12}>
                  <Col span={24}>
                    <Form.Select
                      field='model_limits'
                      label={t('模型限制列表')}
                      placeholder={t(
                        '请选择该令牌支持的模型，留空支持所有模型',
                      )}
                      multiple
                      optionList={models}
                      extraText={t('非必要，不建议启用模型限制')}
                      filter={selectFilter}
                      autoClearSearchValue={false}
                      searchPosition='dropdown'
                      showClear
                      style={{ width: '100%' }}
                    />
                  </Col>
                  <Col span={24}>
                    <Form.TextArea
                      field='allow_ips'
                      label={t('IP白名单（支持CIDR表达式）')}
                      placeholder={t('允许的IP，一行一个，不填写则不限制')}
                      autosize
                      rows={1}
                      extraText={t(
                        '请勿过度信任此功能，IP可能被伪造，请配合nginx和cdn等网关使用',
                      )}
                      showClear
                      style={{ width: '100%' }}
                    />
                  </Col>
                </Row>
              </Card>
            </div>
          );
          }}
        </Form>
      </Spin>
    </SideSheet>
  );
};

export default EditTokenModal;
