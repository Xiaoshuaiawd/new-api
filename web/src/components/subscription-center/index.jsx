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

import React, { useContext, useEffect, useState } from 'react';
import { Avatar, Button, Card, Modal, Typography } from '@douyinfe/semi-ui';
import { Receipt, Sparkles } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { API, showError, showInfo, showSuccess } from '../../helpers';
import { StatusContext } from '../../context/Status';
import SubscriptionPlansCard from '../topup/SubscriptionPlansCard';
import TopupHistoryModal from '../topup/modals/TopupHistoryModal';

const { Text } = Typography;

const SubscriptionCenter = () => {
  const { t } = useTranslation();
  const [statusState] = useContext(StatusContext);
  const subscriptionOnlyModeEnabled =
    statusState?.status?.subscription_only_mode_enabled ??
    localStorage.getItem('subscription_only_mode_enabled') === 'true';
  const topUpLink = statusState?.status?.top_up_link || '';

  const [subscriptionPlans, setSubscriptionPlans] = useState([]);
  const [subscriptionLoading, setSubscriptionLoading] = useState(true);
  const [payMethods, setPayMethods] = useState([]);
  const [enableOnlineTopUp, setEnableOnlineTopUp] = useState(false);
  const [enableStripeTopUp, setEnableStripeTopUp] = useState(false);
  const [enableCreemTopUp, setEnableCreemTopUp] = useState(false);
  const [billingPreference, setBillingPreference] =
    useState('subscription_first');
  const [activeSubscriptions, setActiveSubscriptions] = useState([]);
  const [allSubscriptions, setAllSubscriptions] = useState([]);
  const [redemptionCode, setRedemptionCode] = useState('');
  const [redeeming, setRedeeming] = useState(false);
  const [openHistory, setOpenHistory] = useState(false);

  const openTopUpLink = () => {
    if (!topUpLink) {
      showError(t('超级管理员未设置充值链接！'));
      return;
    }
    window.open(topUpLink, '_blank');
  };

  const loadSubscriptionPlans = async () => {
    setSubscriptionLoading(true);
    try {
      const res = await API.get('/api/subscription/plans');
      if (res.data?.success) {
        setSubscriptionPlans(res.data.data || []);
      } else {
        setSubscriptionPlans([]);
      }
    } catch (error) {
      setSubscriptionPlans([]);
    } finally {
      setSubscriptionLoading(false);
    }
  };

  const loadSubscriptionSelf = async () => {
    try {
      const res = await API.get('/api/subscription/self');
      if (res.data?.success) {
        setBillingPreference(
          res.data.data?.billing_preference || 'subscription_first',
        );
        setActiveSubscriptions(res.data.data?.subscriptions || []);
        setAllSubscriptions(res.data.data?.all_subscriptions || []);
      }
    } catch (error) {
      // ignore
    }
  };

  const loadPaymentInfo = async () => {
    try {
      const res = await API.get('/api/subscription/payment_info');
      if (!res.data?.success) {
        return;
      }
      const data = res.data.data || {};
      let nextPayMethods = data.pay_methods || [];
      if (typeof nextPayMethods === 'string') {
        nextPayMethods = JSON.parse(nextPayMethods);
      }
      nextPayMethods = (nextPayMethods || []).filter(
        (method) => method?.name && method?.type,
      );
      setPayMethods(nextPayMethods);
      setEnableOnlineTopUp(!!data.enable_online_topup);
      setEnableStripeTopUp(!!data.enable_stripe_topup);
      setEnableCreemTopUp(!!data.enable_creem_topup);
    } catch (error) {
      setPayMethods([]);
      setEnableOnlineTopUp(false);
      setEnableStripeTopUp(false);
      setEnableCreemTopUp(false);
    }
  };

  const updateBillingPreference = async (pref) => {
    const previousPref = billingPreference;
    setBillingPreference(pref);
    try {
      const res = await API.put('/api/subscription/self/preference', {
        billing_preference: pref,
      });
      if (res.data?.success) {
        const normalizedPref =
          res.data?.data?.billing_preference || pref || previousPref;
        setBillingPreference(normalizedPref);
        showSuccess(t('更新成功'));
      } else {
        setBillingPreference(previousPref);
        showError(res.data?.message || t('更新失败'));
      }
    } catch (error) {
      setBillingPreference(previousPref);
      showError(t('请求失败'));
    }
  };

  const redeemSubscriptionKey = async () => {
    if (!redemptionCode) {
      showInfo(t('请输入兑换码！'));
      return;
    }
    setRedeeming(true);
    try {
      const res = await API.post('/api/subscription/redeem', {
        key: redemptionCode,
      });
      const { success, message, data } = res.data;
      if (!success) {
        showError(message);
        return;
      }
      const planTitle = data?.plan?.title || t('订阅套餐');
      showSuccess(t('订阅兑换成功！'));
      Modal.success({
        title: t('订阅兑换成功！'),
        content: t('成功兑换订阅套餐：') + planTitle,
        centered: true,
      });
      setRedemptionCode('');
      await loadSubscriptionSelf();
    } catch (error) {
      showError(t('请求失败'));
    } finally {
      setRedeeming(false);
    }
  };

  useEffect(() => {
    loadSubscriptionPlans().then();
    loadSubscriptionSelf().then();
    loadPaymentInfo().then();
  }, []);

  return (
    <div className='w-full max-w-7xl mx-auto mt-[60px] px-2'>
      <TopupHistoryModal
        visible={openHistory}
        onCancel={() => setOpenHistory(false)}
        t={t}
      />

      <Card className='!rounded-2xl shadow-sm border-0'>
        <div className='flex items-center justify-between mb-4 gap-3 flex-wrap'>
          <div className='flex items-center'>
            <Avatar size='small' color='blue' className='mr-3 shadow-md'>
              <Sparkles size={16} />
            </Avatar>
            <div>
              <Text className='text-lg font-medium'>{t('订阅中心')}</Text>
              <div className='text-xs text-gray-500'>
                {t('订阅购买、订阅型 Key 兑换与当前套餐状态统一在这里处理')}
              </div>
            </div>
          </div>
          <Button
            icon={<Receipt size={16} />}
            theme='solid'
            onClick={() => setOpenHistory(true)}
          >
            {t('账单')}
          </Button>
        </div>

        <div className='mb-4 rounded-2xl border border-blue-100 bg-blue-50/70 p-4 text-sm text-gray-600'>
          {subscriptionOnlyModeEnabled
            ? t('当前已开启订阅专用模式，系统会自动保持仅用订阅。')
            : t('订阅逻辑已从钱包管理拆分，后续订阅型 Key 的购买、兑换与使用都在订阅中心完成。')}
        </div>

        <SubscriptionPlansCard
          t={t}
          loading={subscriptionLoading}
          plans={subscriptionPlans}
          payMethods={payMethods}
          enableOnlineTopUp={enableOnlineTopUp}
          enableStripeTopUp={enableStripeTopUp}
          enableCreemTopUp={enableCreemTopUp}
          billingPreference={billingPreference}
          onChangeBillingPreference={updateBillingPreference}
          activeSubscriptions={activeSubscriptions}
          allSubscriptions={allSubscriptions}
          reloadSubscriptionSelf={loadSubscriptionSelf}
          redemptionCode={redemptionCode}
          setRedemptionCode={setRedemptionCode}
          onRedeem={redeemSubscriptionKey}
          redeeming={redeeming}
          topUpLink={topUpLink}
          openTopUpLink={openTopUpLink}
          withCard={false}
          subscriptionOnlyModeEnabled={subscriptionOnlyModeEnabled}
        />
      </Card>
    </div>
  );
};

export default SubscriptionCenter;
